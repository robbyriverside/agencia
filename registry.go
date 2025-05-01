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

func (r *Registry) generateAIInputFromPrompt(ctx context.Context, agent *agents.Agent, prompt string) (string, error) {
	resp, err := r.CallAI(ctx, agent, prompt, &TemplateContext{Input: prompt, ctx: ctx, Registry: *r})
	if err != nil {
		return "", err
	}
	resp = strings.TrimSpace(resp)

	if strings.HasPrefix(resp, "ERROR:") {
		return "", fmt.Errorf("AI error: %s", resp)
	}
	if strings.Contains(resp, "```") {
		start := strings.Index(resp, "```")
		end := strings.LastIndex(resp, "```")
		if start != -1 && end > start {
			resp = resp[start+3 : end]
			resp = strings.TrimSpace(resp)
		}
	}
	if strings.HasPrefix(resp, "yaml\n") {
		resp = strings.TrimPrefix(resp, "yaml\n")
		resp = strings.TrimSpace(resp)
	}
	return resp, nil
}

func (r *Registry) CallAgent(ctx context.Context, name string, input string) AgentResult {
	agent, err := r.LookupAgent(name)
	if err != nil {
		return AgentResult{Ran: false, Error: err, AgentName: name}
	}
	if agent.Alias != "" {
		return r.CallAgent(ctx, agent.Alias, input)
	}

	switch {
	case agent.Function != nil:
		return r.handleFunctionAgent(ctx, agent, input, name)
	case agent.Template != "":
		return r.handleTemplateAgent(ctx, agent, input, name)
	case agent.Prompt != "":
		return r.handlePromptAgent(ctx, agent, input, name)
	default:
		return AgentResult{Ran: false, Error: errors.New("invalid agent: no prompt, template or function"), AgentName: name}
	}
}

// parseYAMLAndCheckRequired parses YAML input into a map and checks if all required fields are present.
func parseYAMLAndCheckRequired(agent *agents.Agent, input string) (map[string]any, bool, error) {
	inputMap := make(map[string]any)
	if err := yaml.Unmarshal([]byte(input), &inputMap); err != nil {
		return nil, false, fmt.Errorf("cannot read function input as yaml: %w", err)
	}
	hasAllRequired := true
	for k, arg := range agent.InputPrompt {
		if arg.Required {
			if _, ok := inputMap[k]; !ok {
				hasAllRequired = false
			}
		}
	}
	return inputMap, hasAllRequired, nil
}

func (r *Registry) handleFunctionAgent(ctx context.Context, agent *agents.Agent, input string, name string) AgentResult {
	var inputMap map[string]any
	var err error

	if agent.InputPrompt != nil {
		inputMap, complete, err := parseYAMLAndCheckRequired(agent, input)
		if err == nil && complete {
			// no AI call needed
		} else {
			promptDesc := "Fill out the following YAML fields based on the input. Each value is described and includes a type hint.\n\nInput:\n" + input + "\n\nFields:\n"
			for k, arg := range agent.InputPrompt {
				required := "optional"
				if arg.Required {
					required = "required"
				}
				promptDesc += fmt.Sprintf("%s: %s (type: %s, %s)\n", k, arg.Description, arg.Type, required)
			}
			promptDesc += `
Respond ONLY with a valid YAML object that matches the above field descriptions. 
Do not include markdown formatting or any explanation. 
If a required field cannot be reasonably inferred from the input, respond with:
ERROR: missing required field <name>

Example:

Input:
Please generate a greeting and optionally add a note.

Fields:
greeting: the greeting message. (type: string, required)
note: an optional note to include. (type: string, optional)

Expected YAML:
greeting: Hello!
note: Have a nice day.
`

			resp, err := r.generateAIInputFromPrompt(ctx, agent, promptDesc)
			if err != nil {
				// Retry once with clarification request
				promptDesc += "\nIf there was an error understanding the request, explain the issue clearly in your YAML response."
				resp, retryErr := r.generateAIInputFromPrompt(ctx, agent, promptDesc)
				if retryErr != nil {
					return AgentResult{Ran: true, Error: retryErr, AgentName: name}
				}
				// try to parse retry response
				if err := yaml.Unmarshal([]byte(resp), &inputMap); err != nil {
					return AgentResult{Ran: true, Error: fmt.Errorf("failed to parse AI YAML output: %w\nAI response:\n%s", err, resp), AgentName: name}
				}
			} else {
				if err := yaml.Unmarshal([]byte(resp), &inputMap); err != nil {
					return AgentResult{Ran: true, Error: fmt.Errorf("failed to parse AI YAML output: %w\nAI response:\n%s", err, resp), AgentName: name}
				}
			}
		}
	} else {
		inputMap = make(map[string]any)
		if err := yaml.Unmarshal([]byte(input), &inputMap); err != nil {
			return AgentResult{Ran: true, Error: fmt.Errorf("cannot read function input as yaml: %w", err), AgentName: name}
		}
	}

	resp, err := agent.Function(ctx, inputMap)
	if err != nil {
		return AgentResult{Ran: true, Error: err, AgentName: name}
	}
	return AgentResult{Output: resp, Ran: true, AgentName: name}
}

func (r *Registry) handleTemplateAgent(ctx context.Context, agent *agents.Agent, input string, name string) AgentResult {
	finalPrompt, err := r.renderFinalPrompt(ctx, agent.Template, name, input)
	if err != nil {
		return AgentResult{Ran: false, Error: err, AgentName: name}
	}
	return AgentResult{Output: finalPrompt, Ran: true, AgentName: name}
}

func (r *Registry) handlePromptAgent(ctx context.Context, agent *agents.Agent, input string, name string) AgentResult {
	finalPrompt, err := r.renderFinalPrompt(ctx, agent.Prompt, name, input)
	if err != nil {
		return AgentResult{Ran: false, Error: err, AgentName: name}
	}
	if finalPrompt == "" {
		return AgentResult{Ran: false, Output: "", AgentName: name}
	}
	resp, err := r.CallAI(ctx, agent, finalPrompt, &TemplateContext{Input: input, ctx: ctx, Registry: *r})
	if err != nil {
		return AgentResult{Ran: true, Error: err, AgentName: name}
	}
	return AgentResult{Output: resp, Ran: true, AgentName: name}
}

func (r *Registry) renderFinalPrompt(ctx context.Context, template string, name string, input string) (string, error) {
	tmpl, err := utils.TemplateParse(name, template)
	if err != nil {
		return "", fmt.Errorf("template parse error: %w", err)
	}
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, &TemplateContext{Input: input, ctx: ctx, Registry: *r})
	if err != nil {
		return "", fmt.Errorf("template exec error: %w", err)
	}
	return strings.TrimSpace(buf.String()), nil
}
