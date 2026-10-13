package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/go-playground/validator/v10"
)

type Payload struct {
	ServerURL                string `json:"serverURL" validate:"required,http_url"`
	ServerPassword           string `json:"serverPassword" validate:"required"`
	BudgetSyncID             string `json:"budgetSyncId" validate:"required,uuid"`
	BudgetEncryptionPassword string `json:"budgetEncryptionPassword"`
	GroupName                string `json:"groupName" validate:"required"`
	Included                 string `json:"included"`
}

// Worker is one long-lived `node worker.js` process. main.go hands it jobs
// one at a time over stdin and reads exactly one JSON response line back
// from stdout per job - see worker.js for the other half of this protocol.
// Workers never persist anything about a job after answering it: each job
// still does its own ephemeral, tmpfs-backed fetch-and-wipe in actual.js.
// Pooling these processes only removes the cost of spawning node and
// loading @actual-app/api fresh for every single request; it changes
// nothing about what's retained between requests (nothing is).
type Worker struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	// dead is closed once the process has exited, for any reason. A worker
	// can answer a job and then exit immediately after (worker.js does this
	// when it recovers from an otherwise-fatal crash) - checking this after
	// a successful run() is how main.go tells that apart from a genuinely
	// healthy, reusable worker.
	dead chan struct{}
}

var (
	validate = newValidator()

	// requestTimeout bounds the whole request: waiting for a free worker
	// plus running the job on it. Overridable via REQUEST_TIMEOUT_SECONDS.
	requestTimeout = requestTimeoutFromEnv()

	// poolSize is how many node worker processes run concurrently, which
	// doubles as the max number of concurrent budget fetches - so concurrent
	// polls from many tenants don't all fetch simultaneously and starve
	// each other's CPU/network. Overridable via MAX_CONCURRENT_JOBS.
	poolSize = maxConcurrentJobsFromEnv()

	// workerPool holds idle, ready-to-use workers. Taking one out bounds
	// concurrency; a worker goes back in once its job is done, or a fresh
	// replacement goes in if it had to be killed after a failure/timeout.
	workerPool chan *Worker

	workersMu  sync.Mutex
	allWorkers []*Worker
)

func main() {
	if poolSize < 1 {
		poolSize = 1
	}
	workerPool = make(chan *Worker, poolSize)
	log.Printf("Starting %d node worker(s), request timeout: %s", poolSize, requestTimeout)
	for i := 0; i < poolSize; i++ {
		wk, err := startWorkerWithRetry()
		if err != nil {
			log.Fatalf("failed to start node worker pool: %v", err)
		}
		workerPool <- wk
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Printf("Received %s, terminating node workers...", sig)
		killAllWorkers()
		os.Exit(0)
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/markup", func(w http.ResponseWriter, r *http.Request) {
		log.Println("Received POST /api/markup request")
		var payload Payload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			log.Printf("Failed to decode JSON: %v\n", err)
			writeJSONError(w, "failed to decode json: "+err.Error())
			return
		}
		if validationError := validatePayload(payload); validationError != "" {
			writeJSONError(w, "invalid request payload: "+validationError)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
		defer cancel()

		jsonBytes, err := json.Marshal(payload)
		if err != nil {
			log.Printf("Failed to marshal payload: %v\n", err)
			writeJSONError(w, "failed to marshal payload: "+err.Error())
			return
		}

		var wk *Worker
		select {
		case wk = <-workerPool:
		case <-ctx.Done():
			log.Println("Timed out waiting for a free worker")
			writeJSONError(w, "server busy: timed out waiting for a free worker, please retry")
			return
		}

		line, err := wk.run(ctx, jsonBytes)
		if err != nil {
			log.Printf("Worker job failed: %v; discarding worker and spawning a replacement\n", err)
			go replaceWorker(wk)
			if errors.Is(err, context.DeadlineExceeded) {
				writeJSONError(w, "node worker timed out")
			} else {
				writeJSONError(w, "node worker failed: "+err.Error())
			}
			return
		}

		if line == "" {
			log.Println("Node worker returned empty output")
			go replaceWorker(wk) // crashed without even producing a line - don't reuse it
			writeJSONError(w, "node worker returned empty output")
			return
		}
		if !json.Valid([]byte(line)) {
			log.Printf("Node worker returned non-JSON output: %s\n", line)
			go replaceWorker(wk)
			writeJSONError(w, "node worker returned non-json output: "+line)
			return
		}

		// A successful response is always a JSON array (see README), so this
		// only ever matches the crash-recovery message worker.js sends right
		// before exiting - see its __retireWorker comment. Unmarshal errors
		// here (e.g. the line is a plain array) just mean "not that case".
		var crash struct {
			Error        string `json:"error"`
			RetireWorker bool   `json:"__retireWorker"`
		}
		if json.Unmarshal([]byte(line), &crash) == nil && crash.RetireWorker {
			log.Println("Worker is retiring after an internal crash; discarding and spawning a replacement")
			go replaceWorker(wk)
			writeJSONError(w, crash.Error)
			return
		}
		workerPool <- wk

		log.Printf("Sending response")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(line))
	})
	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}
	log.Println("Server running on http://localhost:8080")
	log.Fatal(server.ListenAndServe())
}

