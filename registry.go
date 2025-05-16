package agencia

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/robbyriverside/agencia/agents"
	"github.com/robbyriverside/agencia/lib/rag"
	"github.com/robbyriverside/agencia/logs"
	"github.com/robbyriverside/agencia/utils"
	"gopkg.in/yaml.v3"
)

const maxCallDepth = 6 // safeguard against runaway .Get or alias recursion

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
	Prompt      string
	Error       error
	Ran         bool
	PriorCard   *TraceCard
	BranchCards []*TraceCard
	Logs        []*LogMessage
	Facts       map[string]any // facts set by this agent
	LocalFacts  map[string]any // local facts set by this agent
}

func (c *TraceCard) String() string {
	errstr := "no error"
	if c.Error != nil {
		errstr = fmt.Sprintf("Error: %s\n", c.Error)
	}
	ranstr := "did not run"
	if c.Ran {
		ranstr = "agent ran"
	}
	inputs := fmt.Sprintf("%v", c.Inputs)
	inputs = inputs[4 : len(inputs)-1]
	facts := fmt.Sprintf("%v", c.Facts)
	facts = facts[4 : len(facts)-1]
	locals := fmt.Sprintf("%v", c.LocalFacts)
	locals = locals[4 : len(locals)-1]
	if len(locals) == 0 {
		locals = "none"
	} else {
		locals = fmt.Sprintf("%q", locals)
	}
	prompt := c.Prompt
	if len(prompt) > 0 {
		prompt = fmt.Sprintf("Prompt: %q\n", prompt)
	}
	if len(facts) == 0 {
		facts = "none"
	} else {
		facts = fmt.Sprintf("%q", facts)
	}
	if len(inputs) == 0 {
		inputs = "none"
	} else {
		inputs = fmt.Sprintf("%q", inputs)
	}

	results := fmt.Sprintf("Agent: %s\nInput: \"%s\"\nOutput: \"%s\"\n%s%s\n%s\nInputs: %s\nFacts: %s\nLocalFacts: %s",
		c.AgentName, c.Input, c.Output, prompt, ranstr, errstr, inputs, facts, locals)

	if len(c.Logs) == 0 {
		results += "\nno logs"
	} else {
		results += "\nLogs:"
	}
	for _, log := range c.Logs {
		results += fmt.Sprintf("\n  %s: %s", log.Timestamp.Format(time.RFC3339), log.Message)
	}
	return results
}

func (c *TraceCard) ShortString() string {
	errstr := "no error"
	if c.Error != nil {
		errstr = fmt.Sprintf("Error: %s\n", c.Error)
	}
	prior := "none"
	if c.PriorCard != nil {
		prior = c.PriorCard.AgentName
	}
	return fmt.Sprintf("Agent: %s\nFrom: %s\nInput: \"%s\"\nOutput: \"%s\"\n%s",
		c.AgentName, prior, c.Input, c.Output, errstr)
}

func (r *RunContext) NewTraceCard(agent, input string) *TraceCard {
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
	return card
}

func (c *TraceCard) SaveMarkdown(filename string, short ...bool) error {
	log.Println("Saving trace to", filename)
	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("[TRACE CARD ERROR] Failed to create file: %v", err)
	}
	defer f.Close()

	c.WriteMarkdown(f, short...)
	if err != nil {
		return fmt.Errorf("[TRACE CARD ERROR] Failed to write to file: %v", err)
	}
	return nil
}

func (c *TraceCard) WriteMarkdown(w io.Writer, short ...bool) {
	if c == nil {
		return
	}
	if len(short) > 0 && short[0] {
		c.WriteMarkdownShort(w, 1, 1)
		return
	}
	fmt.Fprintf(w, "# Agent Trace: %s\n", c.AgentName)

	c.WriteMarkdownLevel(w, 1, 1)
}

func (c *TraceCard) WriteMarkdownLevel(w io.Writer, index, level int) {
	if c == nil {
		return
	}

	var from string
	if level > 1 {
		from = fmt.Sprintf(" From: %s\n", c.PriorCard.AgentName)
	}
	fmt.Fprintf(w, "\n## %d.%d: %s%s\n", level, index, c.AgentName, from)

	fmt.Fprintf(w, "\n```%s\n```\n", c.String()) // TODO: add from, level, index

	for i, card := range c.BranchCards {
		card.WriteMarkdownLevel(w, i+1, level+1)
	}
}

func (c *TraceCard) WriteMarkdownShort(w io.Writer, index, level int) {
	if c == nil {
		return
	}
	fmt.Fprintf(w, "\n```\nLevel: %d.%d\n%s\n```", level, index, c.ShortString()) // TODO: add from, level, index

	for i, card := range c.BranchCards {
		card.WriteMarkdownShort(w, i+1, level+1)
	}
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
	IsPrint    bool
	Chat       *Chat
	Registry   *Registry
	Card       *TraceCard     // prompt used for this run
	Depth      int            // current depth of nested CallAgent invocations
	LocalFacts map[string]any // All facts stored locally during this run
}

func NewRun(reg *Registry, chat *Chat) *RunContext {
	return &RunContext{
		Chat:       chat,
		Registry:   reg,
		LocalFacts: map[string]any{},
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
		// logs.Error("[AGENT ERROR]", res.Error)
		return res.Error.Error(), run.Card
	}
	if !res.Ran {
		logs.Info("[INFO] Agent did not run (skipped).", res.AgentName)
		return "did not run", run.Card
	}
	out := res.Output
	if !utf8.ValidString(out) {
		out = strings.ToValidUTF8(out, "�")
	}

	if defaultChat != nil {
		agent := defaultChat.Registry.Agents[defaultChat.StartAgent]
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
	card := run.Card
	if card != nil {
		card.SaveMarkdown("trace.md", !IsVerbose())
	}

	return nil
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
	if agent.Alias != "" {
		return r.CallAgent(ctx, agent.Alias, input)
	}

	var result AgentResult
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
	if len(agent.Inputs) == 0 {
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
		r.LocalFacts[k] = v
		r.Card.LocalFacts[k] = v
	}
	return nil
}

func (r *RunContext) execFunctionAgent(ctx context.Context, agent *agents.Agent, input string, name string) AgentResult {
	inputMap, err := r.handleAgentInputs(ctx, agent, input)
	if err != nil {
		return AgentResult{Ran: false, Error: err, AgentName: name}
	}
	if inputMap == nil {
		return AgentResult{Ran: false, Output: "", AgentName: name, Error: errors.New("Function agent requires inputs")}
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
	finalPrompt, err := r.renderFinalPrompt(ctx, agent.Prompt, agent, input)
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
