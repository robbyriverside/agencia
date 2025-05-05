package agencia

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
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

type RegistryCaller interface {
	CallAI(ctx context.Context, agent *agents.Agent, prompt string, tmplCtx *TemplateContext) (string, error)
}

type Registry struct {
	Agents map[string]*agents.Agent
	Chat   *Chat
}

type Libraries map[string]Registry

var libraries Libraries = map[string]Registry{
	"rag": {Agents: rag.Agents},
}

// LookupAgent resolves both unqualified and qualified agent names
func (r *Registry) LookupAgent(name string) (*agents.Agent, error) {
	if !strings.Contains(name, ".") {
		agent, ok := r.Agents[name]
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
	agent, ok := pkg.Agents[agentName]
	if !ok {
		return nil, &AgentNotFoundError{AgentName: name}
	}
	return agent, nil
}

type LogMessage struct {
	Message   string
	Timestamp time.Time
}

type TraceCard struct {
	AgentName   string
	Input       string
	Inputs      map[string]any
	Output      string
	PriorCard   *TraceCard
	BranchCards []*TraceCard
	Logs        []*LogMessage
	Facts       map[string]any // facts set by this agent
	LocalFacts  map[string]any // local facts set by this agent
}

func (r *RunContext) NewTraceCard(agent, input string, prior *TraceCard) *TraceCard {
	card := &TraceCard{
		AgentName:   agent,
		Input:       input,
		Inputs:      make(map[string]any),
		PriorCard:   r.Card,
		BranchCards: make([]*TraceCard, 0),
		Logs:        make([]*LogMessage, 0),
		Facts:       make(map[string]any, 0),
		LocalFacts:  make(map[string]any, 0),
	}
	if r.Card != nil {
		r.Card.BranchCards = append(r.Card.BranchCards, card)
	}
	return card
}

func (r *RunContext) Errorf(format string, args ...any) {
	r.Logf(fmt.Sprintf("[ERROR] %s", format), args...)
}

func (r *RunContext) Logf(format string, args ...any) {
	if r.IsPrint {
		log.Printf(format, args...)
	}
	r.Card.Logs = append(r.Card.Logs, &LogMessage{
		Message:   fmt.Sprintf(format, args...),
		Timestamp: time.Now(),
	})
}

type RunContext struct {
	IsPrint  bool
	Chat     *Chat
	Registry *Registry
	Card     *TraceCard
	Facts    map[string]any // All facts in this run (local)
}

func NewRun(reg *Registry, chat *Chat) *RunContext {
	return &RunContext{
		Chat:     chat,
		Registry: reg,
		Facts:    map[string]any{},
	}
}

func (r *Registry) RegisterAgent(agent *agents.Agent) {
	if r.Agents == nil {
		r.Agents = make(map[string]*agents.Agent)
	}
	r.Agents[agent.Name] = agent
}

// Run is the main entrypoint for calling an agent
func (r *Registry) Run(ctx context.Context, name string, input string) (string, *TraceCard) {
	run := NewRun(r, defaultChat)
	res := run.CallAgent(ctx, name, input)
	if res.Error != nil {
		logs.Error("[AGENT ERROR] %v", res.Error)
		return "", run.Card
	}
	if !res.Ran {
		logs.Info("[INFO] Agent '%s' did not run (skipped).", res.AgentName)
		return "", run.Card
	}
	out := res.Output
	if !utf8.ValidString(out) {
		out = strings.ToValidUTF8(out, "�")
	}

	if defaultChat != nil {
		agent := defaultChat.Registry.Agents[defaultChat.Agent]
		if agent != nil {
			run.ExtractAgentMemory(ctx, agent, input, out)
		}
		defaultChat.Cards = append(defaultChat.Cards, run.Card)
	}
	return out, run.Card
}

// RunPrint is the main entrypoint for calling an agent from the CLI
func (r *Registry) RunPrint(ctx context.Context, name string, input string) error {
	run := NewRun(r, defaultChat)
	run.IsPrint = true
	res := run.CallAgent(ctx, name, input)
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
	if IsVerbose() {
		card := run.Card
		if card != nil {
			// TODO: print card trace (with branches)
		}
	}
	return nil
}

func (r *RunContext) CallAgent(ctx context.Context, name string, input string) AgentResult {
	r.Card = r.NewTraceCard(name, input, r.Card)
	agent, err := r.Registry.LookupAgent(name)
	if err != nil {
		return AgentResult{Ran: false, Error: err, AgentName: name}
	}
	if agent.Alias != "" {
		return r.CallAgent(ctx, agent.Alias, input)
	}

	switch {
	case agent.Function != nil:
		return r.execFunctionAgent(ctx, agent, input, name)
	case agent.Template != "":
		return r.execTemplateAgent(ctx, agent, input, name)
	case agent.Prompt != "":
		return r.execPromptAgent(ctx, agent, input, name)
	default:
		return AgentResult{Ran: false, Error: errors.New("invalid agent: no prompt, template, alias, or function"), AgentName: name}
	}
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
func (r *RunContext) parseAgentFacts(agent *agents.Agent, input string) (map[string]any, map[string]any, error) {
	factMap := make(map[string]any)
	if err := yaml.Unmarshal([]byte(input), &factMap); err != nil {
		r.Errorf("cannot read function input as yaml: %w", err)
	}
	localMap := make(map[string]any)
	missing := []string{}
	for k, arg := range agent.Facts {
		if _, ok := factMap[k]; !ok {
			missing = append(missing, k)
			continue
		}
		if arg.Scope == "local" {
			localMap[arg.Name] = factMap[arg.Name]
			delete(factMap, arg.Name)
		}
	}
	if len(missing) > 0 {
		r.Errorf("required facts missing in agent: %s - %q", agent.Name, missing)
		// return nil, nil, fmt.Errorf("required inputs missing in agent: %s - %q", agent.Name, missing)
	}
	return factMap, localMap, nil
}

func (r *RunContext) handleAgentInputs(ctx context.Context, agent *agents.Agent, input string) (map[string]any, error) {
	promptDesc := "Fill out the following YAML fields based on the input. Each value is described and includes a type hint.\n\nInput:\n" + input + "\n\nFields:\n"
	for k, arg := range agent.Inputs {
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

	resp, err := r.extractAgentValues(ctx, agent, promptDesc)
	if err != nil {
		// Retry once with clarification request
		promptDesc += "\nIf there was an error understanding the request, explain the issue clearly in your YAML response."
		resp, err = r.extractAgentValues(ctx, agent, promptDesc)
		if err != nil {
			return nil, err
		}
	}
	inputMap, err := r.parseAgentInputs(agent, resp)
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
	promptDesc := "Fill out the following YAML fields based on the input. Each value is described and includes a type hint.\n\nInput:\n" + input + "\n\nFields:\n"
	for k, arg := range agent.Facts {
		scope := "global"
		if arg.Scope == "local" {
			scope = "local"
		}
		promptDesc += fmt.Sprintf("%s: %s (type: %s, %s)\n", k, arg.Description, arg.Type, scope)
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
greeting: the greeting message. (type: string, required)
note: an optional note to include. (type: string, optional)

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
	factMap, localMap, err := r.parseAgentFacts(agent, resp)
	if err != nil {
		return err
	}
	for k, v := range factMap {
		if r.Chat != nil {
			r.Chat.Facts[k] = v
		}
		r.Card.Facts[k] = v
	}
	for k, v := range localMap {
		r.Facts[k] = v
		r.Card.LocalFacts[k] = v
	}
	return nil
}

func (r *RunContext) execFunctionAgent(ctx context.Context, agent *agents.Agent, input string, name string) AgentResult {
	inputMap, err := r.handleAgentInputs(ctx, agent, input)
	if err != nil {
		return AgentResult{Ran: false, Error: err, AgentName: name}
	}
	resp, err := agent.Function(ctx, inputMap)
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
	finalPrompt, err := r.renderFinalPrompt(ctx, agent.Prompt, agent, input)
	if err != nil {
		return AgentResult{Ran: false, Error: err, AgentName: name}
	}
	if finalPrompt == "" {
		return AgentResult{Ran: false, Output: "", AgentName: name}
	}
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
	err = tmpl.Execute(&buf, &TemplateContext{UserInput: input, ctx: ctx, Run: r, Inputs: inputMap})
	if err != nil {
		return "", fmt.Errorf("template exec error: %w", err)
	}
	return strings.TrimSpace(buf.String()), nil
}
