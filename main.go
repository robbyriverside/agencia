package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"text/template"

	"github.com/jessevdk/go-flags"
	"github.com/joho/godotenv"
	"github.com/sashabaranov/go-openai"
	"gopkg.in/yaml.v3"
)

type AgentSpec struct {
	Agents     []map[string]any `yaml:"agents,omitempty"`
	Formatters []map[string]any `yaml:"formatters,omitempty"`
}

func (s *AgentSpec) String() string {
	b, _ := yaml.Marshal(s)
	return string(b)
}

type Agent struct {
	Name        string
	Prompt      string
	Function    string
	IsFormatter bool
}

type AgentResult struct {
	Output    string
	Ran       bool
	Error     error
	AgentName string
}

var agentRegistry = map[string]Agent{}
var mockTemplates = map[string]*template.Template{}
var openaiClient *openai.Client

type RunCommand struct {
	Name  string `short:"n" long:"name" required:"true" description:"Agent name to run"`
	Input string `short:"i" long:"input" required:"true" description:"Input string"`
	File  string `short:"f" long:"file" default:"agentic.yaml" description:"Agent definition YAML file"`
	Mock  string `short:"m" long:"mock" description:"Path to mock response YAML file"`
}

func (r *RunCommand) Execute(args []string) error {
	_ = godotenv.Load()
	ctx := context.Background()

	spec, err := loadAgentSpec(r.File)
	if err != nil {
		return fmt.Errorf("[LOAD ERROR] %w", err)
	}
	registerAgents(spec)

	if r.Mock != "" {
		if err := loadMockResponses(r.Mock); err != nil {
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

	res := CallAgent(ctx, r.Name, r.Input)
	if res.Error != nil {
		return fmt.Errorf("[AGENT ERROR] %v", res.Error)
	}
	if !res.Ran {
		fmt.Printf("[INFO] Agent '%s' did not run (empty or skipped).\n", res.AgentName)
		return nil
	}
	// fmt.Printf("[Agent: %s]\nOutput:\n%s\n", res.AgentName, res.Output)
	fmt.Println(res.Output)
	return nil
}

func main() {
	parser := flags.NewParser(nil, flags.Default)
	parser.AddCommand("run", "Run an agent", "Execute a named agent with input", &RunCommand{})
	if _, err := parser.Parse(); err != nil {
		log.Fatalf("[FATAL] CLI error: %v", err)
	}
}

func loadAgentSpec(filename string) (AgentSpec, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return AgentSpec{}, fmt.Errorf("cannot read agent file %s: %w", filename, err)
	}
	// fmt.Printf("[INFO] Loading agent file %s\n", filename)
	var spec AgentSpec
	err = yaml.Unmarshal(data, &spec)
	if err != nil {
		return AgentSpec{}, fmt.Errorf("invalid YAML in %s: %w", filename, err)
	}
	return spec, nil
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

func registerAgents(spec AgentSpec) {
	// fmt.Println("[INFO] Registering agents...", spec)
	if spec.Agents != nil {
		for _, entry := range spec.Agents {
			for name, raw := range entry {
				switch v := raw.(type) {
				case string:
					agentRegistry[name] = Agent{Name: name, Prompt: v}
				case map[string]any:
					ag := Agent{Name: name}
					if fn, ok := v["function"].(string); ok {
						ag.Function = fn
					}
					agentRegistry[name] = ag
				}
			}
		}
	}
	if spec.Formatters != nil {
		for _, entry := range spec.Formatters {
			for name, raw := range entry {
				switch v := raw.(type) {
				case string:
					agentRegistry[name] = Agent{Name: name, Prompt: v, IsFormatter: true}
				case map[string]any:
					ag := Agent{Name: name, IsFormatter: true}
					if p, ok := v["prompt"].(string); ok {
						ag.Prompt = p
					}
					agentRegistry[name] = ag
				}
			}
		}
	}
	if len(agentRegistry) == 0 {
		log.Fatal("no agents defined")
	}
}

type TemplateContext struct {
	Input string
	ctx   context.Context
}

func (t *TemplateContext) Get(name string, optionalInput ...string) string {
	input := t.Input
	if len(optionalInput) > 0 {
		input = optionalInput[0]
	}
	res := CallAgent(t.ctx, name, input)
	if res.Error != nil {
		return fmt.Sprintf("[error calling %s: %v]", name, res.Error)
	}
	return res.Output
}

func CallAgent(ctx context.Context, name string, input string) AgentResult {
	agent, ok := agentRegistry[name]
	if !ok {
		return AgentResult{Ran: false, Error: fmt.Errorf("agent not found: %s", name), AgentName: name}
	}

	if agent.IsFormatter {
		// fmt.Printf("------ [Formatter: %s]\nPrompt:\n%s\n---\n", name, agent.Prompt)
		tmpl, err := template.New(name).Parse(agent.Prompt)
		if err != nil {
			return AgentResult{Ran: false, Error: fmt.Errorf("template parse error: %w", err), AgentName: name}
		}
		var buf bytes.Buffer
		err = tmpl.Execute(&buf, &TemplateContext{Input: input, ctx: ctx})
		if err != nil {
			return AgentResult{Ran: false, Error: fmt.Errorf("template exec error: %w", err), AgentName: name}
		}
		finalPrompt := strings.TrimSpace(buf.String())
		// fmt.Printf("------ [Formatter: %s]\nPrompt:\n%s\n---\n", name, finalPrompt)
		return AgentResult{Output: finalPrompt, Ran: true, AgentName: name}
	} else if agent.Function != "" {
		return AgentResult{
			Output:    fmt.Sprintf("[called external %s with input: %s]", agent.Function, input),
			Ran:       true,
			AgentName: name,
		}
	} else if agent.Prompt != "" {
		tmpl, err := template.New(name).Parse(agent.Prompt)
		if err != nil {
			return AgentResult{Ran: false, Error: fmt.Errorf("error parsing template: %w", err), AgentName: name}
		}
		var buf bytes.Buffer
		err = tmpl.Execute(&buf, &TemplateContext{Input: input, ctx: ctx})
		if err != nil {
			return AgentResult{Ran: false, Error: fmt.Errorf("error executing template: %w", err), AgentName: name}
		}
		finalPrompt := strings.TrimSpace(buf.String())
		if finalPrompt == "" {
			return AgentResult{Ran: false, Output: "", AgentName: name}
		}
		resp, err := callAI(ctx, name, finalPrompt, &TemplateContext{Input: input, ctx: ctx})
		if err != nil {
			return AgentResult{Ran: true, Error: err, AgentName: name}
		}
		return AgentResult{Output: resp, Ran: true, AgentName: name}
	}
	return AgentResult{Ran: false, Error: errors.New("no prompt or function"), AgentName: name}
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
