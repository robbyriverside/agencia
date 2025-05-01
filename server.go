package agencia

import (
	"context"
	"embed"
	"encoding/json"
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
	Output string `json:"output"`
	Error  string `json:"error,omitempty"`
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

	http.HandleFunc("/api/run", handleRun)
	http.HandleFunc("/api/chat", ChatWebSocketHandler)
	http.HandleFunc("/api/facts", FactsHandler)

	log.Fatal(http.ListenAndServe(url, nil))
}

func handleRun(w http.ResponseWriter, r *http.Request) {
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

	registry, err := NewRegistry(req.Spec)
	if err != nil {
		logs.Error("[RUN ERROR] registry error: %v", err)
		http.Error(w, "[RUN ERROR]", http.StatusBadRequest)
		return
	}
	resp := registry.Run(ctx, req.Agent, req.Input)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(runResponse{Output: resp})
}