// run sends one job to the worker and waits for its one-line JSON reply,
// bounded by ctx. A ctx timeout (or any read/write error) leaves the
// worker's stream state undefined, so the caller must discard it rather
// than putting it back in the pool - see replaceWorker.
func (wk *Worker) run(ctx context.Context, payloadJSON []byte) (string, error) {
	type result struct {
		line string
		err  error
	}
	done := make(chan result, 1)
	go func() {
		if _, err := wk.stdin.Write(append(payloadJSON, '\n')); err != nil {
			done <- result{err: fmt.Errorf("write to worker: %w", err)}
			return
		}
		line, err := wk.stdout.ReadString('\n')
		if err != nil {
			done <- result{err: fmt.Errorf("read from worker: %w", err)}
			return
		}
		done <- result{line: strings.TrimSpace(line)}
	}()
	select {
	case res := <-done:
		return res.line, res.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func startWorker() (*Worker, error) {
	cmd := exec.Command("node", "worker.js")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start node worker: %w", err)
	}
	wk := &Worker{cmd: cmd, stdin: stdin, stdout: bufio.NewReader(stdout), dead: make(chan struct{})}
	// The only goroutine allowed to call cmd.Wait() - exec.Cmd forbids
	// calling it twice. killWorker() below just waits on wk.dead instead of
	// calling Wait() itself.
	go func() {
		_ = cmd.Wait()
		close(wk.dead)
	}()
	workersMu.Lock()
	allWorkers = append(allWorkers, wk)
	workersMu.Unlock()
	return wk, nil
}

func startWorkerWithRetry() (*Worker, error) {
	const maxAttempts = 3
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			time.Sleep(time.Duration(attempt-1) * time.Second)
		}
		wk, err := startWorker()
		if err == nil {
			return wk, nil
		}
		lastErr = err
		log.Printf("Worker start attempt %d/%d failed: %v\n", attempt, maxAttempts, err)
	}
	return nil, lastErr
}

// replaceWorker kills a worker that failed or timed out and, best-effort,
// starts a replacement so the pool stays at poolSize. Runs in its own
// goroutine so a slow respawn never delays the response already sent to
// the client.
func replaceWorker(old *Worker) {
	killWorker(old)
	wk, err := startWorkerWithRetry()
	if err != nil {
		log.Printf("Failed to respawn node worker, pool permanently reduced by one: %v\n", err)
		return
	}
	workerPool <- wk
}

func killWorker(wk *Worker) {
	_ = wk.stdin.Close()
	if wk.cmd.Process != nil {
		_ = wk.cmd.Process.Kill()
	}
	<-wk.dead // wait for the reaper goroutine (see startWorker) to confirm exit
	workersMu.Lock()
	for i, w := range allWorkers {
		if w == wk {
			allWorkers = append(allWorkers[:i], allWorkers[i+1:]...)
			break
		}
	}
	workersMu.Unlock()
}

func killAllWorkers() {
	workersMu.Lock()
	workers := append([]*Worker(nil), allWorkers...)
	workersMu.Unlock()
	for _, wk := range workers {
		_ = wk.stdin.Close()
		if wk.cmd.Process != nil {
			_ = wk.cmd.Process.Kill()
		}
	}
}

func writeJSONError(w http.ResponseWriter, message string) {
	log.Printf("Sending error response: %s\n", message)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": message,
	})
}

// maxConcurrentJobsFromEnv returns MAX_CONCURRENT_JOBS if set to a positive
// integer. Otherwise it defaults to min(NumCPU, defaultPoolSize): unlike the
// old per-request exec model, each unit here is a permanently-resident node
// process holding @actual-app/api in memory, so blindly defaulting to
// NumCPU (16+ on a many-core box) would eagerly start far more heavy
// processes than a small self-hosted deployment needs. Operators who want
// more can still set MAX_CONCURRENT_JOBS explicitly.
func maxConcurrentJobsFromEnv() int {
	if v := os.Getenv("MAX_CONCURRENT_JOBS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
		log.Printf("Invalid MAX_CONCURRENT_JOBS value %q, falling back to default\n", v)
	}
	const defaultPoolSize = 4
	if n := runtime.NumCPU(); n < defaultPoolSize {
		return n
	}
	return defaultPoolSize
}

// requestTimeoutFromEnv returns REQUEST_TIMEOUT_SECONDS if set to a positive
// integer, otherwise defaults to 45 seconds.
func requestTimeoutFromEnv() time.Duration {
	const defaultSeconds = 45
	if v := os.Getenv("REQUEST_TIMEOUT_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
		log.Printf("Invalid REQUEST_TIMEOUT_SECONDS value %q, falling back to %ds\n", v, defaultSeconds)
	}
	return defaultSeconds * time.Second
}

func newValidator() *validator.Validate {
	v := validator.New()
	v.RegisterTagNameFunc(func(field reflect.StructField) string {
		tag := field.Tag.Get("json")
		if tag == "" {
			return field.Name
		}
		name := strings.Split(tag, ",")[0]
		if name == "" || name == "-" {
			return field.Name
		}
		return name
	})
	_ = v.RegisterValidation("http_url", func(fl validator.FieldLevel) bool {
		raw := strings.TrimSpace(fl.Field().String())
		parsed, err := url.ParseRequestURI(raw)
		if err != nil || parsed.Host == "" {
			return false
		}
		return parsed.Scheme == "http" || parsed.Scheme == "https"
	})
	return v
}

func validatePayload(p Payload) string {
	if err := validate.Struct(p); err != nil {
		validationErrs, ok := err.(validator.ValidationErrors)
		if !ok {
			return "payload validation failed"
		}
		errs := make([]string, 0, len(validationErrs))
		for _, fieldErr := range validationErrs {
			fieldName := fieldErr.Field()
			switch fieldErr.Tag() {
			case "required":
				errs = append(errs, fmt.Sprintf("%s is required", fieldName))
			case "uuid":
				errs = append(errs, fmt.Sprintf("%s must be a valid UUID", fieldName))
			case "http_url":
				errs = append(errs, fmt.Sprintf("%s must be a valid http/https URL", fieldName))
			default:
				errs = append(errs, fmt.Sprintf("%s is invalid", fieldName))
			}
		}
		return strings.Join(errs, "; ")
	}

	return ""
}
