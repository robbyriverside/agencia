package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/generative-ai-go/genai"
	"github.com/robbyriverside/agencia/config"
	"google.golang.org/api/option"
)

type GeminiProvider struct {
	cfg config.GeminiConfig
}

func NewGeminiProvider(cfg config.GeminiConfig) *GeminiProvider {
	return &GeminiProvider{
		cfg: cfg,
	}
}

func (p *GeminiProvider) Call(ctx context.Context, request AIRequest) (AIResponse, error) {
	if !p.cfg.Enabled {
		return AIResponse{}, fmt.Errorf("Gemini usage disabled via configuration")
	}

	apiKey := p.cfg.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("GEMINI_API_KEY")
	}
	if apiKey == "" {
		return AIResponse{}, fmt.Errorf("GEMINI_API_KEY not set")
	}

	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return AIResponse{}, fmt.Errorf("failed to create Gemini client: %w", err)
	}
	defer client.Close()

	model := client.GenerativeModel(p.cfg.Model)
	if p.cfg.Temperature != nil {
		model.SetTemperature(*p.cfg.Temperature)
	}
	if p.cfg.MaxTokens > 0 {
		model.SetMaxOutputTokens(int32(p.cfg.MaxTokens))
	}

	// Convert Tools
	if len(request.Tools) > 0 {
		model.Tools = []*genai.Tool{
			{
				FunctionDeclarations: p.convertTools(request.Tools),
			},
		}
	}

	// Convert Messages
	// Gemini uses a different message structure (Content with Parts)
	// We need to reconstruct the history

	// The last message is the prompt for GenerateContent, the rest is history
	// But Gemini chat session handles history better.
	// However, the interface is stateless "Call".
	// So we need to pass all history to GenerateContent if we don't use ChatSession.
	// But GenerateContent expects a list of Parts for the *current* turn?
	// No, GenerateContent takes parts.
	// For chat history, we should use StartChat and send the history.

	var resp *genai.GenerateContentResponse
	// err is already declared above

	attempts := p.cfg.RetryAttempts + 1
	backoff := p.cfg.RetryInitialBackoff.Duration()
	maxBackoff := p.cfg.RetryMaxBackoff.Duration()

	for i := 0; i < attempts; i++ {
		if len(request.Messages) > 0 {
			// ... (existing chat logic) ...
			// We need to recreate the chat session for each attempt if we want to be safe,
			// or just reuse it. Since we are stateless here, we recreate it.

			// Re-create client/model logic inside loop?
			// No, client/model is created outside.
			// But `cs := model.StartChat()` creates a new session.

			cs := model.StartChat()

			// Re-construct history
			lastMsg := request.Messages[len(request.Messages)-1]
			historyMsgs := request.Messages[:len(request.Messages)-1]
			cs.History = p.convertHistory(historyMsgs)

			if lastMsg.Role == RoleUser {
				resp, err = cs.SendMessage(ctx, genai.Text(lastMsg.Content))
			} else if lastMsg.Role == RoleTool {
				// Re-construct history including last tool response
				fullHistory := p.convertHistory(request.Messages)
				cs.History = fullHistory[:len(fullHistory)-1]
				lastContent := fullHistory[len(fullHistory)-1]
				resp, err = cs.SendMessage(ctx, lastContent.Parts...)
			} else {
				resp, err = cs.SendMessage(ctx, genai.Text(lastMsg.Content))
			}
		} else {
			if request.Prompt != "" {
				resp, err = model.GenerateContent(ctx, genai.Text(request.Prompt))
			} else {
				return AIResponse{}, fmt.Errorf("empty request")
			}
		}

		if err == nil {
			return p.convertResponse(resp), nil
		}

		// Check for retryable error (429 or 5xx)
		// googleapi errors usually contain the status code
		if isRetryableError(err) {
			if i < attempts-1 {
				select {
				case <-ctx.Done():
					return AIResponse{}, ctx.Err()
				case <-time.After(backoff):
					backoff *= 2
					if maxBackoff > 0 && backoff > maxBackoff {
						backoff = maxBackoff
					}
					continue
				}
			}
		}

		// If not retryable or out of attempts, return error
		return AIResponse{}, err
	}
	return AIResponse{}, err
}

func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	// Check for 429
	if strings.Contains(err.Error(), "429") {
		return true
	}
	// Check for 5xx
	if strings.Contains(err.Error(), "500") || strings.Contains(err.Error(), "503") {
		return true
	}
	// Check for "quota exceeded" text which is common in 429s from Gemini
	if strings.Contains(strings.ToLower(err.Error()), "quota exceeded") {
		return true
	}
	return false
}

