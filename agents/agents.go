package agents

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"text/template"

	"github.com/robbyriverside/agencia/utils"
	"github.com/sashabaranov/go-openai"
	"gopkg.in/yaml.v2"
)

type AgentFn func(ctx context.Context, input map[string]any) (string, error)

type Agent struct {
	Name        string
	Description string
	InputPrompt map[string]string // field -> description
	Prompt      string
	Template    string
	Alias       string
	Function    AgentFn
	Listeners   []string
}

var MockTemplates = map[string]*template.Template{}
var openaiClient *openai.Client
var openaiInitError error
var openaiInitialized bool

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

func loadMockResponses(path string) error {
	type MockSpec struct {
		Responses map[string]string `yaml:"responses"`
	}
	var mock MockSpec
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("cannot read mock file %s: %w", path, err)
	}
	err = yaml.Unmarshal(data, &mock)
	if err != nil {
		return fmt.Errorf("invalid YAML in mock file %s: %w", path, err)
	}
	for name, tmplStr := range mock.Responses {
		tmpl, err := utils.TemplateParse(name, tmplStr)
		if err != nil {
			return fmt.Errorf("error parsing mock template for %s: %w", name, err)
		}
		MockTemplates[name] = tmpl
	}
	return nil
}

func ConfigureAI(ctx context.Context, mockfile string) error {
	if mockfile != "" {
		if err := loadMockResponses(mockfile); err != nil {
			return fmt.Errorf("[MOCK ERROR] %w", err)
		}
	} else {
		apiKey := os.Getenv("OPENAI_API_KEY")
		org := os.Getenv("OPENAI_ORG")
		if apiKey == "" {
			return errors.New("OPENAI_API_KEY must be set")
		}
		config := openai.DefaultConfig(apiKey)
		config.OrgID = org
		openaiClient = openai.NewClientWithConfig(config)
	}
	return nil
}

func CallOpenAI(ctx context.Context, prompt string) (string, error) {
	client, err := GetOpenAIClient()
	if err != nil {
		return "[MOCK ERROR: attempted real OpenAI call in test/mock mode]", err
	}
	req := openai.ChatCompletionRequest{
		Model: openai.GPT4,
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
