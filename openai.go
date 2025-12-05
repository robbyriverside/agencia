package agencia

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/robbyriverside/agencia/agents"
	"github.com/robbyriverside/agencia/config"
	"github.com/robbyriverside/agencia/parley"
	"github.com/robbyriverside/agencia/utils"
	"gopkg.in/yaml.v3"
)

type TemplateContext struct {
	Agent     *agents.Agent
	UserInput string
	inputMap  map[string]any
	Run       *RunContext
	ctx       context.Context
	bindings  map[string]any
}

func NewTemplateContext(ctx context.Context, agent *agents.Agent, input string, run *RunContext, inputMap map[string]any) *TemplateContext {
	return &TemplateContext{
		Agent:     agent,
		UserInput: input,
		Run:       run,
		inputMap:  inputMap,
		ctx:       ctx,
		bindings:  make(map[string]any),
	}
}

func (t *TemplateContext) Fact(name string, optionalInput ...any) any {
	var result any
	if len(optionalInput) > 0 {
		result = optionalInput[0]
	}
	if t.Run.Chat == nil {
		return result
	}
	if !strings.Contains(name, ".") && t.Agent != nil {
		name = t.Agent.Name + "." + name
	}
	return t.Run.Chat.Fact(name)
}

func (t *TemplateContext) Get(name string, optionalInput ...string) string {
	input := t.UserInput
	if len(optionalInput) > 0 {
		input = optionalInput[0]
	}
	res := t.Run.CallAgent(t.ctx, name, input)
	if res.Error != nil {
		return fmt.Sprintf("[error calling %s: %v]", name, res.Error)
	}
	return res.Output
}

// Inputs returns the Inputs for the agent.
// With no agent it returns yaml for the current input values
// With an agent name it returns the definitions for that agent.
func (t *TemplateContext) Inputs(optionalInput ...string) string {
	var agent *agents.Agent
	if len(optionalInput) == 0 {
		yamlData, err := yaml.Marshal(t.inputMap)
		if err != nil {
			return fmt.Sprintf("Error: Failed to encode inputs as YAML: %v", err)
		}
		return fmt.Sprintf("```yaml\n%s```", strings.Replace(string(yamlData), "null", "", -1))
	} else {
		name := optionalInput[0]
		var err error
		agent, err = t.Run.Registry.LookupAgent(name)
		if err != nil {
			return fmt.Sprintf("Error: Agent %q not found", name)
		}
	}

	if agent == nil || len(agent.Inputs) == 0 {
		return ""
	}

	yamlData, err := yaml.Marshal(agent.Inputs)
	if err != nil {
		return fmt.Sprintf("Error: Failed to encode inputs as YAML: %v", err)
	}

	return fmt.Sprintf("```yaml\n%s```", strings.Replace(string(yamlData), "null", "", -1))
}

func (t *TemplateContext) Input(optionalInput ...string) any {
	if len(optionalInput) == 0 {
		return t.UserInput
	} else if len(optionalInput) == 1 {
		return t.inputMap[optionalInput[0]]
	}
	if len(optionalInput) > 2 {
		t.Run.Errorf("invalid Input arguments %d < 3  %q", len(optionalInput), optionalInput)
	}
	result := t.inputMap[optionalInput[0]]
	if result == "" {
		return optionalInput[1]
	} else {
		return result
	}
}

func (t *TemplateContext) Call(agent string) string {
	return t.Get(agent)
}

func (t *TemplateContext) CallWith(agent string, value any) string {
	return t.Get(agent, t.toString(value))
}

func (t *TemplateContext) CallOnList(agent string, values any) string {
	items := t.listWithStyle(values, "bullets")
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	for _, item := range items {
		out := t.Get(agent, item)
		if out == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(out)
	}
	return b.String()
}

func (t *TemplateContext) List(value any, style string) string {
	return t.ListFormat(value, style, style)
}

func (t *TemplateContext) ListFormat(value any, readStyle, writeStyle string) string {
	items := t.listWithStyle(value, readStyle)
	switch writeStyle {
	case "lines":
		return strings.Join(items, "\n")
	case "paragraphs":
		return strings.Join(items, "\n\n")
	case "bullets":
		fallthrough
	default:
		for i, item := range items {
			items[i] = "- " + item
		}
		return strings.Join(items, "\n")
	}
}

