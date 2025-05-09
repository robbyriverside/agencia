package agents

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/sashabaranov/go-openai"
)

type Argument struct {
	Type        string `yaml:"type"`
	Required    bool   `yaml:"required"`
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

type AgentContext interface {
	Get(name string, optionalInput ...string) string
	Input(optionalInput ...string) any
	Start(name string) string
}

type AgentFn func(ctx context.Context, input map[string]any, agent *Agent) (string, error)

type Fact struct {
	Name        string
	Description string
	Scope       string
	Type        string
	Tags        []string
}

type Agent struct {
	Name        string
	Description string
	Inputs      map[string]*Argument // field name -> Argument details
	Prompt      string
	Template    string
	Alias       string
	Function    AgentFn
	Listeners   []string
	Facts       map[string]*Fact
	Job         []string
	Role        string
}

// IsValid if the agent has only one of the following:
// - Function
// - Template
// - Prompt
// - Alias
// This is used to determine if the agent is valid for use in the registry.
func (r *Agent) IsValid() bool {
	var score int
	if r.Function != nil {
		score++
	}
	if r.Template != "" {
		score++
	}
	if r.Prompt != "" {
		score++
	}
	if r.Alias != "" {
		score++
	}
	return score == 1
}

var openaiClient *openai.Client

// var openaiInitError error
// var openaiInitialized bool

func GetOpenAIClient() (*openai.Client, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	org := os.Getenv("OPENAI_ORG")
	if apiKey == "" {
		return nil, errors.New("OPENAI_API_KEY must be set")
	}
	config := openai.DefaultConfig(apiKey)
	config.OrgID = org
	openaiClient = openai.NewClientWithConfig(config)
	return openaiClient, nil
}

// CallOpenAI calls the OpenAI API with the given prompt and returns the response.
// Used by library agents to call OpenAI.
func CallOpenAI(ctx context.Context, prompt string) (string, error) {
	client, err := GetOpenAIClient()
	if err != nil {
		return "[MOCK ERROR: attempted real OpenAI call in test/mock mode]", err
	}
	req := openai.ChatCompletionRequest{
		Model:       openai.GPT4o,
		Temperature: 0.2,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleUser, Content: prompt},
		},
	}
	resp, err := client.CreateChatCompletion(ctx, req)
	if err != nil {
		return "", fmt.Errorf("OpenAI API error: %w", err)
	}
	return strings.TrimSpace(resp.Choices[0].Message.Content), nil
}
