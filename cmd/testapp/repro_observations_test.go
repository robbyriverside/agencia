package main

import (
	"context"
	"os"
	"testing"

	"github.com/joho/godotenv"
	"github.com/robbyriverside/agencia"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepro_Observations(t *testing.T) {
	_ = godotenv.Load("../../.env")
	os.Setenv("AGENCIA_CONFIG", "../../openai.config.yaml")

	// 1. Load helpline_parley.yaml (which now includes roles)
	data, err := os.ReadFile("../../helpline_parley.yaml")
	require.NoError(t, err, "Failed to read helpline spec")

	// Note: previous tests concatenated files, but user said they merged them.
	// The current file content should be self-contained if user edits were saved.

	reg, err := agencia.NewRegistry(string(data))
	require.NoError(t, err, "Failed to parse registry")

	// 2. Prepare Input
	input := "This is very frustrating. My doctor is not answering my calls. He is in the city somewhere. Springfield maybe"

	// 3. Run 'mainmenu'
	ctx := context.Background()
	chat := agencia.NewChat("mainmenu", reg)
	output, _ := reg.Run(ctx, chat, "mainmenu", input)

	t.Logf("Agent Output: %s", output)

	// 4. Verify Observations
	// We expect role 'cognitive_evaluator' to have 'state_of_mind'.
	obsMap := chat.Observations
	t.Logf("Captured Observations: %v", obsMap)

	// Check if any observations exist
	hasObservations := false
	if roleObs, ok := obsMap["cognitive_evaluator"]; ok {
		if val, ok := roleObs["state_of_mind"]; ok && len(val) > 0 {
			hasObservations = true
			t.Logf("Found state_of_mind: %v", val)
		}
	}

	assert.True(t, hasObservations, "Should have captured state_of_mind observation")
}
