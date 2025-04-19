package agencia

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"text/template"

	"github.com/sashabaranov/go-openai"
	"gopkg.in/yaml.v3"
)

var mockTemplates = map[string]*template.Template{}
var openaiClient *openai.Client

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
		tmpl, err := template.New(name).Parse(tmplStr)
		if err != nil {
			return fmt.Errorf("error parsing mock template for %s: %w", name, err)
		}
		mockTemplates[name] = tmpl
	}
	return nil
}

func callAI(ctx context.Context, agentName, prompt string, input *TemplateContext) (string, error) {
	if tmpl, ok := mockTemplates[agentName]; ok {
		var buf bytes.Buffer
		err := tmpl.Execute(&buf, input)
		if err != nil {
			return "", fmt.Errorf("error executing mock template: %w", err)
		}
		return buf.String(), nil
	}
	return callOpenAI(ctx, prompt)
}

func callOpenAI(ctx context.Context, prompt string) (string, error) {
	req := openai.ChatCompletionRequest{
		Model: openai.GPT4,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleUser, Content: prompt},
		},
	}
	resp, err := openaiClient.CreateChatCompletion(ctx, req)
	if err != nil {
		return "", fmt.Errorf("OpenAI API error: %w", err)
	}
	return strings.TrimSpace(resp.Choices[0].Message.Content), nil
}
