package agencia

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/robbyriverside/agencia/agents"
	"github.com/robbyriverside/agencia/utils"
	"github.com/sashabaranov/go-openai"
	"gopkg.in/yaml.v3"
)

type TemplateContext struct {
	Agent     *agents.Agent
	UserInput string
	inputMap  map[string]any
	Run       *RunContext
	ctx       context.Context
}

func NewTemplateContext(ctx context.Context, agent *agents.Agent, input string, run *RunContext, inputMap map[string]any) *TemplateContext {
	return &TemplateContext{
		Agent:     agent,
		UserInput: input,
		Run:       run,
		inputMap:  inputMap,
		ctx:       ctx,
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

func (t *TemplateContext) Start(name string) string {
	if t.Run.Chat.IsValidStartAgent(name) {
		t.Run.Chat.SetStartAgent(name)
		return fmt.Sprintf("New Starting Agent: %s", name)
	}
	return fmt.Sprintf("Invalid Starting Agent: %s", name)
}

func (r *RunContext) CallAI(ctx context.Context, agent *agents.Agent, prompt string) (string, error) {
	return r.CallOpenAI(ctx, agent, prompt)
}

func (r *RunContext) CallOpenAI(ctx context.Context, agent *agents.Agent, prompt string) (string, error) {
	client, err := agents.GetOpenAIClient()
	if err != nil {
		return "", err
	}
	tools := []openai.Tool{}
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
		paramSchema := buildToolParameters(listenerAgent)
		tools = append(tools, openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        listenerName,
				Description: listenerAgent.Description,
				Parameters:  paramSchema,
			},
		})
	}

	if len(badListeners) > 0 {
		return "", fmt.Errorf("invalid listeners detected (missing description or input prompt): %s", strings.Join(badListeners, ", "))
	}

	req := openai.ChatCompletionRequest{
		Model:       openai.GPT4o,
		Temperature: 0.2,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleUser, Content: prompt},
		},
		Tools:      tools,
		ToolChoice: nil,
	}
	resp, err := client.CreateChatCompletion(ctx, req)
	if err != nil {
		return "", fmt.Errorf("OpenAI API error: %w", err)
	}

	if len(resp.Choices) > 0 && len(resp.Choices[0].Message.ToolCalls) > 0 {
		return r.handleToolCalls(ctx, prompt, tools, resp.Choices[0].Message.ToolCalls, 1, []string{})
	}

	return strings.TrimSpace(resp.Choices[0].Message.Content), nil
}

func (r *RunContext) handleToolCalls(ctx context.Context, prompt string, tools []openai.Tool, initialToolCalls []openai.ToolCall, depth int, trace []string) (string, error) {
	if depth > 5 {
		return "", fmt.Errorf(
			"too many recursive tool call levels (depth=%d); possible infinite loop.\nTrace:\n%s",
			depth,
			strings.Join(trace, "\n"),
		)
	}

	messages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: prompt},
		{Role: openai.ChatMessageRoleAssistant, ToolCalls: initialToolCalls},
	}

	functionResults := []openai.ChatCompletionMessage{}
	for _, toolCall := range initialToolCalls {
		if toolCall.Type == "function" {
			agentName := toolCall.Function.Name
			args := toolCall.Function.Arguments
			traceEntry := fmt.Sprintf("Depth %d: called tool %s with args %s", depth, agentName, args)
			trace = append(trace, traceEntry)
			res := r.CallAgent(ctx, agentName, args)
			if res.Error != nil {
				return "", fmt.Errorf("error handling tool callback for %s: %w", agentName, res.Error)
			}
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
				functionResults = append(functionResults, openai.ChatCompletionMessage{
					Role:       openai.ChatMessageRoleTool,
					ToolCallID: toolCall.ID,
					Content:    buf.String(),
				})
			} else {
				outputContent := res.Output
				if outputContent == "" {
					outputContent = " " // must be a non-nil string to satisfy OpenAI API
				}
				functionResults = append(functionResults, openai.ChatCompletionMessage{
					Role:       openai.ChatMessageRoleTool,
					ToolCallID: toolCall.ID,
					Content:    outputContent,
				})
			}
		}
	}
	messages = append(messages, functionResults...)

	contReq := openai.ChatCompletionRequest{
		Model:       openai.GPT4o,
		Temperature: 0.2,
		Messages:    messages,
		Tools:       tools,
	}

	client, err := agents.GetOpenAIClient()
	if err != nil {
		return "", err
	}

	contResp, err := client.CreateChatCompletion(ctx, contReq)
	if err != nil {
		return "", fmt.Errorf("OpenAI API error on continuation: %w", err)
	}

	if len(contResp.Choices) > 0 {
		choice := contResp.Choices[0]
		if len(choice.Message.ToolCalls) > 0 {
			if depth+1 > 5 {
				return "", fmt.Errorf(
					"too many recursive tool call levels (depth=%d); possible infinite loop.\nTrace:\n%s",
					depth+1,
					strings.Join(trace, "\n"),
				)
			}
			// Recursive: new tool calls need handling
			return r.handleToolCalls(ctx, prompt, tools, choice.Message.ToolCalls, depth+1, trace)
		}
		return strings.TrimSpace(choice.Message.Content), nil
	}
	return "", errors.New("no choices returned from continuation OpenAI call")
}

func buildToolParameters(agent *agents.Agent) map[string]interface{} {
	paramSchema := map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
		"required":   []string{},
	}

	for fieldName, arg := range agent.Inputs {
		properties := paramSchema["properties"].(map[string]interface{})
		argType := arg.Type
		if argType == "" {
			argType = "string"
		}
		properties[fieldName] = map[string]interface{}{
			"type":        argType,
			"description": arg.Description,
		}
		isRequired := true
		if !arg.Required {
			isRequired = false
		}
		if isRequired {
			paramSchema["required"] = append(paramSchema["required"].([]string), fieldName)
		}
	}

	return paramSchema
}
