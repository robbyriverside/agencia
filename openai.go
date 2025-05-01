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
)

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

func (r *Registry) CallAI(ctx context.Context, agent *agents.Agent, prompt string, input any) (string, error) {
	tc, _ := input.(*TemplateContext)
	if tmpl, ok := agents.MockTemplates[agent.Name]; ok {
		var buf bytes.Buffer
		err := tmpl.Execute(&buf, tc)
		if err != nil {
			return "", fmt.Errorf("error executing mock template: %w", err)
		}
		return "MOCK: " + buf.String(), nil
	}
	return r.CallOpenAI(ctx, agent, prompt)
}

func (r *Registry) CallOpenAI(ctx context.Context, agent *agents.Agent, prompt string) (string, error) {
	client, err := agents.GetOpenAIClient()
	if err != nil {
		return "", err
	}
	tools := []openai.Tool{}
	badListeners := []string{}
	for _, listenerName := range agent.Listeners {
		listenerAgent, err := r.LookupAgent(listenerName)
		if err != nil {
			return "", fmt.Errorf("error looking up listener agent %s: %w", listenerName, err)
		}
		if listenerAgent.Description == "" || len(listenerAgent.InputPrompt) == 0 {
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
		Model: openai.GPT4o,
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

func (r *Registry) handleToolCalls(ctx context.Context, prompt string, tools []openai.Tool, initialToolCalls []openai.ToolCall, depth int, trace []string) (string, error) {
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
					Input:    args,
					Registry: *r,
					ctx:      ctx,
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
		Model:    openai.GPT4o,
		Messages: messages,
		Tools:    tools,
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

	for fieldName, arg := range agent.InputPrompt {
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
