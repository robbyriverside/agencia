package text_lib

import (
	"context"

	"github.com/google/uuid"
	"github.com/robbyriverside/agencia/agents"
)

var Agents = map[string]*agents.Agent{
	"uuid": {
		Description: "Generates a new UUID.",
		Inputs:      map[string]*agents.Argument{},
		Function: func(ctx context.Context, input map[string]any, agent *agents.Agent) (string, error) {
			return uuid.New().String(), nil
		},
	},
}
