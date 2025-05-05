package agencia

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/joho/godotenv"
	"github.com/robbyriverside/agencia/agents"
	"github.com/stretchr/testify/assert"
)

// Run tests multiple times to ensure the AI consistently fills in the required fields
const loopCount = 10

func TestFunctionAgentWithInputs(t *testing.T) {
	ctx := context.Background()
	_ = godotenv.Load()

	if os.Getenv("OPENAI_API_KEY") == "" {
		t.Fatal("OPENAI_API_KEY must be set (either in environment or .env file)")
	}

	// Agent with InputPrompt including required and optional fields
	agent := &agents.Agent{
		Name:        "test_func",
		Description: "Test function agent with input prompt",
		Function: func(ctx context.Context, input map[string]any) (string, error) {
			a, _ := input["a"].(string)
			b, _ := input["b"].(string)
			return a + "|" + b, nil
		},
		Inputs: map[string]*agents.Argument{
			"a": {
				Name:        "a",
				Description: "Required field A",
				Type:        "string",
				Required:    true,
			},
			"b": {
				Name:        "b",
				Description: "Optional field B",
				Type:        "string",
				Required:    false,
			},
		},
	}

	reg := &Registry{
		Agents: map[string]*agents.Agent{
			"test_func": agent,
		},
	}

	t.Run("missing required value", func(t *testing.T) {
		for i := 0; i < loopCount; i++ {
			t.Run(fmt.Sprintf("run %d", i), func(t *testing.T) {
				res := NewRun(reg, nil).CallAgent(ctx, "test_func", "b: optional\n")
				t.Logf("Run %d: %+v", i, res.Error)
				assert.False(t, res.Ran)
				if assert.Error(t, res.Error) {
					assert.Contains(t, res.Error.Error(), "a")
				}
			})
		}
	})

	t.Run("missing optional value", func(t *testing.T) {
		for i := 0; i < loopCount; i++ {
			t.Run(fmt.Sprintf("run %d", i), func(t *testing.T) {
				res := NewRun(reg, nil).CallAgent(ctx, "test_func", "a: hello\n")
				t.Logf("Run %d: %+v", i, res.Error)
				assert.True(t, res.Ran)
				if assert.NoError(t, res.Error) {
					assert.NotEmpty(t, res.Output)
				}
			})
		}
	})
}
