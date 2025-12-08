package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

type Payload struct {
	ServerURL                string `json:"serverURL"`
	ServerPassword           string `json:"serverPassword"`
	BudgetSyncID             string `json:"budgetSyncId"`
	BudgetEncryptionPassword string `json:"budgetEncryptionPassword"`
	GroupName                string `json:"groupName"`
	Included                 string `json:"included"`
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/markup", func(w http.ResponseWriter, r *http.Request) {
		log.Println("Received POST /api/markup request")
		var payload Payload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			log.Printf("Failed to decode JSON: %v\n", err)
			http.Error(w, "failed to decode json: "+err.Error(), http.StatusInternalServerError)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		jsonBytes, err := json.Marshal(payload)
		if err != nil {
			log.Printf("Failed to marshal payload: %v\n", err)
			http.Error(w, "failed to marshal: "+err.Error(), http.StatusInternalServerError)
			return
		}

		cmd := exec.CommandContext(ctx, "node", "script.js", string(jsonBytes))
		out, err := cmd.Output()
		if ctx.Err() == context.DeadlineExceeded {
			log.Println("Node script execution timed out")
			http.Error(w, "Node script timed out", http.StatusInternalServerError)
			return
		}
		if err != nil {
			msg := strings.Builder{}
			msg.WriteString("Node error: ")
			msg.WriteString(err.Error())
			if len(out) > 0 {
				msg.WriteString("\nOutput: ")
				msg.Write(out)
			}
			log.Printf("Command failed. Error: %v, Output: %s\n", err, string(out))
			http.Error(w, msg.String(), http.StatusInternalServerError)
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

		log.Printf("Sending response: %s\n", last)
		w.Header().Set("Content-Type", "application/json")
		containsError := strings.Contains(last, "error")
		if containsError {
			jsonStirng := fmt.Sprintf(`{"error":"%s"}`, last)
			_, _ = w.Write([]byte(jsonStirng))
		} else {
			_, _ = w.Write([]byte(last))
		}
	})
	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}
	log.Println("Server running on http://localhost:8080")
	log.Fatal(server.ListenAndServe())
}
