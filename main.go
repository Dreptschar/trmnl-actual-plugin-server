package main

import (
	"context"
	"encoding/json"
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
		// payload := `{"serverURL":"https://actual.dreptschar.com","serverPassword":"3HQ*AiiD-qrkVJF","budgetSyncId":"d0423922-2037-4479-ba30-35e0289fe2c8","budgetEncryptionPassword":"WNCsGQZ7V.3*Gb@EeXg4","groupName":"Flexi","included":"🛒 Lebensmittel,🍽️ Restaurants,💩 Budget,🎢 Aktivitäten"}`

		var payload Payload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "failed to encode json: "+err.Error(), http.StatusInternalServerError)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		jsonBytes, err := json.Marshal(payload)
		if err != nil {
			http.Error(w, "failed to marshal: "+err.Error(), http.StatusInternalServerError)
			return
		}
		cmd := exec.CommandContext(ctx, "node", "script.js", string(jsonBytes))

		out, err := cmd.Output()
		if ctx.Err() == context.DeadlineExceeded {
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

		w.Header().Set("Content-Type", "application/json")
		_,_ = w.Write([]byte(last))
	})
	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}
	log.Println("Server running on http://localhost:8080")
	log.Fatal(server.ListenAndServe())
}
