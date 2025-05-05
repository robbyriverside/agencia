package agents

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"text/template"

	"github.com/sashabaranov/go-openai"
)

type Argument struct {
	Type        string `yaml:"type"`
	Required    bool   `yaml:"required"`
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

type AgentFn func(ctx context.Context, input map[string]any) (string, error)

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
	Procedure   []string
}

var MockTemplates = map[string]*template.Template{}
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

func CallOpenAI(ctx context.Context, prompt string) (string, error) {
	client, err := GetOpenAIClient()
	if err != nil {
		return "[MOCK ERROR: attempted real OpenAI call in test/mock mode]", err
	}
	req := openai.ChatCompletionRequest{
		Model: openai.GPT4o,
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
