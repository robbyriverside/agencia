package agencia

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/robbyriverside/agencia/agents"
	"github.com/robbyriverside/agencia/utils"
	"gopkg.in/yaml.v3"
)

type RunContext struct {
	IsPrint    bool
	Chat       *Chat
	Registry   *Registry
	Card       *TraceCard     // prompt used for this run
	Depth      int            // current depth of nested CallAgent invocations
	LocalFacts map[string]any // All facts stored locally during this run
}

func NewRun(reg *Registry, chat *Chat) *RunContext {
	return &RunContext{
		Chat:       chat, // can be nil
		Registry:   reg,
		LocalFacts: map[string]any{},
	}
}

func (r *RunContext) CallAgent(ctx context.Context, name string, input string) AgentResult {
	// recursion guard
	if r.Depth >= maxCallDepth {
		return AgentResult{
			Ran:       false,
			Error:     fmt.Errorf("recursive agent calls exceeded %d", maxCallDepth),
			AgentName: name,
		}
	}
	r.Depth++
	defer func() { r.Depth-- }()

	card := r.NewTraceCard(name, input)
	if r.Card != nil {
		r.Card.BranchCards = append(r.Card.BranchCards, card)
	}
	r.Card = card
	agent, err := r.Registry.LookupAgent(name)
	if err != nil {
		return AgentResult{Ran: false, Error: err, AgentName: name}
	}

	var result AgentResult
	if agent.Alias != "" {
		aliasAgent, err := r.Registry.LookupAgent(agent.Alias)
		if err != nil {
			return AgentResult{Ran: false, Error: fmt.Errorf("invalid alias agent: %s", err), AgentName: name}
		}
		switch {
		case aliasAgent.Function != nil:
			result = r.execFunctionAlias(ctx, agent, aliasAgent, input, name)
		case aliasAgent.Template != "":
			result = r.execTemplateAlias(ctx, agent, aliasAgent, input, name)
		case aliasAgent.Prompt != "":
			result = r.execPromptAlias(ctx, agent, aliasAgent, input, name)
		case aliasAgent.Alias != "":
			return AgentResult{Ran: false, Error: errors.New("invalid alias of an alias agent not allowed"), AgentName: name}
		default:
			return AgentResult{Ran: false, Error: errors.New("invalid alias agent: no prompt, template, or function"), AgentName: name}
		}
	} else {
		switch {
		case agent.Function != nil:
			result = r.execFunctionAgent(ctx, agent, input, name)
		case agent.Template != "":
			result = r.execTemplateAgent(ctx, agent, input, name)
		case agent.Prompt != "":
			result = r.execPromptAgent(ctx, agent, input, name)
		default:
			return AgentResult{Ran: false, Error: errors.New("invalid agent: no prompt, template, alias, or function"), AgentName: name}
		}
	}
	r.Card.Output = result.Output
	r.Card.Ran = result.Ran
	r.Card.Error = result.Error
	if card.PriorCard != nil {
		r.Card = card.PriorCard // may be nil for top‑level
	}
	return result
}