func (p *GeminiProvider) convertHistory(messages []Message) []*genai.Content {
	var history []*genai.Content
	for _, msg := range messages {
		role := "user"
		if msg.Role == RoleAssistant {
			role = "model"
		} else if msg.Role == RoleTool {
			// Gemini (via genai library or API version) seems to reject 'function' role in history
			// with "Please use a valid role: user, model".
			// Using 'user' role for tool outputs works and allows the model to continue.
			role = "user"
		} else if msg.Role == RoleSystem {
			role = "user" // Gemini doesn't have system role in chat history usually, or it's handled differently.
			// For now map system to user or prepend to first user message.
		}

		parts := []genai.Part{}

		if msg.Content != "" {
			parts = append(parts, genai.Text(msg.Content))
		}

		if len(msg.ToolCalls) > 0 {
			for _, tc := range msg.ToolCalls {
				parts = append(parts, genai.FunctionCall{
					Name: tc.Name,
					Args: p.unmarshalArgs(tc.Arguments),
				})
			}
		}

		// If it's a tool response (RoleTool)
		if msg.Role == RoleTool {
			// In Gemini, a tool response is a FunctionResponse part
			// It needs the name of the function and the response
			// The Message struct has ToolCallID, but Gemini uses function name mapping usually?
			// Wait, genai.FunctionResponse has Name and Response.
			// It seems it relies on Name.
			// OpenAI uses ID.
			// This is a mismatch. We might need to look up the name from previous messages if not provided.
			// OR we assume the generic Message for Tool response includes the Name?
			// The current Message struct has ToolCallID.
			// We might need to update Message struct to include ToolName for Tool responses to support Gemini easily.

			// For now, let's assume we can't easily get the name without tracking.
			// But wait, the user of the provider (RunContext) knows the name.
			// Let's update the Message struct in ai_provider.go to include ToolName for tool responses.

			toolName := msg.ToolName
			if toolName == "" {
				toolName = "UNKNOWN_FUNCTION"
			}

			parts = append(parts, genai.FunctionResponse{
				Name: toolName,
				Response: map[string]any{
					"content": msg.Content,
				},
			})
		}

		history = append(history, &genai.Content{
			Role:  role,
			Parts: parts,
		})
	}
	return history
}

func (p *GeminiProvider) convertTools(tools []Tool) []*genai.FunctionDeclaration {
	out := make([]*genai.FunctionDeclaration, len(tools))
	for i, t := range tools {
		out[i] = &genai.FunctionDeclaration{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  p.convertSchema(t.Parameters),
		}
	}
	return out
}

func (p *GeminiProvider) convertSchema(params map[string]interface{}) *genai.Schema {
	// Simple conversion. Gemini schema is similar to OpenAPI schema.
	// We might need a recursive conversion if it's complex.
	// For now, let's assume it's a simple object with properties.

	// This is a simplification. A robust implementation would do full recursion.
	return &genai.Schema{
		Type:       genai.TypeObject,
		Properties: p.convertProperties(params["properties"]),
		Required:   p.convertRequired(params["required"]),
	}
}

func (p *GeminiProvider) convertProperties(props interface{}) map[string]*genai.Schema {
	out := make(map[string]*genai.Schema)
	if m, ok := props.(map[string]interface{}); ok {
		for k, v := range m {
			if vm, ok := v.(map[string]interface{}); ok {
				t := genai.TypeString
				if typeStr, ok := vm["type"].(string); ok {
					switch typeStr {
					case "string":
						t = genai.TypeString
					case "integer":
						t = genai.TypeInteger
					case "number":
						t = genai.TypeNumber
					case "boolean":
						t = genai.TypeBoolean
					case "array":
						t = genai.TypeArray
					case "object":
						t = genai.TypeObject
					}
				}
				out[k] = &genai.Schema{
					Type:        t,
					Description: vm["description"].(string),
				}
			}
		}
	}
	return out
}

func (p *GeminiProvider) convertRequired(req interface{}) []string {
	if s, ok := req.([]string); ok {
		return s
	}
	return nil
}

func (p *GeminiProvider) convertResponse(resp *genai.GenerateContentResponse) AIResponse {
	content := ""
	var toolCalls []ToolCall

	for _, cand := range resp.Candidates {
		if cand.Content != nil {
			for _, part := range cand.Content.Parts {
				if txt, ok := part.(genai.Text); ok {
					content += string(txt)
				}
				if fc, ok := part.(genai.FunctionCall); ok {
					argsBytes, _ := json.Marshal(fc.Args)
					toolCalls = append(toolCalls, ToolCall{
						ID:        "gemini_call_" + fc.Name, // Gemini doesn't give IDs?
						Name:      fc.Name,
						Arguments: string(argsBytes),
					})
				}
			}
		}
	}

	// Token counts
	promptTokens := 0
	outputTokens := 0
	if resp.UsageMetadata != nil {
		promptTokens = int(resp.UsageMetadata.PromptTokenCount)
		outputTokens = int(resp.UsageMetadata.CandidatesTokenCount)
	}

	return AIResponse{
		Content:      content,
		ToolCalls:    toolCalls,
		PromptTokens: promptTokens,
		OutputTokens: outputTokens,
		TotalTokens:  promptTokens + outputTokens,
	}
}

func (p *GeminiProvider) unmarshalArgs(args string) map[string]any {
	var m map[string]any
	json.Unmarshal([]byte(args), &m)
	return m
}
