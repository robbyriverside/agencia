package agencia

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLoadFactsHandler(t *testing.T) {
	defaultChat = NewChat("test")
	defaultChat.Facts["old"] = "value"

	payload := map[string]any{
		"facts": map[string]any{
			"new": "fact",
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/loadfacts", bytes.NewReader(data))
	w := httptest.NewRecorder()
	LoadFactsHandler(w, req)

	res := w.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected status OK, got %v", res.Status)
	}
	if _, ok := defaultChat.Facts["old"]; ok {
		t.Fatalf("expected old facts to be replaced")
	}
	if v, ok := defaultChat.Facts["new"]; !ok || v != "fact" {
		t.Fatalf("expected new fact to be loaded, got %v", defaultChat.Facts)
	}
}
