package agencia

import (
	"context"
	"fmt"
	"testing"

	"github.com/robbyriverside/agencia/agents"
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

	tests := []testPair{
		{"My mother loved cooking. She always wore the most amazing apron.", []string{"apron", "aprons"}},
		{"I'm taking a cooking class.", []string{"apron", "aprons", "cooking"}},
	}
	execAgenciaTests(t, "obs", spec, tests)
}

type testPair struct {
	input  string
	checks []string
}

func execAgenciaTests(t *testing.T, traceName, spec string, tests []testPair) {
	reg, err := defaultChat.NewRegistry(spec)
	require.NoError(t, err)

	if defaultChat == nil {
		defaultChat = NewChat("helpline")
	}
	require.NoError(t, err)

	for i, test := range tests {
		// First call should run greeter and change chat.Start
		out1, trace := reg.Run(context.Background(), defaultChat.StartAgent, test.input)
		require.NoError(t, trace.Error)
		t.Log(trace.SaveMarkdown(fmt.Sprintf("trace_%s%d.md", traceName, i)))

		//assert.Contains(t, out1, test.output)
		t.Logf("Input: %s", test.input)
		t.Logf("Result: %s", out1)
		t.Logf("Compare: %s\n", test.checks)
	}
}

// func TestObservationPromptPersonalizesResponse(t *testing.T) {
// 	requireAPI(t)

// 	defaultChat = NewChat("buddy")
// 	spec := `
// roles:
//   friend:
//     description: friendly helper
//     observations:
//       family: family details about the user
//       classes: classes the user attends
// agents:
//   buddy:
//     role: friend
//     prompt: "Respond to the user: {{ .Input }}"`
// 	reg, err := defaultChat.NewRegistry(spec)
// 	require.NoError(t, err)
// 	agent := reg.Agents["buddy"]
// 	run := NewRun(reg, defaultChat)
// 	ctx := context.Background()

// 	// Seed observations
// 	_, _, err = runPrompt(ctx, run, agent, "My mother loved cooking. She always wore the most amazing apron.")
// 	require.NoError(t, err)
// 	_, _, err = runPrompt(ctx, run, agent, "I'm taking a cooking class.")
// 	require.NoError(t, err)

// 	output, card, err := runPrompt(ctx, run, agent, "Any tips for my cooking class?")
// 	t.Logf("Output: %s", output)
// 	require.NoError(t, err)
// 	assert.Contains(t, strings.ToLower(output), "aprons", "response should use stored observation")
// 	assert.Contains(t, strings.ToLower(card.Prompt), "aprons")
// }
