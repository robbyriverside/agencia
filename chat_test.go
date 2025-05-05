package agencia

import (
	"context"
	"os"
	"testing"

	"github.com/joho/godotenv"
	"github.com/robbyriverside/agencia/agents"
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
