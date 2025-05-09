package agencia

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/joho/godotenv"
	"github.com/robbyriverside/agencia/agents"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessAgentMemory(t *testing.T) {
	// ctx := context.Background()
	_ = godotenv.Load()

	if os.Getenv("OPENAI_API_KEY") == "" {
		t.Fatal("OPENAI_API_KEY must be set (either in environment or .env file)")
	}
	// Define a simple agent with one fact
	agent := &agents.Agent{
		Name: "printer",
		Facts: map[string]*agents.Fact{
			"sheet_size": {
				Name:        "sheet_size",
				Type:        "string",
				Description: "The size of the paper",
				Tags:        []string{"cards"},
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
	chat := NewChat("printer")
	reg.Chat = chat
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

	// Validate tagging
	tagged := false
	for _, key := range chat.TaggedFacts["cards"] {
		if key == wantKey {
			tagged = true
			break
		}
	}
	if !tagged {
		t.Errorf("Expected fact key %s to be tagged with 'cards'", wantKey)
	}
}

// TestTemplateStartSwitch verifies that using {{ .Start "agent" }} in a template
// changes the chat's start agent for the next user message.
func TestTemplateStartSwitch(t *testing.T) {
	requireAPI(t)

	const spec = `
agents:
  greeter:
    description: Greets then switches to helper
    template: "Hi there! {{ .Start \"helper\" }}"
  helper:
    description: Responds after becoming the new start agent
    template: "Helper heard: {{ .Input }}"
`

	// Chat starts with 'greeter'
	if defaultChat == nil {
		defaultChat = NewChat("greeter")
	}
	reg, err := defaultChat.NewRegistry(spec)
	require.NoError(t, err)

	// First call should run greeter and change chat.Start
	out1, trace := reg.Run(context.Background(), defaultChat.StartAgent, "first")
	assert.Equal(t, "greeter", trace.AgentName, "chat start agent should helper")
	trace.SaveMarkdown("trace1.md", true)
	assert.Contains(t, out1, "Hi there!")
	assert.Equal(t, "helper", defaultChat.StartAgent, "chat start agent should switch to helper")

	// Second call should now go to helper automatically
	out2, trace := reg.Run(context.Background(), defaultChat.StartAgent, "second")
	trace.SaveMarkdown("trace2.md", true)
	assert.Equal(t, "helper", trace.AgentName, "chat start agent should helper")
	assert.Equal(t, "Helper heard: second", strings.TrimSpace(out2))
}
