package web_lib

import (
	"context"
	"fmt"

	"github.com/robbyriverside/agencia/agents"
)

var Agents = map[string]*agents.Agent{
	"search": {
		Description: "Performs a web search (mocked).",
		Inputs: map[string]*agents.Argument{
			"query": {
				Description: "The search query.",
				Type:        "string",
				Required:    true,
			},
		},
		Function: func(ctx context.Context, input map[string]any, agent *agents.Agent) (string, error) {
			query, ok := input["query"].(string)
			if !ok {
				return "", fmt.Errorf("missing query")
			}
			// Mock response
			return fmt.Sprintf("Search results for '%s':\n- Result 1: Example page for %s\n- Result 2: Wikipedia entry for %s", query, query, query), nil
		},
	},
	"fetch": {
		Description: "Fetches content from a URL (mocked).",
		Inputs: map[string]*agents.Argument{
			"url": {
				Description: "The URL to fetch.",
				Type:        "string",
				Required:    true,
			},
		},
		Function: func(ctx context.Context, input map[string]any, agent *agents.Agent) (string, error) {
			url, ok := input["url"].(string)
			if !ok {
				return "", fmt.Errorf("missing url")
			}
			return fmt.Sprintf("Content from %s: This is mocked content.", url), nil
		},
	},
}
