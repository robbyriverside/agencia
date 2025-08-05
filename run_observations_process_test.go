package agencia

import (
	"context"
	"strings"
	"testing"

	"github.com/robbyriverside/agencia/agents"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runPrompt executes a prompt agent and returns its output and trace card.
func runPrompt(ctx context.Context, run *RunContext, agent *agents.Agent, input string) (string, *TraceCard, error) {
	card := run.NewTraceCard(agent.Name, input)
	if run.Card != nil {
		run.Card.BranchCards = append(run.Card.BranchCards, card)
	}
	card.PriorCard = run.Card
	run.Card = card

	result := run.execPromptAgent(ctx, agent, input, agent.Name)

	if card.PriorCard != nil {
		run.Card = card.PriorCard
	} else {
		run.Card = nil
	}
	return result.Output, card, result.Error
}

func TestPromptAgentAddsObservationTrace(t *testing.T) {
	requireAPI(t)

	defaultChat = NewChat("buddy")
	spec := `
roles:
  friend:
    description: friendly helper
    observations:
      family: family details about the user
      classes: classes the user attends
agents:
  buddy:
    role: friend
    prompt: "Respond to the user: {{ .Input }}"`
	reg, err := defaultChat.NewRegistry(spec)
	require.NoError(t, err)
	agent := reg.Agents["buddy"]
	run := NewRun(reg, defaultChat)
	ctx := context.Background()

	tests := []struct {
		input        string
		expectations []string
	}{
		{"My mother loved aprons.", []string{"apron"}},
		{"I'm taking a cooking class.", []string{"apron", "cooking"}},
	}

	for _, tt := range tests {
		_, card, err := runPrompt(ctx, run, agent, tt.input)
		require.NoError(t, err)
		lower := strings.ToLower(card.Prompt)
		for _, want := range tt.expectations {
			assert.Contains(t, lower, want, "prompt should include observation containing %q", want)
		}
	}
}

func TestObservationPromptPersonalizesResponse(t *testing.T) {
	requireAPI(t)

	defaultChat = NewChat("buddy")
	spec := `
roles:
  friend:
    description: friendly helper
    observations:
      family: family details about the user
      classes: classes the user attends
agents:
  buddy:
    role: friend
    prompt: "Respond to the user: {{ .Input }}"`
	reg, err := defaultChat.NewRegistry(spec)
	require.NoError(t, err)
	agent := reg.Agents["buddy"]
	run := NewRun(reg, defaultChat)
	ctx := context.Background()

	// Seed observations
	_, _, err = runPrompt(ctx, run, agent, "My mother loved aprons.")
	require.NoError(t, err)
	_, _, err = runPrompt(ctx, run, agent, "I'm taking a cooking class.")
	require.NoError(t, err)

	output, card, err := runPrompt(ctx, run, agent, "Any tips for my cooking class?")
	require.NoError(t, err)
	assert.Contains(t, strings.ToLower(output), "apron", "response should use stored observation")
	assert.Contains(t, strings.ToLower(card.Prompt), "apron")
}
