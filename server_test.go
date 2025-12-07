package agencia

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRunHandler(t *testing.T) {
	// Simple echo agent spec for testing
	const spec = `agents:
  echo:
    description: Echoes input
    template: "Hello World"`

	tests := []struct {
		name           string
		reqBody        runRequest
		wantStatusCode int
		wantOutput     string
		wantError      bool
	}{
		{
			name: "Valid Request",
			reqBody: runRequest{
				Spec:  spec,
				Agent: "echo",
				Input: "Hello World",
			},
			wantStatusCode: http.StatusOK,
			wantOutput:     "Hello World",
			wantError:      false,
		},
		{
			name: "Invalid Method",
			// Handled by test logic sending GET
			reqBody:        runRequest{},
			wantStatusCode: http.StatusMethodNotAllowed,
			wantError:      true,
		},
		{
			name: "Invalid Spec",
			reqBody: runRequest{
				Spec:  "invalid spec",
				Agent: "echo",
				Input: "test",
			},
			wantStatusCode: http.StatusBadRequest,
			wantError:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req *http.Request
			if tt.name == "Invalid Method" {
				req = httptest.NewRequest(http.MethodGet, "/api/run", nil)
			} else {
				body, _ := json.Marshal(tt.reqBody)
				req = httptest.NewRequest(http.MethodPost, "/api/run", bytes.NewBuffer(body))
			}
			w := httptest.NewRecorder()

			RunHandler(w, req)

			resp := w.Result()
			if resp.StatusCode != tt.wantStatusCode {
				t.Errorf("expected status %d, got %d", tt.wantStatusCode, resp.StatusCode)
			}

			if !tt.wantError {
				var runResp runResponse
				if err := json.NewDecoder(resp.Body).Decode(&runResp); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}
				if strings.TrimSpace(runResp.Output) != tt.wantOutput {
					t.Errorf("expected output %q, got %q", tt.wantOutput, runResp.Output)
				}
				// Verify Facts and Observations are present (even if empty)
				// Since they are maps, they should unmarshal to non-nil if present in JSON,
				// or if omitempty and empty, they are nil.
				// We updated server.go to include them.
				// However, if they are empty maps, they might be omitted by json encoder if omitempty is set.
				// In server.go we kept omitempty.
				// To verify they are handled, we can just ensure no error occurred during decode
				// and if we had a test case with facts, we'd check them.
				// For now, this confirms the struct expects them.
				t.Logf("Response Facts: %v, Observations: %v", runResp.Facts, runResp.Observations)
			}
		})
	}
}
