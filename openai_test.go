package agencia

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/joho/godotenv"
	"github.com/robbyriverside/agencia/agents"
	"github.com/stretchr/testify/assert"
)

func TestCallOpenAI_FunctionCalling(t *testing.T) {
	// Try to load .env file first
	_ = godotenv.Load()

	// Check the API key
	if os.Getenv("OPENAI_API_KEY") == "" {
		t.Fatal("OPENAI_API_KEY must be set (either in environment or .env file)")
	}

	reg := &Registry{}
	ctx := context.Background()

	reg.RegisterAgent(&agents.Agent{
		Name:        "greet",
		Description: "Generates a greeting message given a person's name",
		Template:    "Hello, {{ .Input }}!",
	})

	reg.RegisterAgent(&agents.Agent{
		Name:      "tryme",
		Prompt:    "Say hello to {{ .Input }}.",
		Listeners: []string{"greet"},
	})

	input := &TemplateContext{
		Input:    "Alice",
		Registry: *reg,
		ctx:      ctx,
	}

	agent, err := reg.LookupAgent("tryme")
	assert.NoError(t, err, "should find tryme agent")

	output, err := reg.CallAI(ctx, agent, "Say hello to Alice.", input)
	assert.NoError(t, err, "should not error")
	assert.Contains(t, output, "Hello, Alice", "should generate greeting via function call")
}

func TestCallOpenAI_ToolNotFound(t *testing.T) {
	_ = godotenv.Load()

	if os.Getenv("OPENAI_API_KEY") == "" {
		t.Fatal("OPENAI_API_KEY must be set (either in environment or .env file)")
	}

	reg := &Registry{}
	ctx := context.Background()

	// Register only the tryme agent but no greet agent
	reg.RegisterAgent(&agents.Agent{
		Name:      "tryme",
		Prompt:    "Say hello to {{ .Input }}.",
		Listeners: []string{"nonexistent_tool"},
	})

	input := &TemplateContext{
		Input:    "Alice",
		Registry: *reg,
		ctx:      ctx,
	}

	agent, err := reg.LookupAgent("tryme")
	assert.NoError(t, err, "should find tryme agent")

	output, err := reg.CallAI(ctx, agent, "Say hello to Alice.", input)
	assert.Error(t, err, "should error due to missing tool")
	assert.Contains(t, err.Error(), "could not find agent", "should mention missing agent")
	assert.Empty(t, output, "output should be empty on tool not found")
}

func TestCallOpenAI_MultipleToolCalls(t *testing.T) {
	_ = godotenv.Load()

	if os.Getenv("OPENAI_API_KEY") == "" {
		t.Fatal("OPENAI_API_KEY must be set (either in environment or .env file)")
	}

	reg := &Registry{}
	ctx := context.Background()

	// Register two agents
	reg.RegisterAgent(&agents.Agent{
		Name:        "greet",
		Description: "Generates a greeting message given a person's name.",
		Template:    "Hello, {{ .Input }}!",
		InputPrompt: map[string]agents.Argument{
			"personName": {
				Description: "The name of the person to greet.",
			},
		},
	})
	reg.RegisterAgent(&agents.Agent{
		Name:        "farewell",
		Description: "Generates a farewell message saying goodbye to a person's name.",
		Template:    "Goodbye, {{ .Input }}!",
		InputPrompt: map[string]agents.Argument{
			"personName": {
				Description: "The name of the person to say goodbye to.",
			},
		},
	})
	reg.RegisterAgent(&agents.Agent{
		Name:      "tryme",
		Prompt:    "Say hello and goodbye to {{ .Input }}.",
		Listeners: []string{"greet", "farewell"},
	})

	res := reg.CallAgent(ctx, "tryme", "My name is Alice")
	fmt.Printf("***Output: %s\n", res.Output)
	assert.NoError(t, res.Error, "should not error")
	assert.Contains(t, res.Output, "Hello, Alice", "should generate greeting via function call")
	assert.Contains(t, res.Output, "Goodbye, Alice", "should generate farewell via function call")
}

