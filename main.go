package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os/exec"
	"reflect"
	"strings"
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

var validate = newValidator()

func main() {
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
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		jsonBytes, err := json.Marshal(payload)
		if err != nil {
			log.Printf("Failed to marshal payload: %v\n", err)
			writeJSONError(w, "failed to marshal payload: "+err.Error())
			return
		}

		cmd := exec.CommandContext(ctx, "node", "script.js", string(jsonBytes))
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if ctx.Err() == context.DeadlineExceeded {
			log.Println("Node script execution timed out")
			writeJSONError(w, "node script timed out")
			return
		}
		if err != nil {
			errMsg := strings.Builder{}
			errMsg.WriteString("node execution failed: ")
			errMsg.WriteString(err.Error())
			if s := strings.TrimSpace(stderr.String()); s != "" {
				errMsg.WriteString(" | stderr: ")
				errMsg.WriteString(s)
			}
			if s := strings.TrimSpace(string(out)); s != "" {
				errMsg.WriteString(" | stdout: ")
				errMsg.WriteString(s)
			}
			log.Printf("Command failed. Error: %v, Stdout: %s, Stderr: %s\n", err, string(out), stderr.String())
			writeJSONError(w, errMsg.String())
			return
		}

		result := strings.TrimSpace(string(out))
		lines := strings.Split(result, "\n")
		var last string
		for i := len(lines) - 1; i >= 0; i-- {
			line := strings.TrimSpace(lines[i])
			if line == "" {
				continue
			}
			last = line
			break
		}
		if last == "" {
			log.Println("Node script returned empty output")
			writeJSONError(w, "node script returned empty output")
			return
		}

		if !json.Valid([]byte(last)) {
			log.Printf("Node script returned non-JSON output: %s\n", last)
			writeJSONError(w, "node script returned non-json output: "+last)
			return
		}

		log.Printf("Sending response")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(last))
	})
	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}
	log.Println("Server running on http://localhost:8080")
	log.Fatal(server.ListenAndServe())
}

func writeJSONError(w http.ResponseWriter, message string) {
	log.Printf("Sending error response: %s\n", message)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": message,
	})
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
