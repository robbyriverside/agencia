package agencia

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/robbyriverside/agencia/agents"
	"github.com/tmc/langchaingo/jsonschema"
	"github.com/tmc/langchaingo/tools"
)

// ----------------------------------------------------------------------------------
// Small self‑contained stubs so this file compiles; delete if you already have them.
// ----------------------------------------------------------------------------------
// type Agent struct {
// 	Description string
// 	InputPrompt map[string]string // field -> description
// }

// ----------------------------------------------------------------------------------

// -----------------------------------------------------------------------------
// Function‑tool plumbing (replaces the deprecated FunctionTool + friends)
// -----------------------------------------------------------------------------

// functionParameters is the minimal wrapper around jsonschema for OpenAI/Gemini.
type functionParameters struct {
	Type       string                           `json:"type"` // always "object"
	Properties map[string]jsonschema.Definition `json:"properties,omitempty"`
	Required   []string                         `json:"required,omitempty"`
}

// toolDefinition is what we’ll marshal into the system prompt:
//
//	You have access to the following tools:
//
//	[
//	  { "name": "...", "description": "...", "parameters": { … } },
//	  ...
//	]
type toolDefinition struct {
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Parameters  functionParameters `json:"parameters"`
}

// functionTool multiplexes all Agencia agents behind ONE LangChainGo Tool.
// The model calls it with:
//
//	{ "tool": "<name>", "tool_input": { ... } }
type functionTool struct {
	defs     []toolDefinition
	callback func(ctx context.Context, name string, input map[string]any) (string, error)
}

// Name/Description satisfy tools.Tool.
func (f functionTool) Name() string        { return "agencia_function_tool" }
func (f functionTool) Description() string { return "Internal dispatcher for Agencia agents" }

// Call parses the JSON wrapper and forwards to the right agent.
func (f functionTool) Call(ctx context.Context, input string) (string, error) {
	var payload struct {
		Tool      string         `json:"tool"`
		ToolInput map[string]any `json:"tool_input"`
	}
	if err := json.Unmarshal([]byte(input), &payload); err != nil {
		return "", fmt.Errorf("tool dispatcher: invalid JSON: %w", err)
	}
	return f.callback(ctx, payload.Tool, payload.ToolInput)
}

// -----------------------------------------------------------------------------
// Public helper – exactly what you wanted AgentsToTool to do.
// -----------------------------------------------------------------------------
func AgentsToTool(
	agents map[string]agents.Agent,
	callFunc func(context.Context, agents.Agent, map[string]any) (string, error),
) tools.Tool { // NOTE: returns the standard Tool interface
	defs := make([]toolDefinition, 0, len(agents))

	for name, agent := range agents {
		if agent.Description == "" {
			continue // skip hidden agents
		}

		// Build JSON‑schema for each agent’s parameters.
		props := make(map[string]jsonschema.Definition)
		req := make([]string, 0, len(agent.InputPrompt))

		for field, desc := range agent.InputPrompt {
			props[field] = jsonschema.Definition{
				Type:        jsonschema.String,
				Description: desc,
			}
			req = append(req, field)
		}

		defs = append(defs, toolDefinition{
			Name:        name,
			Description: agent.Description,
			Parameters: functionParameters{
				Type:       "object",
				Properties: props,
				Required:   req,
			},
		})
	}

	// Return one composite tool that LangChainGo can register.
	return functionTool{
		defs: defs,
		callback: func(ctx context.Context, name string, input map[string]any) (string, error) {
			agent, ok := agents[name]
			if !ok {
				return "", fmt.Errorf("unknown agent %q", name)
			}
			return callFunc(ctx, agent, input)
		},
	}
}
