package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/joho/godotenv"
	"github.com/robbyriverside/agencia"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRepro_MissingFacts(t *testing.T) {
	_ = godotenv.Load("../../.env")
	os.Setenv("AGENCIA_CONFIG", "../../openai.config.yaml")

	// 1. Initialize Registry with helpline_parley.yaml and helpline_roles.yaml
	data, err := os.ReadFile("../../helpline_parley.yaml")
	require.NoError(t, err, "Failed to read helpline spec")
	helplineSpec := string(data)

	rolesData, err := os.ReadFile("../../helpline_roles.yaml")
	require.NoError(t, err, "Failed to read roles spec")

	// Combine specs or load them appropriately.
	// Agencia's NewRegistry takes a single string.
	// We can concatenate them if they are both valid YAML parts associated with 'agents' and 'roles' keys respectively or merged.
	// helpline_parley.yaml has 'agents:' key.
	// helpline_roles.yaml likely has 'roles:' key.
	fullSpec := string(rolesData) + "\n" + helplineSpec

	reg, err := agencia.NewRegistry(fullSpec)
	require.NoError(t, err, "Failed to parse helpline spec")

	// 2. Prepare Input
	// User provides information about a doctor appointment.
	input := "I want to see Dr. Smith on Monday regarding my headaches."

	// 3. Run 'mainmenu' agent
	ctx := context.Background()
	// Create a chat session manually as Server does
	chat := agencia.NewChat("mainmenu", reg)
	output, _ := reg.Run(ctx, chat, "mainmenu", input)

	t.Logf("Agent Output: %s", output)

	// 4. Verify Facts
	// Expect 'information' fact to contain something about doctor or headaches
	facts := chat.Facts
	t.Logf("Captured Facts: %v", facts)

	// Facts are scoped by agent name, BUT standard expectation is global access if not local.
	// We want to verify if we can get it as "information" or if we must use "mainmenu.information".
	// The user complains of empty facts, likely because they expect "information".
	// We assert for "information" to drive the fix to respect Global scope.
	info, ok := facts["information"]
	assert.True(t, ok, "Fact 'information' should exist (unscoped)")

	if ok {
		infoStr := fmt.Sprintf("%v", info)
		assert.True(t, strings.Contains(strings.ToLower(infoStr), "doctor") || strings.Contains(strings.ToLower(infoStr), "smith") || strings.Contains(strings.ToLower(infoStr), "headaches"), "Fact 'information' should contain captured details")
	}
}