func (t *TemplateContext) Bind(label string, value any) string {
	if t.bindings == nil {
		t.bindings = make(map[string]any)
	}
	t.bindings[label] = value
	return ""
}

func (t *TemplateContext) Lookup(label string) any {
	if label == "" {
		return ""
	}
	if val, ok := t.bindings[label]; ok {
		return val
	}
	if t.inputMap != nil {
		if val, ok := t.inputMap[label]; ok {
			return val
		}
	}
	if t.Run != nil && t.Run.Chat != nil {
		if v := t.Run.Chat.Fact(label); v != nil {
			return v
		}
	}
	return t.Fact(label)
}

// Equals of two case insensitive strings
func (t *TemplateContext) Equals(value any, expected string) bool {
	return strings.EqualFold(t.toString(value), expected)
}

func (t *TemplateContext) Has(value any, expected string) bool {
	items := t.listWithStyle(value, "bullets")
	expectedLower := strings.ToLower(expected)
	for _, item := range items {
		if strings.Contains(strings.ToLower(item), expectedLower) {
			return true
		}
	}
	return false
}

func (t *TemplateContext) IsEmpty(value any) bool {
	switch v := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(v) == ""
	case []string:
		return len(v) == 0
	case []any:
		return len(v) == 0
	default:
		return strings.TrimSpace(t.toString(v)) == ""
	}
}

func (t *TemplateContext) toString(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		return fmt.Sprintf("%v", v)
	}
}

func (t *TemplateContext) toStringSlice(value any) []string {
	return t.listWithStyle(value, "bullets")
}

func (t *TemplateContext) Eval(block string) string {
	translated, err := parley.Translate(block)
	if err != nil {
		return fmt.Sprintf("[parley error: %v]", err)
	}
	tmpl, err := utils.TemplateParse("parley-block", translated)
	if err != nil {
		return fmt.Sprintf("[parley parse error: %v]", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, t); err != nil {
		return fmt.Sprintf("[parley exec error: %v]", err)
	}
	return strings.TrimSpace(buf.String())
}

func (t *TemplateContext) listWithStyle(value any, style string) []string {
	style = strings.ToLower(style)
	switch v := value.(type) {
	case nil:
		return nil
	case []string:
		out := make([]string, 0, len(v))
		for _, item := range v {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			out = append(out, item)
		}
		return out
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			text := strings.TrimSpace(t.toString(item))
			if text == "" {
				continue
			}
			out = append(out, text)
		}
		return out
	case string:
		return parseListString(v, style)
	default:
		text := strings.TrimSpace(t.toString(v))
		if text == "" {
			return nil
		}
		return []string{text}
	}
}

func parseListString(input, style string) []string {
	input = strings.ReplaceAll(input, "\r\n", "\n")
	switch style {
	case "lines":
		lines := strings.Split(input, "\n")
		out := make([]string, 0, len(lines))
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			out = append(out, trimmed)
		}
		return out
	case "paragraphs":
		return splitParagraphs(input)
	default: // bullets
		lines := strings.Split(input, "\n")
		out := make([]string, 0, len(lines))
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			if !strings.HasPrefix(trimmed, "-") {
				continue
			}
			trimmed = strings.TrimSpace(trimmed[1:])
			if trimmed == "" {
				continue
			}
			out = append(out, trimmed)
		}
		return out
	}
}

func splitParagraphs(input string) []string {
	lines := strings.Split(input, "\n")
	paragraphs := []string{}
	current := []string{}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if len(current) > 0 {
				paragraphs = append(paragraphs, strings.Join(current, " "))
				current = nil
			}
			continue
		}
		current = append(current, trimmed)
	}
	if len(current) > 0 {
		paragraphs = append(paragraphs, strings.Join(current, " "))
	}
	return paragraphs
}

func (t *TemplateContext) Start(name string) string {
	if t.Run.Chat.IsValidStartAgent(name) {
		t.Run.Chat.SetStartAgent(name)
		return fmt.Sprintf("New Starting Agent: %s", name)
	}
	return fmt.Sprintf("Invalid Starting Agent: %s", name)
}

