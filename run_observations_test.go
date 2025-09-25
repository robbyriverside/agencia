package agencia

import (
	"context"
	"testing"

	"github.com/robbyriverside/agencia/agents"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessObservations(t *testing.T) {
	calls := 0
	observationAICall = func(ctx context.Context, prompt string) (string, error) {
		calls++
		if calls == 1 {
			return `{"writing_pref": ["likes novels"]}`, nil
		}
		return `{"writing_pref": ["likes poetry", "likes novels"]}`, nil
	}
	defer func() { observationAICall = agents.CallOpenAI }()

	reg := &Registry{Roles: map[string]*agents.AgentRole{
		"writer": {ID: "writer", Name: "writer", Observations: map[string]string{"writing_pref": "writing preferences"}},
	}}
	chat := NewChat("a", reg)
	chat.AddObservation("writer", "writing_pref", "likes poetry")
	run := NewRun(reg, chat)

	agent := &agents.Agent{Name: "a", Prompt: "Say hi", Role: "writer"}

	sub, err := run.processObservations(context.Background(), agent, "I enjoy long novels")
	require.NoError(t, err)
	obs := chat.ObservationsByRole("writer")
	require.Len(t, obs["writing_pref"], 2)
	assert.Contains(t, sub, "likes poetry")
	assert.Contains(t, sub, "likes novels")
}

func TestGatherObservationsBadJSON(t *testing.T) {
	observationAICall = func(ctx context.Context, prompt string) (string, error) {
		return "not json", nil
	}
	defer func() { observationAICall = agents.CallOpenAI }()

	run := &RunContext{}
	role := &agents.AgentRole{ID: "writer", Name: "writer", Observations: map[string]string{"writing_pref": "prefs"}}
	_, err := run.gatherObservationsFromInput(context.Background(), role, "I like novels")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not json")
}

func TestMergeObservationsBadJSON(t *testing.T) {
	observationAICall = func(ctx context.Context, prompt string) (string, error) {
		return "{oops", nil
	}
	defer func() { observationAICall = agents.CallOpenAI }()

	run := &RunContext{}
	existing := map[string][]string{"writing_pref": []string{"likes poetry"}}
	incoming := map[string][]string{"writing_pref": []string{"likes novels"}}
	_, err := run.mergeObservations(context.Background(), existing, incoming)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "{oops")
}
