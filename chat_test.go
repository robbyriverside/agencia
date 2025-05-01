package agencia

import (
	"context"
	"testing"

	"github.com/robbyriverside/agencia/agents"
)

type MockRegistry struct {
	*Registry
}

func (m *MockRegistry) CallAI(ctx context.Context, agent *agents.Agent, prompt string, tmplCtx any) (string, error) {
	return "sheet_size: 3x5", nil
}

func TestProcessAgentMemory(t *testing.T) {
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
	mockReg := &MockRegistry{Registry: reg}

	// Create a chat and bind it to the registry
	chat := NewChat("printer")
	reg.Chat = chat

	// Simulate input/output for fact extraction
	input := "Please print on a small card."
	output := "Sure, I will use 3x5 card size for printing."

	// Process memory
	chat.ProcessAgentMemory(context.Background(), mockReg, agent, input, output)

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
