package mcp_lib

import (
	"context"
	"fmt"

	"github.com/robbyriverside/agencia/agents"
)

var Agents = map[string]*agents.Agent{
	"call_tool": {
		Description: "Calls an MCP tool (stub).",
		Inputs: map[string]*agents.Argument{
			"server": {
				Description: "The MCP server name.",
				Type:        "string",
				Required:    true,
			},
			"tool": {
				Description: "The tool name.",
				Type:        "string",
				Required:    true,
			},
			"arguments": {
				Description: "JSON arguments for the tool.",
				Type:        "string",
				Required:    false,
			},
		},
		Function: func(ctx context.Context, input map[string]any, agent *agents.Agent) (string, error) {
			server, _ := input["server"].(string)
			tool, _ := input["tool"].(string)
			// Stub implementation
			if server == "" || tool == "" {
				return "", fmt.Errorf("server and tool are required")
			}
			return fmt.Sprintf("Called MCP tool %s on server %s. (Stub)", tool, server), nil
		},
	},
}
