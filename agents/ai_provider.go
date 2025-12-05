package agents

import (
	"context"
)

// AIProvider defines the interface for interacting with different AI vendors.
type AIProvider interface {
	Call(ctx context.Context, request AIRequest) (AIResponse, error)
}

// AIRequest represents a generic request to an AI provider.
type AIRequest struct {
	Prompt   string
	Messages []Message
	Tools    []Tool
}

// AIResponse represents a generic response from an AI provider.
type AIResponse struct {
	Content      string
	ToolCalls    []ToolCall
	PromptTokens int
	OutputTokens int
	TotalTokens  int
}

// Message represents a chat message.
type Message struct {
	Role       string
	Content    string
	ToolCalls  []ToolCall
	ToolCallID string // For tool responses
	ToolName   string // For tool responses (required by Gemini)
}

// Tool represents a tool/function that the AI can call.
type Tool struct {
	Name        string
	Description string
	Parameters  map[string]interface{}
}

// ToolCall represents a request from the AI to call a tool.
type ToolCall struct {
	ID        string
	Name      string
	Arguments string
}

const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleSystem    = "system"
	RoleTool      = "tool"
)
