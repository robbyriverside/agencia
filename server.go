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
)

//go:embed web/*
var embeddedFiles embed.FS

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
	webFS, err := fs.Sub(embeddedFiles, "web")
	if err != nil {
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
	log.Println("Agencia server running on :8080")
	log.Fatal(http.ListenAndServe(url, nil))
}

func handleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST supported", http.StatusMethodNotAllowed)
		return
	}
	var req runRequest
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Cannot read request body", http.StatusBadRequest)
		return
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	ctx := context.Background()

	registry, err := Compile(req.Spec)
	if err != nil {
		http.Error(w, fmt.Sprintf("[LOAD ERROR] %s", err), http.StatusBadRequest)
		return
	}
	resp := registry.Run(ctx, req.Agent, req.Input)
	// output, err := agents.RunSpec(ctx, req.Spec, req.Input, req.Agent)
	// resp := runResponse{Output: output}
	// if err != nil {
	// 	resp.Error = err.Error()
	// }
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(runResponse{Output: resp})
}
