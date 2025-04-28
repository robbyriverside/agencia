package agencia

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/robbyriverside/agencia/agents"
	"github.com/robbyriverside/agencia/lib/rag"
	"github.com/robbyriverside/agencia/logs"
	"github.com/robbyriverside/agencia/utils"
	"gopkg.in/yaml.v3"
)

type AgentNotFoundError struct {
	AgentName string
}

func (e *AgentNotFoundError) Error() string {
	return fmt.Sprintf("could not find agent: %s", e.AgentName)
}

type Libraries map[string]Registry

type Registry map[string]*agents.Agent

var libraries Libraries = map[string]Registry{
	"rag": rag.Agents,
}

// LookupAgent resolves both unqualified and qualified agent names
func (r *Registry) LookupAgent(name string) (*agents.Agent, error) {
	if !strings.Contains(name, ".") {
		agent, ok := (*r)[name]
		if !ok {
			return nil, &AgentNotFoundError{AgentName: name}
		}
		return agent, nil
	}

	parts := strings.SplitN(name, ".", 2)
	pkgName, agentName := parts[0], parts[1]
	pkg, ok := libraries[pkgName]
	if !ok {
		return nil, &AgentNotFoundError{AgentName: name}
	}
	agent, ok := pkg[agentName]
	if !ok {
		return nil, &AgentNotFoundError{AgentName: name}
	}
	return agent, nil
}

func (r *Registry) RegisterAgent(agent *agents.Agent) {
	(*r)[agent.Name] = agent
}

func (r *Registry) Run(ctx context.Context, name string, input string) string {
	res := r.CallAgent(ctx, name, input)
	if res.Error != nil {
		logs.Error("[AGENT ERROR] %v", res.Error)
		return ""
	}
	if !res.Ran {
		logs.Info("[INFO] Agent '%s' did not run (skipped).", res.AgentName)
		return ""
	}
	out := res.Output
	if !utf8.ValidString(out) {
		out = strings.ToValidUTF8(out, "�")
	}
	return out
}

func (r *Registry) RunPrint(ctx context.Context, name string, input string) error {
	res := r.CallAgent(ctx, name, input)
	if res.Error != nil {
		return fmt.Errorf("[AGENT ERROR] %v", res.Error)
	}
	if !res.Ran {
		fmt.Printf("[INFO] Agent '%s' did not run (skipped).\n", res.AgentName)
		return nil
	}
	out := res.Output
	if !utf8.ValidString(out) {
		out = strings.ToValidUTF8(out, "�")
	}
	fmt.Println(out)
	return nil
}

func (r *Registry) CallAgent(ctx context.Context, name string, input string) AgentResult {
	agent, err := r.LookupAgent(name)
	if err != nil {
		return AgentResult{Ran: false, Error: err, AgentName: name}
	}
	if agent.Alias != "" {
		return r.CallAgent(ctx, agent.Alias, input)
	}

	if agent.Function != nil {
		var inputMap map[string]any
		if err := yaml.Unmarshal([]byte(input), &inputMap); err != nil {
			return AgentResult{Ran: true, Error: fmt.Errorf("cannot read function input as yaml: %w", err), AgentName: name}
		}
		resp, err := agent.Function(ctx, inputMap)
		if err != nil {
			return AgentResult{Ran: true, Error: err, AgentName: name}
		}
		return AgentResult{
			Output:    resp,
			Ran:       true,
			AgentName: name,
		}
	}

	prompt := agent.Prompt
	if agent.Template != "" {
		prompt = agent.Template
	}
	// fmt.Printf("------ [Formatter: %s]\nPrompt:\n%s\n---\n", name, agent.Prompt)
	tmpl, err := utils.TemplateParse(name, prompt)
	if err != nil {
		return AgentResult{Ran: false, Error: fmt.Errorf("template parse error: %w", err), AgentName: name}
	}
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, &TemplateContext{Input: input, ctx: ctx, Registry: *r})
	if err != nil {
		return AgentResult{Ran: false, Error: fmt.Errorf("template exec error: %w", err), AgentName: name}
	}
	finalPrompt := strings.TrimSpace(buf.String())
	// fmt.Printf("------ [Formatter: %s]\nPrompt:\n%s\n---\n", name, finalPrompt)

	if agent.Template != "" {
		return AgentResult{Output: finalPrompt, Ran: true, AgentName: name}
	} else if agent.Prompt != "" {
		if finalPrompt == "" {
			return AgentResult{Ran: false, Output: "", AgentName: name}
		}
		resp, err := r.CallAI(ctx, agent, finalPrompt, &TemplateContext{Input: input, ctx: ctx, Registry: *r})
		if err != nil {
			return AgentResult{Ran: true, Error: err, AgentName: name}
		}
		return AgentResult{Output: resp, Ran: true, AgentName: name}
	}
	return AgentResult{Ran: false, Error: errors.New("invalid agent: no prompt, template or function"), AgentName: name}
}