func (r *RunContext) extractAgentValues(ctx context.Context, agent *agents.Agent, prompt string) (string, error) {
	resp, err := r.CallAI(ctx, agent, prompt)
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

// parseAgentInputs parses YAML input into a map and checks if all required fields are present.
// the input is yaml provided by AI
func (r *RunContext) parseAgentInputs(agent *agents.Agent, input string) (map[string]any, error) {
	inputMap := make(map[string]any)
	if err := yaml.Unmarshal([]byte(input), &inputMap); err != nil {
		r.Errorf("cannot read function input as yaml: %w", err)
	}
	missing := []string{}
	for k, arg := range agent.Inputs {
		if arg.Required {
			if _, ok := inputMap[k]; !ok {
				missing = append(missing, k)
			}
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("required inputs missing in agent: %s - %q", agent.Name, missing)
	}
	return inputMap, nil
}

// parseAgentInputs parses YAML input into a map and checks if all required fields are present.
// the input is yaml provided by AI
func (r *RunContext) parseAgentFacts(agent *agents.Agent, role *agents.AgentRole, input string) (map[string]any, map[string]any, error) {
	factMap := make(map[string]any)
	if err := yaml.Unmarshal([]byte(input), &factMap); err != nil {
		r.Errorf("cannot read function input as yaml: %w", err)
	}
	localMap := make(map[string]any)
	for k, arg := range agent.Facts {
		if arg.Scope == "local" {
			localMap[k] = factMap[k]
			delete(factMap, k)
		}
	}
	if role != nil && role.Facts != nil {
		for k, arg := range role.Facts {
			if arg.Scope == "local" {
				localMap[k] = factMap[k]
				delete(factMap, k)
			}
		}
	}
	return factMap, localMap, nil
}

func (r *RunContext) handleAgentInputs(ctx context.Context, agent *agents.Agent, input string) (map[string]any, error) {
	var role *agents.AgentRole
	roleInputs := map[string]*agents.Argument{}
	if agent.Role != "" {
		var ok bool
		role, ok = r.Registry.LookupRole(agent.Role)
		if !ok {
			return nil, fmt.Errorf("agent role not found: %s", agent.Role)
		}
		roleInputs = role.Inputs
	}
	if len(agent.Inputs)+len(roleInputs) == 0 {
		return nil, nil
	}
	promptDesc := "Fill out the following YAML fields based on the input. Each value is described and includes a type hint.\n\nInput:\n" + input + "\n\nFields:\n"
	for k, arg := range agent.Inputs {
		required := "optional"
		if arg.Required {
			required = "required"
		}
		promptDesc += fmt.Sprintf("%s: %s (type: %s, %s)\n", k, arg.Description, arg.Type, required)
	}
	for k, arg := range roleInputs {
		if agent.Inputs[k] != nil {
			r.Errorf("agent input %s conflicts with role input %s", k, arg.Name)
			continue
		}
		promptDesc += fmt.Sprintf("%s: %s (type: %s, %s)\n", k, arg.Description, arg.Type, "optional")
	}
	promptDesc += `
Respond ONLY with a valid YAML object that matches the above field descriptions. 
Do not include markdown formatting or any explanation. 

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

	values, err := r.extractAgentValues(ctx, agent, promptDesc)
	if err != nil {
		// Retry once with clarification request
		promptDesc += "\nIf there was an error understanding the request, explain the issue clearly in your YAML response."
		values, err = r.extractAgentValues(ctx, agent, promptDesc)
		if err != nil {
			return nil, err
		}
	}
	inputMap, err := r.parseAgentInputs(agent, values)
	if err != nil {
		return nil, err
	}
	if len(inputMap) == 0 && agent.Function != nil {
		inputMap = make(map[string]any)
		if err := yaml.Unmarshal([]byte(input), &inputMap); err != nil {
			r.Errorf("cannot read function input as yaml: %w", err)
		}
	}
	r.Card.Inputs = inputMap
	return inputMap, nil
}

func (r *RunContext) handleAgentFacts(ctx context.Context, agent *agents.Agent, input string) error {
	if len(agent.Facts) == 0 {
		return nil
	}
	chat := r.Chat
	facts := make(map[string]any)
	if chat != nil {
		for k := range agent.Facts {
			val, ok := chat.Facts[k]
			if !ok {
				continue
			}
			facts[k] = val
		}
	}
	var role *agents.AgentRole
	var ok bool
	if agent.Role != "" {
		role, ok = r.Registry.LookupRole(agent.Role)
		if ok && role.Facts != nil {
			for k := range role.Facts {
				val, ok := chat.Facts[k]
				if !ok {
					continue
				}
				facts[k] = val
			}
		}
	}
	promptDesc := "Fill out the following YAML fields based on the input. Each value is described and includes a type hint.\n\nInput:\n" + input + "\n\nFields:\n"
	for k, arg := range agent.Facts {
		scope := "global"
		if arg.Scope == "local" {
			scope = "local"
		}
		val, ok := facts[k]
		if !ok {
			val = arg.EmptyDefault()
		}
		promptDesc += fmt.Sprintf("%s: %s (type: %s, %s) (old: %v)\n", k, arg.Description, arg.Type, scope, val)
	}
	if role != nil && role.Facts != nil {
		for k, arg := range role.Facts {
			scope := "global"
			if arg.Scope == "local" {
				scope = "local"
			}
			val, ok := facts[k]
			if !ok {
				val = arg.EmptyDefault()
			}
			promptDesc += fmt.Sprintf("%s: %s (type: %s, %s) (old: %v)\n", k, arg.Description, arg.Type, scope, val)
		}
	}
	promptDesc += `
Respond ONLY with a valid YAML object that matches the above field descriptions. 
Do not include markdown formatting or any explanation. 
If a required field cannot be reasonably inferred from the input, leave the field blank.
If a field is not relevant to the input, leave it blank.

Example:

Input:
Please generate a greeting and optionally add a note.

Fields:
greeting: the greeting message. (type: string, required) (old: "prior fact value")
note: an optional note to include. (type: string, optional) (old: "prior fact value")

Expected YAML:
greeting: Hello!
note: Have a nice day.
`

	resp, err := r.extractAgentValues(ctx, agent, promptDesc)
	if err != nil {
		// Retry once with clarification request
		promptDesc += "\nIf there was an error understanding the request, explain the issue clearly in your YAML response."
		resp, err = r.extractAgentValues(ctx, agent, promptDesc)
		if err != nil {
			return err
		}
	}
	factMap, localMap, err := r.parseAgentFacts(agent, role, resp)
	if err != nil {
		return err
	}
	for k, v := range factMap {
		if _, ok := agent.Facts[k]; ok {
			name := k
			if !strings.Contains(k, ".") {
				name = fmt.Sprintf("%s.%s", agent.Name, k)
			}
			r.AssignFact(agent, name, v)
			r.Card.Facts[name] = v
			continue
		}
		if role != nil && role.Facts != nil {
			if _, ok := role.Facts[k]; ok {
				name := k
				if !strings.Contains(k, ".") {
					name = fmt.Sprintf("%s.%s", role.ID, k)
				}
				r.AssignRoleFact(role, name, v)
				r.Card.Facts[name] = v
			}
		}
	}
	for k, v := range localMap {
		if _, ok := agent.Facts[k]; ok {
			name := k
			if !strings.Contains(k, ".") {
				name = fmt.Sprintf("%s.%s", agent.Name, k)
			}
			r.AssignLocalFact(agent, name, v)
			r.Card.LocalFacts[name] = v
			continue
		}
		if role != nil && role.Facts != nil {
			if _, ok := role.Facts[k]; ok {
				name := k
				if !strings.Contains(k, ".") {
					name = fmt.Sprintf("%s.%s", role.ID, k)
				}
				r.AssignRoleLocalFact(role, name, v)
				r.Card.LocalFacts[name] = v
			}
		}
	}
	return nil
}

func (r *RunContext) execFunctionAgent(ctx context.Context, agent *agents.Agent, input string, name string) AgentResult {
	inputMap, err := r.handleAgentInputs(ctx, agent, input)
	if err != nil {
		return AgentResult{Ran: false, Error: err, AgentName: name}
	}
	if inputMap == nil {
		return AgentResult{Ran: false, Output: "", AgentName: name, Error: errors.New("function agent requires inputs")}
	}
	resp, err := agent.Function(ctx, inputMap, agent)
	if err != nil {
		return AgentResult{Ran: true, Error: err, AgentName: name}
	}
	if r.Chat != nil {
		r.ExtractAgentMemory(ctx, agent, input, resp)
	}
	if err := r.handleAgentFacts(ctx, agent, input); err != nil {
		return AgentResult{Output: resp, Ran: true, Error: err, AgentName: name}
	}
	return AgentResult{Output: resp, Ran: true, AgentName: name}
}

func (r *RunContext) execTemplateAgent(ctx context.Context, agent *agents.Agent, input string, name string) AgentResult {
	finalPrompt, err := r.renderFinalPrompt(ctx, agent.Template, agent, input)
	if err != nil {
		return AgentResult{Ran: false, Error: err, AgentName: name}
	}
	if r.Chat != nil {
		r.ExtractAgentMemory(ctx, agent, input, finalPrompt)
	}
	if err := r.handleAgentFacts(ctx, agent, input); err != nil {
		return AgentResult{Output: finalPrompt, Ran: true, Error: err, AgentName: name}
	}
	return AgentResult{Output: finalPrompt, Ran: true, AgentName: name}
}

func (r *RunContext) execPromptAgent(ctx context.Context, agent *agents.Agent, input string, name string) AgentResult {
	template := agent.Prompt
	if agent.Role != "" {
		role, ok := r.Registry.LookupRole(agent.Role)
		if ok && role.Description != "" || role.Personality != "" || role.Performance != "" {
			var personality string
			var performance string
			if role.Performance != "" {
				performance = fmt.Sprintf("\nDiscuss your job with this Performance approach:\n %s\n", role.Performance)
			}
			if role.Personality != "" {
				personality = fmt.Sprintf("\nRespond with this Personality:\n %s\n", role.Personality)
				template = fmt.Sprintf("%s\n\nYou Are Playing: %s\n%s%s", template, role.ID, personality, performance)
			}
		}
	}
	finalPrompt, err := r.renderFinalPrompt(ctx, template, agent, input)
	if err != nil {
		return AgentResult{Ran: false, Error: err, AgentName: name}
	}
	if finalPrompt == "" {
		return AgentResult{Ran: false, Output: "", AgentName: name}
	}
	r.Card.Prompt = finalPrompt
	resp, err := r.CallAI(ctx, agent, finalPrompt)
	if err != nil {
		return AgentResult{Ran: true, Error: err, AgentName: name}
	}
	if r.Chat != nil {
		r.ExtractAgentMemory(ctx, agent, input, resp)
	}
	if err := r.handleAgentFacts(ctx, agent, input); err != nil {
		return AgentResult{Output: resp, Ran: true, Error: err, AgentName: name}
	}
	return AgentResult{Output: resp, Ran: true, AgentName: name}
}

func (r *RunContext) execFunctionAlias(ctx context.Context, agent, alias *agents.Agent, input string, name string) AgentResult {
	inputMap, err := r.handleAgentInputs(ctx, agent, input)
	if err != nil {
		return AgentResult{Ran: false, Error: err, AgentName: name}
	}
	if inputMap == nil {
		return AgentResult{Ran: false, Output: "", AgentName: name, Error: errors.New("function agent requires inputs")}
	}
	resp, err := alias.Function(ctx, inputMap, agent)
	if err != nil {
		return AgentResult{Ran: true, Error: err, AgentName: name}
	}
	if r.Chat != nil {
		r.ExtractAgentMemory(ctx, agent, input, resp)
	}
	if err := r.handleAgentFacts(ctx, agent, input); err != nil {
		return AgentResult{Output: resp, Ran: true, Error: err, AgentName: name}
	}
	return AgentResult{Output: resp, Ran: true, AgentName: name}
}

func (r *RunContext) execTemplateAlias(ctx context.Context, agent, alias *agents.Agent, input string, name string) AgentResult {
	finalPrompt, err := r.renderFinalPrompt(ctx, alias.Template, agent, input)
	if err != nil {
		return AgentResult{Ran: false, Error: err, AgentName: name}
	}
	if r.Chat != nil {
		r.ExtractAgentMemory(ctx, agent, input, finalPrompt)
	}
	if err := r.handleAgentFacts(ctx, agent, input); err != nil {
		return AgentResult{Output: finalPrompt, Ran: true, Error: err, AgentName: name}
	}
	return AgentResult{Output: finalPrompt, Ran: true, AgentName: name}
}

func (r *RunContext) execPromptAlias(ctx context.Context, agent, alias *agents.Agent, input string, name string) AgentResult {
	template := alias.Prompt
	if agent.Role != "" {
		role, ok := r.Registry.LookupRole(agent.Role)
		if ok && role.Personality != "" || role.Performance != "" {
			var personality string
			var performance string
			if role.Performance != "" {
				performance = fmt.Sprintf("\nDiscuss your job with this Performance approach:\n %s\n", role.Performance)
			}
			if role.Personality != "" {
				personality = fmt.Sprintf("\nRespond with this Personality:\n %s\n", role.Personality)
				template = fmt.Sprintf("%s\n\nYou Are Playing: %s\n%s%s", template, role.ID, personality, performance)
			}
		}
	}
	finalPrompt, err := r.renderFinalPrompt(ctx, template, agent, input)
	if err != nil {
		return AgentResult{Ran: false, Error: err, AgentName: name}
	}
	if finalPrompt == "" {
		return AgentResult{Ran: false, Output: "", AgentName: name}
	}
	r.Card.Prompt = finalPrompt
	resp, err := r.CallAI(ctx, agent, finalPrompt)
	if err != nil {
		return AgentResult{Ran: true, Error: err, AgentName: name}
	}
	if r.Chat != nil {
		r.ExtractAgentMemory(ctx, agent, input, resp)
	}
	if err := r.handleAgentFacts(ctx, agent, input); err != nil {
		return AgentResult{Output: resp, Ran: true, Error: err, AgentName: name}
	}
	return AgentResult{Output: resp, Ran: true, AgentName: name}
}

func (r *RunContext) renderFinalPrompt(ctx context.Context, template string, agent *agents.Agent, input string) (string, error) {
	inputMap, err := r.handleAgentInputs(ctx, agent, input)
	if err != nil {
		return "", err
	}
	tmpl, err := utils.TemplateParse(agent.Name, template)
	if err != nil {
		return "", fmt.Errorf("template parse error: %w", err)
	}
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, NewTemplateContext(ctx, agent, input, r, inputMap))
	if err != nil {
		return "", fmt.Errorf("template exec error: %w", err)
	}
	return strings.TrimSpace(buf.String()), nil
}

// AssignFact assigns or updates a fact in the chat's Facts map according to the agent's Fact definition.
func (r *RunContext) AssignFact(agent *agents.Agent, name string, v any) {
	if r.Chat == nil {
		return
	}
	fact := agent.Facts[name]
	if existing, ok := r.Chat.Facts[name]; ok && fact != nil && fact.Add {
		switch val := existing.(type) {
		case []any:
			r.Chat.Facts[name] = append(val, v)
		case string:
			r.Chat.Facts[name] = fmt.Sprintf("%s\n%s", val, v)
		case int:
			switch v2 := v.(type) {
			case int:
				r.Chat.Facts[name] = val + v2
			case float64:
				r.Chat.Facts[name] = float64(val) + v2
			default:
				r.Chat.Facts[name] = v
			}
		case float64:
			switch v2 := v.(type) {
			case int:
				r.Chat.Facts[name] = val + float64(v2)
			case float64:
				r.Chat.Facts[name] = val + v2
			default:
				r.Chat.Facts[name] = v2
			}
		default:
			r.Chat.Facts[name] = v
		}
	} else {
		r.Chat.Facts[name] = v
	}
}

// AssignLocalFact assigns or updates a local fact in the RunContext's LocalFacts map according to the agent's Fact definition.
func (r *RunContext) AssignLocalFact(agent *agents.Agent, name string, v any) {
	if r.LocalFacts == nil {
		r.LocalFacts = make(map[string]any)
	}
	fact := agent.Facts[name]
	if existing, ok := r.LocalFacts[name]; ok && fact != nil && fact.Add {
		switch val := existing.(type) {
		case []any:
			r.LocalFacts[name] = append(val, v)
		case string:
			r.LocalFacts[name] = fmt.Sprintf("%s\n%s", val, v)
		case int:
			switch v2 := v.(type) {
			case int:
				r.LocalFacts[name] = val + v2
			case float64:
				r.LocalFacts[name] = float64(val) + v2
			default:
				r.LocalFacts[name] = v
			}
		case float64:
			switch v2 := v.(type) {
			case int:
				r.LocalFacts[name] = val + float64(v2)
			case float64:
				r.LocalFacts[name] = val + v2
			default:
				r.LocalFacts[name] = v2
			}
		default:
			r.LocalFacts[name] = v
		}
	} else {
		r.LocalFacts[name] = v
	}
}

// AssignRoleFact assigns or updates a fact in the chat's Facts map according to the role's Fact definition.
func (r *RunContext) AssignRoleFact(role *agents.AgentRole, name string, v any) {
	if r.Chat == nil {
		return
	}
	fact := role.Facts[name]
	if existing, ok := r.Chat.Facts[name]; ok && fact != nil && fact.Add {
		switch val := existing.(type) {
		case []any:
			r.Chat.Facts[name] = append(val, v)
		case string:
			r.Chat.Facts[name] = fmt.Sprintf("%s\n%s", val, v)
		case int:
			switch v2 := v.(type) {
			case int:
				r.Chat.Facts[name] = val + v2
			case float64:
				r.Chat.Facts[name] = float64(val) + v2
			default:
				r.Chat.Facts[name] = v
			}
		case float64:
			switch v2 := v.(type) {
			case int:
				r.Chat.Facts[name] = val + float64(v2)
			case float64:
				r.Chat.Facts[name] = val + v2
			default:
				r.Chat.Facts[name] = v2
			}
		default:
			r.Chat.Facts[name] = v
		}
	} else {
		r.Chat.Facts[name] = v
	}
}

// AssignRoleLocalFact assigns or updates a local fact in the RunContext's LocalFacts map according to the role's Fact definition.
func (r *RunContext) AssignRoleLocalFact(role *agents.AgentRole, name string, v any) {
	if r.LocalFacts == nil {
		r.LocalFacts = make(map[string]any)
	}
	fact := role.Facts[name]
	if existing, ok := r.LocalFacts[name]; ok && fact != nil && fact.Add {
		switch val := existing.(type) {
		case []any:
			r.LocalFacts[name] = append(val, v)
		case string:
			r.LocalFacts[name] = fmt.Sprintf("%s\n%s", val, v)
		case int:
			switch v2 := v.(type) {
			case int:
				r.LocalFacts[name] = val + v2
			case float64:
				r.LocalFacts[name] = float64(val) + v2
			default:
				r.LocalFacts[name] = v
			}
		case float64:
			switch v2 := v.(type) {
			case int:
				r.LocalFacts[name] = val + float64(v2)
			case float64:
				r.LocalFacts[name] = val + v2
			default:
				r.LocalFacts[name] = v2
			}
		default:
			r.LocalFacts[name] = v
		}
	} else {
		r.LocalFacts[name] = v
	}
}
