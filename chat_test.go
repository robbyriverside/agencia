package agencia

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/robbyriverside/agencia/agents"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessAgentMemory(t *testing.T) {
	requireAPI(t)

	// Define a simple agent with one fact
	agent := &agents.Agent{
		Name: "printer",
		Facts: map[string]*agents.Fact{
			"sheet_size": {
				Name:        "sheet_size",
				Type:        "string",
				Description: "The size of the paper",
			},
		},
	}

	// Create a registry and attach the agent
	reg := &Registry{
		Agents: map[string]*agents.Agent{
			"printer": agent,
		},
	}

	// Create a chat and bind it to the registry
	chat := NewChat("printer", reg)
	run := NewRun(reg, chat)
	// Simulate input/output for fact extraction
	input := "Please print on a small card."
	output := "Sure, I will use 3x5 card size for printing."

	// Process memory
	run.ExtractAgentMemory(context.Background(), agent, input, output)

	// Validate facts stored
	wantKey := "printer.sheet_size"
	if val, ok := chat.Facts[wantKey]; !ok {
		t.Errorf("Expected fact %s not found", wantKey)
	} else if val != "3x5" {
		t.Errorf("Expected fact value '3x5', got '%v'", val)
	}
}

// TestTemplateStartSwitch verifies that using {{ .Start "agent" }} in a template
// changes the chat's start agent for the next user message.
func TestTemplateStartSwitch(t *testing.T) {
	const spec = `
agents:
  greeter:
    description: Greets then switches to helper
    template: "Hi there! {{ .Start \"helper\" }}"
  helper:
    description: Responds after becoming the new start agent
    template: "Helper heard: {{ .Input }}"
`

	reg, err := NewRegistry(spec)
	require.NoError(t, err)
	chat := NewChat("greeter", reg)

	// First call should run greeter and change chat.Start
	out1, trace := reg.Run(context.Background(), chat, chat.StartAgent, "first")
	assert.Equal(t, "greeter", trace.AgentName, "chat start agent should helper")
	trace.SaveMarkdown("trace1.md", true)
	assert.Contains(t, out1, "Hi there!")
	assert.Equal(t, "helper", chat.StartAgent, "chat start agent should switch to helper")

	// Second call should now go to helper automatically
	out2, trace := reg.Run(context.Background(), chat, chat.StartAgent, "second")
	trace.SaveMarkdown("trace2.md", true)
	assert.Equal(t, "helper", trace.AgentName, "chat start agent should helper")
	assert.Equal(t, "Helper heard: second", strings.TrimSpace(out2))
}

func TestChatAddObservation(t *testing.T) {
	chat := NewChat("test", &Registry{})
	chat.AddObservation("writer", "preference", "prefers novels to short stories")

	obs := chat.ObservationsByRole("writer")
	require.NotNil(t, obs)
	assert.Equal(t, []string{"prefers novels to short stories"}, obs["preference"])
}

func TestChatSessionStoreResume(t *testing.T) {
	store := newChatSessionStore()
	conn1 := &websocket.Conn{}
	chat := store.startSession(conn1, "agent", &Registry{})
	require.NotNil(t, chat)
	assert.Equal(t, conn1, chat.Conn)

	store.endSession(conn1)
	assert.Nil(t, chat.Conn)

	conn2 := &websocket.Conn{}
	resumed, err := store.resumeSession(chat.ChatID, conn2)
	require.NoError(t, err)
	require.Equal(t, chat, resumed)
	assert.Equal(t, conn2, resumed.Conn)

	_, err = store.closeChat(chat.ChatID)
	require.NoError(t, err)
	_, err = store.resumeSession(chat.ChatID, &websocket.Conn{})
	require.Error(t, err)
}

func TestCloseChatHandler(t *testing.T) {
	oldStore := chatSessions
	chatSessions = newChatSessionStore()
	defer func() { chatSessions = oldStore }()

	chat := NewChat("test", &Registry{})
	chatSessions.sessions[chat.ChatID] = chat

	req := httptest.NewRequest(http.MethodPost, "/api/closechat?chat_id="+chat.ChatID, nil)
	w := httptest.NewRecorder()
	CloseChatHandler(w, req)

	res := w.Result()
	require.Equal(t, http.StatusNoContent, res.StatusCode)
	if _, ok := chatSessions.get(chat.ChatID); ok {
		t.Fatalf("expected chat to be removed from sessions")
	}
	require.True(t, chat.Closed)

	req2 := httptest.NewRequest(http.MethodPost, "/api/closechat?chat_id="+chat.ChatID, nil)
	w2 := httptest.NewRecorder()
	CloseChatHandler(w2, req2)
	require.Equal(t, http.StatusNotFound, w2.Result().StatusCode)
}
