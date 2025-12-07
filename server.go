package agencia

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"

	"github.com/robbyriverside/agencia/logs"
)

//go:embed web/*
var website embed.FS

type runRequest struct {
	Spec  string `json:"spec"`
	Input string `json:"input"`
	Agent string `json:"agent"`
}

type runResponse struct {
	Output       string                         `json:"output"`
	Error        string                         `json:"error,omitempty"`
	Facts        map[string]any                 `json:"facts,omitempty"`
	Observations map[string]map[string][]string `json:"observations,omitempty"`
}

func Server(ctx context.Context, url string) {
	webFS, err := fs.Sub(website, "web")
	if err != nil {
		logs.Error("[SERVER ERROR] Failed to locate embedded web directory: %v", err)
		log.Fatalf("Failed to locate embedded web directory: %v", err)
	}
	fileServer := http.FileServer(http.FS(webFS))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			w.Header().Set("Cache-Control", "no-store")
		}
		fileServer.ServeHTTP(w, r)
	})

	http.HandleFunc("/api/run", RunHandler)
	http.HandleFunc("/api/chat", ChatSessionHandler) // ChatWebSocketHandler)
	http.HandleFunc("/api/closechat", CloseChatHandler)
	http.HandleFunc("/api/facts", FactsHandler)
	http.HandleFunc("/api/loadfacts", LoadFactsHandler)

	log.Fatal(http.ListenAndServe(url, nil))
}

func RunHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		logs.Error("[RUN ERROR] Only POST supported")
		http.Error(w, "Only POST supported", http.StatusMethodNotAllowed)
		return
	}
	var req runRequest
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logs.Error("[RUN ERROR] Cannot read request body: %v", err)
		http.Error(w, "Cannot read request body", http.StatusBadRequest)
		return
	}
	if err := json.Unmarshal(body, &req); err != nil {
		logs.Error("[RUN ERROR] Invalid request body: %v", err)
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	ctx := context.Background()

	res := LintSpecFile([]byte(req.Spec))
	if !res.Valid {
		logs.Error("[RUN ERROR] Invalid spec: %s", res.Result())
		http.Error(w, fmt.Sprintf("Invalid SPEC: %s", res.Result()), http.StatusBadRequest)
		return
	}

	registry, err := NewRegistry(req.Spec)
	if err != nil {
		logs.Error("[RUN ERROR] registry error: %v", err)
		http.Error(w, "[RUN ERROR] registry creation failed", http.StatusBadRequest)
		return
	}

	// Create a chat session to capture facts and observations
	chat := NewChat(req.Agent, registry)
	resp, _ := registry.Run(ctx, chat, req.Agent, req.Input)
	logs.Info("[RUN OUTPUT]", resp)

	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(runResponse{
		Output:       resp,
		Facts:        chat.Facts,
		Observations: chat.Observations,
	})
	if err != nil {
		logs.Error("[RUN ERROR] Failed to encode response: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}
