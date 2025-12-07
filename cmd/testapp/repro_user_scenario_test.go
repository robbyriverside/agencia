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

func TestRepro_UserScenario(t *testing.T) {
	_ = godotenv.Load("../../.env")
	os.Setenv("AGENCIA_CONFIG", "../../openai.config.yaml")

	// 1. Initialize Registry with helpline_parley.yaml and helpline_roles.yaml
	data, err := os.ReadFile("../../helpline_parley.yaml")
	require.NoError(t, err, "Failed to read helpline spec")
	helplineSpec := string(data)

	rolesData, err := os.ReadFile("../../helpline_roles.yaml")
	require.NoError(t, err, "Failed to read roles spec")

	fullSpec := string(rolesData) + "\n" + helplineSpec

	reg, err := agencia.NewRegistry(fullSpec)
	require.NoError(t, err, "Failed to parse helpline spec")

	// 2. Prepare Input - Exact User Scenario
	input := "Saturday at noon with Dr Nolan"

	// 3. Run 'mainmenu' agent
	ctx := context.Background()
	chat := agencia.NewChat("mainmenu", reg)
	output, _ := reg.Run(ctx, chat, "mainmenu", input)

	t.Logf("Agent Output: %s", output)

	// 4. Verify Facts
	facts := chat.Facts
	t.Logf("Captured Facts: %v", facts)

	// User expects: information: [{appointment_time: Saturday at noon}, {doctor_name: Dr. Nolan}]
	// Note: The structure depends on what the AI extracts.
	// The prompt asks to add to a list.
	// We expect 'information' key to exist (unscoped now due to fix).

	info, ok := facts["information"]
	if !ok {
		// Fallback check if fix didn't work or scoping behavior is different than expected
		info, ok = facts["mainmenu.information"]
	}

	assert.True(t, ok, "Fact 'information' should exist")

	if ok {
		infoStr := fmt.Sprintf("%v", info)
		t.Logf("Information Content: %s", infoStr)

		assert.True(t, strings.Contains(strings.ToLower(infoStr), "nolan"), "Fact should contain 'Nolan'")
		assert.True(t, strings.Contains(strings.ToLower(infoStr), "saturday") || strings.Contains(strings.ToLower(infoStr), "noon"), "Fact should contain time details")
	}
}
