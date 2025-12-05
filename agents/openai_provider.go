package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/robbyriverside/agencia/config"
	"github.com/robbyriverside/agencia/logs"
	"github.com/sashabaranov/go-openai"
)

type OpenAIProvider struct {
	cfg config.OpenAIConfig
}

func NewOpenAIProvider(cfg config.OpenAIConfig) *OpenAIProvider {
	return &OpenAIProvider{
		cfg: cfg,
	}
}

func (p *OpenAIProvider) Call(ctx context.Context, request AIRequest) (AIResponse, error) {
	if !p.cfg.Enabled {
		return AIResponse{}, fmt.Errorf("OpenAI usage disabled via configuration")
	}

	client, err := GetOpenAIClient()
	if err != nil {
		return AIResponse{}, err
	}

	messages := p.convertMessages(request.Messages)
	tools := p.convertTools(request.Tools)

	req := BuildChatCompletionRequest(p.cfg, messages, tools)

	if p.cfg.LogPrompts {
		logs.Infof("[openai] request: %s", formatMessagesForLog(messages))
	}

	// Acquire slot and call with retry
	release, err := AcquireOpenAISlot(ctx, &config.Config{OpenAI: p.cfg}) // Temporary hack to reuse AcquireOpenAISlot
	if err != nil {
		return AIResponse{}, err
	}
	defer release()

	resp, err := CallChatCompletionWithRetry(ctx, client, req, p.cfg)
	if err != nil {
		return AIResponse{}, fmt.Errorf("OpenAI API error: %w", err)
	}

	if p.cfg.LogPrompts {
		logs.Infof("[openai] response: %s", formatResponseForLog(resp))
	}

	if len(resp.Choices) == 0 {
		return AIResponse{}, fmt.Errorf("OpenAI API returned no choices")
	}

	choice := resp.Choices[0]

	return AIResponse{
		Content:      choice.Message.Content,
		ToolCalls:    p.convertToolCalls(choice.Message.ToolCalls),
		PromptTokens: resp.Usage.PromptTokens,
		OutputTokens: resp.Usage.CompletionTokens,
		TotalTokens:  resp.Usage.TotalTokens,
	}, nil
}

func (p *OpenAIProvider) convertMessages(messages []Message) []openai.ChatCompletionMessage {
	out := make([]openai.ChatCompletionMessage, len(messages))
	for i, msg := range messages {
		role := msg.Role
		if role == RoleTool {
			role = openai.ChatMessageRoleTool
		}

		out[i] = openai.ChatCompletionMessage{
			Role:       role,
			Content:    msg.Content,
			ToolCallID: msg.ToolCallID,
		}
		if len(msg.ToolCalls) > 0 {
			out[i].ToolCalls = make([]openai.ToolCall, len(msg.ToolCalls))
			for j, tc := range msg.ToolCalls {
				out[i].ToolCalls[j] = openai.ToolCall{
					ID:   tc.ID,
					Type: openai.ToolTypeFunction,
					Function: openai.FunctionCall{
						Name:      tc.Name,
						Arguments: tc.Arguments,
					},
				}
			}
		}
	}
	return out
}

func (p *OpenAIProvider) convertTools(tools []Tool) []openai.Tool {
	out := make([]openai.Tool, len(tools))
	for i, t := range tools {
		out[i] = openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		}
	}
	return out
}

func (p *OpenAIProvider) convertToolCalls(toolCalls []openai.ToolCall) []ToolCall {
	out := make([]ToolCall, len(toolCalls))
	for i, tc := range toolCalls {
		out[i] = ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		}
	}
	return out
}

// Helper functions for logging (copied/adapted from openai.go)
func formatMessagesForLog(messages []openai.ChatCompletionMessage) string {
	parts := make([]string, 0, len(messages))
	for _, msg := range messages {
		if len(msg.ToolCalls) > 0 {
			names := make([]string, 0, len(msg.ToolCalls))
			for _, call := range msg.ToolCalls {
				names = append(names, call.Function.Name)
			}
			parts = append(parts, fmt.Sprintf("%s:tool-calls(%s)", msg.Role, strings.Join(names, ",")))
			continue
		}
		parts = append(parts, fmt.Sprintf("%s:%s", msg.Role, truncateForLog(msg.Content, 160)))
	}
	return strings.Join(parts, " | ")
}

func formatResponseForLog(resp openai.ChatCompletionResponse) string {
	if len(resp.Choices) == 0 {
		return "no choices"
	}
	choice := resp.Choices[0]
	if len(choice.Message.ToolCalls) > 0 {
		names := make([]string, 0, len(choice.Message.ToolCalls))
		for _, call := range choice.Message.ToolCalls {
			names = append(names, call.Function.Name)
		}
		return fmt.Sprintf("tool_calls=%s", strings.Join(names, ","))
	}
	return truncateForLog(strings.TrimSpace(choice.Message.Content), 200)
}

func buildToolParameters(agent *Agent) map[string]interface{} {
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

// BuildToolDefinition creates a Tool struct from an Agent definition
func BuildToolDefinition(agent *Agent) Tool {
	return Tool{
		Name:        agent.Name,
		Description: agent.Description,
		Parameters:  buildToolParameters(agent),
	}
}

// MarshalArguments converts a map of arguments to a JSON string
func MarshalArguments(args map[string]any) string {
	b, _ := json.Marshal(args)
	return string(b)
}
