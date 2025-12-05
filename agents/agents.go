package agents

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/robbyriverside/agencia/config"
	"github.com/robbyriverside/agencia/logs"
	"github.com/sashabaranov/go-openai"
)

type Argument struct {
	Type        string `yaml:"type"`
	Required    bool   `yaml:"required"`
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

type AgentFn func(ctx context.Context, input map[string]any, agent *Agent) (string, error)

type Fact struct {
	Name        string
	Description string
	Add         bool // TODO: add to existing fact or replace it (list and string append, number addition, bool NA)
	Scope       string
	Type        string
}

func (f *Fact) EmptyDefault() any {
	if f.Type == "string" {
		return ""
	}
	if f.Type == "int" {
		return 0
	}
	if f.Type == "float" {
		return 0.0
	}
	if f.Type == "bool" {
		return false
	}
	if f.Type == "list" {
		return []any{}
	}
	return nil
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
	Role        string // TODO: apply role to agent
}

type AgentRole struct {
	Name         string
	ID           string
	Description  string
	Personality  string
	Performance  string
	Observations map[string]string // key -> observation
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
	cfg, err := config.Get()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	if !cfg.OpenAI.Enabled {
		return nil, errors.New("OpenAI usage disabled via configuration")
	}

	apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if apiKey == "" {
		return nil, errors.New("OPENAI_API_KEY must be set")
	}

	openAIConfig := openai.DefaultConfig(apiKey)
	if base := strings.TrimSpace(cfg.OpenAI.APIBase); base != "" {
		openAIConfig.BaseURL = strings.TrimRight(base, "/")
	}
	if org := strings.TrimSpace(cfg.OpenAI.Organization); org != "" {
		openAIConfig.OrgID = org
	} else {
		openAIConfig.OrgID = strings.TrimSpace(os.Getenv("OPENAI_ORG"))
	}
	if timeout := cfg.OpenAI.RequestTimeout.Duration(); timeout > 0 {
		openAIConfig.HTTPClient = &http.Client{Timeout: timeout}
	}
	openaiClient = openai.NewClientWithConfig(openAIConfig)
	return openaiClient, nil
}

// CallOpenAI calls the OpenAI API with the given prompt and returns the response.
// Used by library agents to call OpenAI.
func CallOpenAI(ctx context.Context, prompt string) (string, error) {
	cfg, err := config.Get()
	if err != nil {
		return "", fmt.Errorf("load config: %w", err)
	}
	if !cfg.OpenAI.Enabled {
		return "", errors.New("OpenAI usage disabled via configuration")
	}

	requestCtx := ctx
	var cancel context.CancelFunc
	if timeout := cfg.OpenAI.RequestTimeout.Duration(); timeout > 0 {
		requestCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	if cancel != nil {
		defer cancel()
	}

	release, err := AcquireOpenAISlot(requestCtx, cfg)
	if err != nil {
		return "", err
	}
	defer release()

	client, err := GetOpenAIClient()
	if err != nil {
		return "", err
	}

	messages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleUser, Content: prompt},
	}
	req := BuildChatCompletionRequest(cfg.OpenAI, messages, nil)

	if cfg.OpenAI.LogPrompts {
		logs.Infof("[openai] prompt: %s", prompt)
	}

	resp, err := CallChatCompletionWithRetry(requestCtx, client, req, cfg.OpenAI)
	if err != nil {
		return "", fmt.Errorf("OpenAI API error: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "", errors.New("OpenAI API returned no choices")
	}

	output := strings.TrimSpace(resp.Choices[0].Message.Content)
	if cfg.OpenAI.LogPrompts {
		logs.Infof("[openai] response: %s", truncateForLog(output, 800))
	}
	return output, nil
}

func truncateForLog(s string, limit int) string {
	if limit <= 0 || len(s) <= limit {
		return s
	}
	return fmt.Sprintf("%s...(%d chars truncated)", s[:limit], len(s)-limit)
}

func GetAIProvider(cfg *config.Config) (AIProvider, error) {
	switch cfg.Vendor {
	case config.VendorOpenAI:
		return NewOpenAIProvider(cfg.OpenAI), nil
	case config.VendorGemini:
		return NewGeminiProvider(cfg.Gemini), nil
	default:
		return nil, fmt.Errorf("unknown vendor: %s", cfg.Vendor)
	}
}
