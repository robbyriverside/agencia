package agencia

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParley_Start verifies that the START directive updates the chat session's StartAgent.
func TestParley_Start(t *testing.T) {
	const spec = `
agents:
  switcher:
    description: Switches to target agent
    template: "{{ START target }}"
  target:
    description: The target agent
    template: "Hello Target"
`
	reg, err := NewRegistry(spec)
	require.NoError(t, err)

	chat := NewChat("switcher", reg)

	// 1. Run switcher
	out, _ := reg.Run(context.Background(), chat, "switcher", "")
	assert.Equal(t, "", strings.TrimSpace(out), "START should produce no output")

	// 2. Verify StartAgent updated
	assert.Equal(t, "target", chat.StartAgent, "StartAgent should be updated to 'target'")

	// 3. Run the new start agent (simulate next turn)
	out2, _ := reg.Run(context.Background(), chat, chat.StartAgent, "")
	assert.Equal(t, "Hello Target", out2)
}
