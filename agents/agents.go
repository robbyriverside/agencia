package agents

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"text/template"
)

type AgentFn func(ctx context.Context, input map[string]any) (string, error)

type Agent struct {
	Name        string
	Description string
	InputPrompt map[string]string // field -> description
	Prompt      string
	Template    string
	Function    AgentFn
}

type AgentResult struct {
	Output    string
	Ran       bool
	Error     error
	AgentName string
}

func (r Registry) Run(ctx context.Context, name string, input string) string {
	res := r.CallAgent(ctx, name, input)
	if res.Error != nil {
		return fmt.Sprintf("[AGENT ERROR] %v", res.Error)
	}
	if !res.Ran {
		return fmt.Sprintf("[INFO] Agent '%s' did not run (empty or skipped).", res.AgentName)
	}
	return res.Output
}

func (r Registry) RunPrint(ctx context.Context, name string, input string) error {
	res := r.CallAgent(ctx, name, input)
	if res.Error != nil {
		return fmt.Errorf("[AGENT ERROR] %v\n", res.Error)
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
	Input    string
	Registry Registry
	ctx      context.Context
}

func (t *TemplateContext) Get(name string, optionalInput ...string) string {
	input := t.Input
	if len(optionalInput) > 0 {
		input = optionalInput[0]
	}
	res := t.Registry.CallAgent(t.ctx, name, input)
	if res.Error != nil {
		return fmt.Sprintf("[error calling %s: %v]", name, res.Error)
	}
	return res.Output
}

func (r Registry) CallAgent(ctx context.Context, name string, input string) AgentResult {
	agent, ok := r[name]
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
	err = tmpl.Execute(&buf, &TemplateContext{Input: input, ctx: ctx, Registry: r})
	if err != nil {
		return AgentResult{Ran: false, Error: fmt.Errorf("template exec error: %w", err), AgentName: name}
	}
	finalPrompt := strings.TrimSpace(buf.String())
	// fmt.Printf("------ [Formatter: %s]\nPrompt:\n%s\n---\n", name, finalPrompt)

	if agent.Template != "" {
		return AgentResult{Output: finalPrompt, Ran: true, AgentName: name}
	} else if agent.Function != nil {
		return AgentResult{
			Output:    fmt.Sprintf("[called external %s with input: %s]", name, input),
			Ran:       true,
			AgentName: name,
		}
	} else if agent.Prompt != "" {
		if finalPrompt == "" {
			return AgentResult{Ran: false, Output: "", AgentName: name}
		}
		resp, err := callAI(ctx, name, finalPrompt, &TemplateContext{Input: input, ctx: ctx, Registry: r})
		if err != nil {
			return AgentResult{Ran: true, Error: err, AgentName: name}
		}
		return AgentResult{Output: resp, Ran: true, AgentName: name}
	}
	return AgentResult{Ran: false, Error: errors.New("no prompt or function"), AgentName: name}
}
