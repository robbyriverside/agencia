package agencia

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const minimalSpec = `
agents:
  test:
    description: Simple echo agent
    template: "{{ .Input }}"
`

func TestChatClientReconnectAndClose(t *testing.T) {
	oldStore := chatSessions
	chatSessions = newChatSessionStore()
	defer func() { chatSessions = oldStore }()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/chat", ChatSessionHandler)
	mux.HandleFunc("/api/closechat", CloseChatHandler)
	mux.HandleFunc("/api/facts", FactsHandler)
	mux.HandleFunc("/api/loadfacts", LoadFactsHandler)

	server := startTestServer(t, mux)
	if server == nil {
		return
	}
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/chat"

	client := &ChatClient{
		WSURL:    wsURL,
		HTTPBase: server.URL,
		Agent:    "test",
		Spec:     minimalSpec,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, client.Connect(ctx))
	firstID := client.ChatID
	require.NotEmpty(t, firstID)

	require.NoError(t, client.Send("hello"))
	recvCtx, recvCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer recvCancel()
	msg, err := client.Receive(recvCtx)
	require.NoError(t, err)
	require.Equal(t, "test", msg["sender"])
	require.Equal(t, "hello", strings.TrimSpace(msg["message"].(string)))

	require.NoError(t, client.Disconnect())
	reconnectCtx, reconnectCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer reconnectCancel()
	require.NoError(t, client.Connect(reconnectCtx))
	require.Equal(t, firstID, client.ChatID)

	closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer closeCancel()
	require.NoError(t, client.CloseChat(closeCtx))

	if _, ok := chatSessions.get(firstID); ok {
		t.Fatalf("expected chat %s to be removed after close", firstID)
	}
}

func startTestServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	func() {
		defer func() {
			if r := recover(); r != nil {
				msg := fmt.Sprint(r)
				if strings.Contains(msg, "failed to listen") {
					t.Skipf("skipping chat client test: %s", msg)
					return
				}
				panic(r)
			}
		}()
		srv = httptest.NewServer(handler)
	}()
	return srv
}