func TestCallOpenAI_EmptyToolOutput(t *testing.T) {
	_ = godotenv.Load()

	if os.Getenv("OPENAI_API_KEY") == "" {
		t.Fatal("OPENAI_API_KEY must be set (either in environment or .env file)")
	}

	reg := &Registry{}
	ctx := context.Background()

	// Register a valid function agent that returns empty output
	reg.RegisterAgent(&agents.Agent{
		Name:        "silent_tool",
		Description: "A tool that returns no output.",
		Function: func(ctx context.Context, input map[string]interface{}) (string, error) {
			return "", nil
		},
		InputPrompt: map[string]agents.Argument{
			"input": {
				Description: "Any input to trigger the silent tool.",
			},
		},
	})

	// Register a tryme agent that triggers it
	reg.RegisterAgent(&agents.Agent{
		Name:      "tryme",
		Prompt:    "Trigger silent_tool with {{ .Input }}.",
		Listeners: []string{"silent_tool"},
	})

	res := reg.CallAgent(ctx, "tryme", "trigger silence")
	fmt.Printf("***Output: %s\n", res.Output)
	assert.NoError(t, res.Error, "should not error even if tool output is empty")
	// Optionally check if output is still acceptable (blank or partial)
}

func TestCallOpenAI_RecursiveToolCalling(t *testing.T) {
	_ = godotenv.Load()

	if os.Getenv("OPENAI_API_KEY") == "" {
		t.Fatal("OPENAI_API_KEY must be set (either in environment or .env file)")
	}

	reg := &Registry{}
	ctx := context.Background()

	// Define two agents that keep triggering each other
	reg.RegisterAgent(&agents.Agent{
		Name:        "echo1",
		Description: "Echo agent 1.",
		Template:    "Calling echo2: {{ .Input }}",
		InputPrompt: map[string]agents.Argument{
			"input": {
				Description: "Input to echo1.",
			},
		},
	})
	reg.RegisterAgent(&agents.Agent{
		Name:        "echo2",
		Description: "Echo agent 2.",
		Template:    "Calling echo1: {{ .Input }}",
		InputPrompt: map[string]agents.Argument{
			"input": {
				Description: "Input to echo2.",
			},
		},
	})
	reg.RegisterAgent(&agents.Agent{
		Name:      "tryme",
		Prompt:    "Start the echo chain with {{ .Input }}.",
		Listeners: []string{"echo1", "echo2"},
	})

	res := reg.CallAgent(ctx, "tryme", "recursive start")
	fmt.Printf("***Output: %s\n", res.Output)
	assert.Error(t, res.Error, "should error due to recursion limit")
	assert.Contains(t, res.Error.Error(), "recursive tool call", "error should mention recursion limit")
}

func TestCallOpenAI_InvalidToolSchema(t *testing.T) {
	_ = godotenv.Load()

	if os.Getenv("OPENAI_API_KEY") == "" {
		t.Fatal("OPENAI_API_KEY must be set (either in environment or .env file)")
	}

	reg := &Registry{}
	ctx := context.Background()

	// Register an agent with invalid InputPrompt schema
	reg.RegisterAgent(&agents.Agent{
		Name:        "badtool",
		Description: "This tool has a broken input schema.",
		Template:    "Hello, {{ .Input }}!",
		InputPrompt: map[string]agents.Argument{
			"input": {
				Description: "This input field is fine.",
				Type:        "strnig", // intentionally invalid type (should be "string")
			},
		},
	})

	reg.RegisterAgent(&agents.Agent{
		Name:      "tryme",
		Prompt:    "Trigger badtool with {{ .Input }}.",
		Listeners: []string{"badtool"},
	})

	res := reg.CallAgent(ctx, "tryme", "test invalid schema")
	fmt.Printf("***Error: %s\n", res.Error)

	assert.Error(t, res.Error, "should error due to invalid schema")
	assert.True(t, strings.Contains(strings.ToLower(res.Error.Error()), "invalid"), "error should mention invalid schema or tool setup")
}

