package time_lib

import (
	"context"
	"time"

	"github.com/robbyriverside/agencia/agents"
)

var Agents = map[string]*agents.Agent{
	"now": {
		Description: "Returns the current time. Optional input 'format' specifies the Go time format string.",
		Inputs: map[string]*agents.Argument{
			"format": {
				Description: "Optional Go time format string (default: RFC3339)",
				Type:        "string",
				Required:    false,
			},
		},
		Function: func(ctx context.Context, input map[string]any, agent *agents.Agent) (string, error) {
			format := time.RFC3339
			if f, ok := input["format"].(string); ok && f != "" {
				format = f
			} else if f, ok := input["input"].(string); ok && f != "" {
				// Allow "INPUT <format>" directly
				format = f
			}
			return time.Now().Format(format), nil
		},
	},
	"date": {
		Description: "Returns the current date (YYYY-MM-DD).",
		Inputs:      map[string]*agents.Argument{},
		Function: func(ctx context.Context, input map[string]any, agent *agents.Agent) (string, error) {
			return time.Now().Format("2006-01-02"), nil
		},
	},
	"clock": {
		Description: "Returns the current time (HH:MM:SS).",
		Inputs:      map[string]*agents.Argument{},
		Function: func(ctx context.Context, input map[string]any, agent *agents.Agent) (string, error) {
			return time.Now().Format("15:04:05"), nil
		},
	},
}