func (r *RunContext) CallAI(ctx context.Context, agent *agents.Agent, prompt string) (string, error) {
	cfg, err := config.Get()
	if err != nil {
		return "", err
	}

	provider, err := agents.GetAIProvider(cfg)
	if err != nil {
		return "", err
	}

	tools := []agents.Tool{}
	badListeners := []string{}
	for _, listenerName := range agent.Listeners {
		listenerAgent, err := r.Registry.LookupAgent(listenerName)
		if err != nil {
			return "", fmt.Errorf("error looking up listener agent %s: %w", listenerName, err)
		}
		if listenerAgent.Description == "" || len(listenerAgent.Inputs) == 0 {
			badListeners = append(badListeners, listenerName)
			continue
		}
		tools = append(tools, agents.BuildToolDefinition(listenerAgent))
	}

	if len(badListeners) > 0 {
		return "", fmt.Errorf("invalid listeners detected (missing description or input prompt): %s", strings.Join(badListeners, ", "))
	}

	messages := []agents.Message{
		{Role: agents.RoleUser, Content: prompt},
	}

	req := agents.AIRequest{
		Prompt:   prompt,
		Messages: messages,
		Tools:    tools,
	}

	resp, err := provider.Call(ctx, req)
	if err != nil {
		return "", err
	}

	r.PromptTokens += resp.PromptTokens
	r.CompletionTokens += resp.OutputTokens
	r.TotalTokens += resp.TotalTokens
	if r.Card != nil {
		r.Card.PromptTokens += resp.PromptTokens
		r.Card.CompletionTokens += resp.OutputTokens
		r.Card.TotalTokens += resp.TotalTokens
	}

	if len(resp.ToolCalls) > 0 {
		return r.handleToolCalls(ctx, provider, agent, prompt, tools, resp.ToolCalls, 1, messages)
	}

	return strings.TrimSpace(resp.Content), nil
}

func (r *RunContext) handleToolCalls(ctx context.Context, provider agents.AIProvider, agent *agents.Agent, prompt string, tools []agents.Tool, initialToolCalls []agents.ToolCall, depth int, history []agents.Message) (string, error) {
	if depth > 5 {
		return "", fmt.Errorf(
			"too many recursive tool call levels (depth=%d); possible infinite loop.",
			depth,
		)
	}

	// Append assistant message with tool calls to history
	history = append(history, agents.Message{
		Role:      agents.RoleAssistant,
		ToolCalls: initialToolCalls,
	})

	for _, toolCall := range initialToolCalls {
		agentName := toolCall.Name
		args := toolCall.Arguments

		res := r.CallAgent(ctx, agentName, args)
		if res.Error != nil {
			return "", fmt.Errorf("error handling tool callback for %s: %w", agentName, res.Error)
		}

		content := res.Output
		if strings.Contains(res.Output, "{{") && strings.Contains(res.Output, "}}") {
			tmpl, err := utils.TemplateParse(agentName, res.Output)
			if err != nil {
				return "", fmt.Errorf("error parsing template output from agent %s: %w", agentName, err)
			}
			var buf bytes.Buffer
			err = tmpl.Execute(&buf, &TemplateContext{
				UserInput: args,
				Run:       r,
				ctx:       ctx,
			})
			if err != nil {
				return "", fmt.Errorf("error executing template output from agent %s: %w", agentName, err)
			}
			content = buf.String()
		}

		if content == "" {
			content = " " // must be a non-nil string
		}

		history = append(history, agents.Message{
			Role:       agents.RoleTool,
			ToolCallID: toolCall.ID,
			ToolName:   toolCall.Name,
			Content:    content,
		})
	}

	req := agents.AIRequest{
		Prompt:   prompt,
		Messages: history,
		Tools:    tools,
	}

	resp, err := provider.Call(ctx, req)
	if err != nil {
		return "", err
	}

	r.PromptTokens += resp.PromptTokens
	r.CompletionTokens += resp.OutputTokens
	r.TotalTokens += resp.TotalTokens
	if r.Card != nil {
		r.Card.PromptTokens += resp.PromptTokens
		r.Card.CompletionTokens += resp.OutputTokens
		r.Card.TotalTokens += resp.TotalTokens
	}

	if len(resp.ToolCalls) > 0 {
		return r.handleToolCalls(ctx, provider, agent, prompt, tools, resp.ToolCalls, depth+1, history)
	}

	return strings.TrimSpace(resp.Content), nil
}