func TestCallOpenAI_ContinuationMissingTool(t *testing.T) {
	_ = godotenv.Load()

	if os.Getenv("OPENAI_API_KEY") == "" {
		t.Fatal("OPENAI_API_KEY must be set (either in environment or .env file)")
	}

	reg := &Registry{}
	ctx := context.Background()

	// Register only a tryme agent but no actual listener agents
	reg.RegisterAgent(&agents.Agent{
		Name:      "tryme",
		Prompt:    "Say hello to {{ .Input }} and call a missing agent.",
		Listeners: []string{"nonexistent_tool"},
	})

	res := reg.CallAgent(ctx, "tryme", "Alice")
	fmt.Printf("***Error: %s\n", res.Error)

	assert.Error(t, res.Error, "should error due to missing agent during continuation")
	assert.Contains(t, res.Error.Error(), "could not find", "error should mention missing agent or tool")
}

func TestCallOpenAI_AgentTemplateFails(t *testing.T) {
	_ = godotenv.Load()

	if os.Getenv("OPENAI_API_KEY") == "" {
		t.Fatal("OPENAI_API_KEY must be set (either in environment or .env file)")
	}

	reg := &Registry{}
	ctx := context.Background()

	// Register an agent with a broken template
	reg.RegisterAgent(&agents.Agent{
		Name:        "broken_template",
		Description: "An agent with a bad template that references a missing field.",
		Template:    "Hello, {{ .nonexistentField }}!",
	})

	// Register a simple agent to trigger it
	reg.RegisterAgent(&agents.Agent{
		Name:      "tryme",
		Prompt:    "Trigger broken_template with {{ .Input }}.",
		Listeners: []string{"broken_template"},
	})

	res := reg.CallAgent(ctx, "tryme", "Alice")
	fmt.Printf("***Error: %s\n", res.Error)

	assert.Error(t, res.Error, "should error due to template execution failure")
	assert.Contains(t, res.Error.Error(), "template", "error should mention template execution")
}

func TestCallOpenAI_MultipleBadListeners(t *testing.T) {
	_ = godotenv.Load()

	if os.Getenv("OPENAI_API_KEY") == "" {
		t.Fatal("OPENAI_API_KEY must be set (either in environment or .env file)")
	}

	reg := &Registry{}
	ctx := context.Background()

	// Bad listener 1 (missing Description)
	reg.RegisterAgent(&agents.Agent{
		Name: "badtool1",
		InputPrompt: map[string]agents.Argument{
			"input": {
				Description: "An input.",
			},
		},
	})

	// Bad listener 2 (missing InputPrompt)
	reg.RegisterAgent(&agents.Agent{
		Name:        "badtool2",
		Description: "Bad tool without input prompt.",
	})

	// Good listener (correct agent)
	reg.RegisterAgent(&agents.Agent{
		Name:        "goodtool",
		Description: "Good tool for testing.",
		InputPrompt: map[string]agents.Argument{
			"input": {
				Description: "Good input.",
			},
		},
		Template: "Good: {{ .Input }}",
	})

	// Main tryme agent
	reg.RegisterAgent(&agents.Agent{
		Name:      "tryme",
		Prompt:    "Try calling multiple tools: {{ .Input }}.",
		Listeners: []string{"badtool1", "badtool2", "goodtool"},
	})

	res := reg.CallAgent(ctx, "tryme", "testing bad listeners")
	fmt.Printf("***Error: %s\n", res.Error)

	assert.Error(t, res.Error, "should error due to multiple invalid listeners")
	assert.Contains(t, res.Error.Error(), "badtool1", "should mention badtool1")
	assert.Contains(t, res.Error.Error(), "badtool2", "should mention badtool2")
	assert.NotContains(t, res.Error.Error(), "goodtool", "should not mention goodtool")
}
