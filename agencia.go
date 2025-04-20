package agencia

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"text/template"
)

type Agent struct {
	Name        string
	Description string
	Prompt      string
	Template    string
	Function    string
}

type AgentResult struct {
	Output    string
	Ran       bool
	Error     error
	AgentName string
}

var agentRegistry = map[string]Agent{}

func Run(ctx context.Context, name string, input string) error {
	res := CallAgent(ctx, name, input)
	if res.Error != nil {
		return fmt.Errorf("[AGENT ERROR] %v", res.Error)
	}
	if !res.Ran {
		fmt.Printf("[INFO] Agent '%s' did not run (empty or skipped).\n", res.AgentName)
		return nil
	}
	// fmt.Printf("[Agent: %s]\nOutput:\n%s\n", res.AgentName, res.Output)
	fmt.Println(res.Output)
	return nil
}

type TemplateContext struct {
	Input string
	ctx   context.Context
}

func (t *TemplateContext) Get(name string, optionalInput ...string) string {
	input := t.Input
	if len(optionalInput) > 0 {
		input = optionalInput[0]
	}
	res := CallAgent(t.ctx, name, input)
	if res.Error != nil {
		return fmt.Sprintf("[error calling %s: %v]", name, res.Error)
	}
	return res.Output
}

func CallAgent(ctx context.Context, name string, input string) AgentResult {
	agent, ok := agentRegistry[name]
	if !ok {
		return AgentResult{Ran: false, Error: fmt.Errorf("agent not found: %s", name), AgentName: name}
	}

	prompt := agent.Prompt
	if agent.Template != "" {
		prompt = agent.Template
	}
	// fmt.Printf("------ [Formatter: %s]\nPrompt:\n%s\n---\n", name, agent.Prompt)
	tmpl, err := template.New(name).Parse(prompt)
	if err != nil {
		return AgentResult{Ran: false, Error: fmt.Errorf("template parse error: %w", err), AgentName: name}
	}
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, &TemplateContext{Input: input, ctx: ctx})
	if err != nil {
		return AgentResult{Ran: false, Error: fmt.Errorf("template exec error: %w", err), AgentName: name}
	}
	finalPrompt := strings.TrimSpace(buf.String())
	// fmt.Printf("------ [Formatter: %s]\nPrompt:\n%s\n---\n", name, finalPrompt)

	if agent.Template != "" {
		return AgentResult{Output: finalPrompt, Ran: true, AgentName: name}
	} else if agent.Function != "" {
		return AgentResult{
			Output:    fmt.Sprintf("[called external %s with input: %s]", agent.Function, input),
			Ran:       true,
			AgentName: name,
		}
	} else if agent.Prompt != "" {
		if finalPrompt == "" {
			return AgentResult{Ran: false, Output: "", AgentName: name}
		}
		resp, err := callAI(ctx, name, finalPrompt, &TemplateContext{Input: input, ctx: ctx})
		if err != nil {
			return AgentResult{Ran: true, Error: err, AgentName: name}
		}
		return AgentResult{Output: resp, Ran: true, AgentName: name}
	}
	return AgentResult{Ran: false, Error: errors.New("no prompt or function"), AgentName: name}
}
