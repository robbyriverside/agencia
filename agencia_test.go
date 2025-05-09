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
	"github.com/stretchr/testify/require"
)

// TestTemplateAgent_Echos verifies that a simple template agent renders the
// expected string given an input value.
func TestTemplateAgent_Echos(t *testing.T) {
	// requireAPI(t)

	const spec = `
agents:
  greet:
    description: Simple greeting template
    template: "Hello, {{ .Input }}!"
`
	ctx := context.Background()

	registry, err := NewRegistry(spec)
	assert.NoError(t, err, "NewRegistry() error = %v", err)

	got, card := registry.Run(ctx, "greet", "Bob")
	want := "Hello, Bob!"
	if got != want {
		t.Fatalf("greet output = %q, want %q", got, want)
	}

	t.Logf("Card: %v", card)
}

// TestInvalidAgent_NoType ensures NewRegistry returns an error when an agent
// definition lacks template, prompt, function, or alias.
func TestInvalidAgent_NoType(t *testing.T) {
	requireAPI(t)

	const spec = `
agents:
  broken:
    description: "I forgot to define what kind of agent I am"
`
	_, err := NewRegistry(spec)
	require.Error(t, err, "expected NewRegistry to fail on agent with no type")
}

// TestCircularAlias detects a self‑referencing alias loop and returns an error.
func TestCircularAlias(t *testing.T) {
	requireAPI(t)

	const spec = `
agents:
  whoopsy:
     description: "I am whoopsy"
     template: "I am whoopsy {{ .Get \"loopy\" }}"
  soupy:
    description: "I am soupy"
    template: "I am soupy {{ .Get \"whoopsy\" }}"
  loopy:
    alias: "soupy"
`
	result := LintSpecFile([]byte(spec))
	t.Logf("### Response: %s", result.Summary)
	if len(result.Errors) > 0 {
		for _, err := range result.Errors {
			t.Log(err)
		}
	}
	if len(result.Warnings) > 0 {
		for _, warn := range result.Warnings {
			t.Log(warn)
		}
	}
	if result.Valid {
		t.Error("Expected invalid spec due to invalid fact scope")
	}
	if len(result.Errors) == 0 {
		t.Error("Expected error for invalid fact scope")
	}
	_, err := NewRegistry(spec)
	require.Error(t, err, "expected NewRegistry to fail on circular alias")
}

// TestTemplate_GetMissing ensures that calling .Get on a non‑existent agent
// surfaces an error at run time.
func TestTemplate_GetMissing(t *testing.T) {
	requireAPI(t)

	const spec = `
agents:
  caller:
    description: Attempts to invoke unknown agent
    template: "{{ .Get \"does.not.exist\" }}"
`
	reg, err := NewRegistry(spec)
	require.NoError(t, err)

	out, card := reg.Run(context.Background(), "caller", "")
	require.NotNil(t, card)
	t.Logf("Output: %s", out)
	assert.Contains(t, out, "could not find agent: does.not.exist", "trace card should note missing agent")
}

// TestSprigFunc_DefaultIfEmpty validates sprig's default helper.
func TestSprigFunc_DefaultIfEmpty(t *testing.T) {
	requireAPI(t)

	const spec = `
agents:
  anon:
    description: Returns a name or defaults to 'anonymous'
    template: "{{ default \"anonymous\" .Input }}"
`
	reg, err := NewRegistry(spec)
	require.NoError(t, err)

	outEmpty, _ := reg.Run(context.Background(), "anon", "")
	outName, _ := reg.Run(context.Background(), "anon", "Lilith")

	assert.Equal(t, "anonymous", outEmpty)
	assert.Equal(t, "Lilith", outName)
}

// TestPromptAgent_WithInput verifies that a prompt agent can embed an input
// variable in its prompt and the model can echo it back.
func TestPromptAgent_WithInput(t *testing.T) {
	requireAPI(t)

	const spec = `
agents:
  echo.ask:
    description: Ask the model to reply with exactly: "ECHO: <msg>"
    inputs:
      msg:
        description: The message to echo.
    prompt: |
      Please reply with exactly the text: "ECHO: {{ .msg }}"
      No additional commentary.
`
	reg, err := NewRegistry(spec)
	require.NoError(t, err)

	got, _ := reg.Run(context.Background(), "echo.ask", "hello there")
	assert.Equal(t, `ECHO: hello there`, strings.TrimSpace(got))
}

// requireAPI skips a test if OPENAI_API_KEY is not set.
func requireAPI(t *testing.T) {
	_ = godotenv.Load()
	if os.Getenv("OPENAI_API_KEY") == "" {
		t.Skip("OPENAI_API_KEY not configured; skipping test that calls OpenAI")
	}
}

// TestPromptAgent_Deterministic verifies that a prompt‑style agent can call the
// OpenAI backend and return a deterministic answer.
func TestPromptAgent_Deterministic(t *testing.T) {
	requireAPI(t)

	const spec = `
agents:
  oksay:
    description: Ask the model to reply with exactly the single word "OK".
    prompt: |
      Reply with exactly the single word OK — no punctuation or quotation marks.`
	reg, err := NewRegistry(spec)
	assert.NoError(t, err)

	got, _ := reg.Run(context.Background(), "oksay", "ready")
	assert.Equal(t, "OK", strings.TrimSpace(got))
}

// TestAliasAgent_Redirect ensures that an alias points to its target agent.
func TestAliasAgent_Redirect(t *testing.T) {
	requireAPI(t)
	const spec = `
agents:
  greet:
    description: Says hello
    template: "Hello, {{ .Input }}!"
  hello:
    alias: "greet"
`
	reg, err := NewRegistry(spec)
	assert.NoError(t, err)

	got, _ := reg.Run(context.Background(), "hello", "Carl")
	assert.Equal(t, "Hello, Carl!", got)
}

// TestGetTemplate_Call tests the .Get template helper for nested agent calls.
func TestGetTemplate_Call(t *testing.T) {
	// requireAPI(t)
	const spec = `
agents:
  greet:
    description: Greets a user
    template: "Hello, {{ .Input }}!"
  intro:
    description: Introduces and greets
    template: "{{ .Get \"greet\" .Input }} Welcome to Agencia."
`
	reg, err := NewRegistry(spec)
	assert.NoError(t, err)

	got, _ := reg.Run(context.Background(), "intro", "Dana")
	assert.Equal(t, "Hello, Dana! Welcome to Agencia.", got)
}

// TestSprigFunc_Upper ensures that sprig functions are available inside agent templates.
func TestSprigFunc_Upper(t *testing.T) {
	const spec = `
agents:
  shout:
    description: Upper‑cases the input
    template: "{{ .Input | upper }}"
`
	reg, err := NewRegistry(spec)
	assert.NoError(t, err)

	got, _ := reg.Run(context.Background(), "shout", "whisper")
	assert.Equal(t, "WHISPER", got)
}

// TestBlankTemplate_Skips ensures that an empty template returns an empty string.
func TestBlankTemplate_Skips(t *testing.T) {
	requireAPI(t)

	const spec = `
agents:
  skip:
    description: A no‑op agent
    template: ""
`
	reg, err := NewRegistry(spec)
	require.Error(t, err)
	if reg != nil {
		out, card := reg.Run(context.Background(), "skip", "")
		t.Logf("Card: %v", card)
		assert.Equal(t, "", strings.TrimSpace(out))
	}
}

// TestSprigFunc_Truncate validates that sprig's truncate helper is wired in.
func TestSprigFunc_Truncate(t *testing.T) {
	requireAPI(t)

	const spec = `
agents:
  abbrev:
    description: Truncates long input
    template: "{{ truncate .Input 6 }}"
`
	reg, err := NewRegistry(spec)
	assert.NoError(t, err)

	long := "supercalifragilistic"
	out, _ := reg.Run(context.Background(), "abbrev", long)
	assert.Equal(t, "superc", out)
}

// TestTemplateAgent_EmptyInput checks that a valid template agent returns an
// empty string when supplied with an empty input.
func TestTemplateAgent_EmptyInput(t *testing.T) {
	requireAPI(t)

	const spec = `
agents:
  echo:
    description: Echoes whatever text is passed in verbatim.
    template: "{{ .Input }}"
`
	reg, err := NewRegistry(spec)
	assert.NoError(t, err, "NewRegistry() should succeed for a simple template agent")

	out, _ := reg.Run(context.Background(), "echo", "")
	assert.Equal(t, "", strings.TrimSpace(out))
}

// TestInputExtraction_Greets uses the inputs block to have AI pull a name out
// of free‑form text and verifies the rendered greeting.
func TestInputExtraction_Greets(t *testing.T) {
	requireAPI(t)

	const spec = `
agents:
  greeter:
    description: Greets a person by name after extracting it from free text.
    inputs:
      name:
        description: The full name of the person to greet.
    template: Hi, {{ .Input "name" }}!
`
	reg, err := NewRegistry(spec)
	assert.NoError(t, err)

	// Send a free‑form sentence; handleAgentInputs should ask AI to pull "Veronica".
	userMsg := "Nice to meet you, my name is Dr. Veronica Zuul and I love Go."
	out, _ := reg.Run(context.Background(), "greeter", userMsg)
	t.Logf("Output: %q", out)
	assert.Contains(t, out, "Dr. Veronica Zuul", "expected extracted name in greeting")
}

// TestAliasChain ensures that multiple alias hops still reach the base agent.
func TestAliasChain(t *testing.T) {
	requireAPI(t)

	const spec = `
agents:
  greet:
    description: Greets someone
    template: "Hello, {{ .Input }}!"
  hello:
    alias: "greet"
  hi:
    alias: "hello"
`
	reg, err := NewRegistry(spec)
	assert.NoError(t, err)

	out, _ := reg.Run(context.Background(), "hi", "Sam")
	assert.Equal(t, "Hello, Sam!", out)
}

// TestSprigFunc_Title verifies that sprig's title helper is available.
func TestSprigFunc_Title(t *testing.T) {
	requireAPI(t)

	const spec = `
agents:
  proper:
    description: Title-cases the input
    template: "{{ .Input | title }}"
`
	reg, err := NewRegistry(spec)
	assert.NoError(t, err)

	out, _ := reg.Run(context.Background(), "proper", "once upon a time")
	assert.Equal(t, "Once Upon A Time", out)
}

// TestRecursiveTemplate_GetLoop creates two agents that invoke each other via
// .Get. The recursion counter inside CallAgent should stop the infinite loop
// and surface an error on the trace card.
func TestRecursiveTemplate_GetLoop(t *testing.T) {
	const spec = `
agents:
  echo1:
    description: Calls echo2 recursively
    template: "{{ .Get \"echo2\" }}"
  echo2:
    description: Calls echo1 recursively
    template: "{{ .Get \"echo1\" }}"
`
	reg, err := NewRegistry(spec)
	require.NoError(t, err)

	out, card := reg.Run(context.Background(), "echo1", "recursion test")
	t.Logf("Output: %q", out)
	require.NotNil(t, card, "expected a trace card even on recursion failure")

	// The exact error message depends on implementation; look for the word "recursive"
	assert.Contains(t, strings.ToLower(out), "recursive", "expected recursion error in trace card")
	card.SaveMarkdown("test.md")
}

// TestComplexCalls_DeepTrace builds a 4‑level chain (Alias → Template → Prompt → Function)
// to exercise every agent type without recursion. It saves the markdown trace so the
// new WriteMarkdown format can be inspected manually.
func TestComplexCalls_DeepTrace(t *testing.T) {
	requireAPI(t)

	const spec = `
agents:
  start:
    description: "Top level agent that calls the first layer"
    alias: "layer1"

  layer1:
    description: "First template layer that calls the second layer"
    template: "{{ .Get \"layer2\" }}"

  layer2:
    description: "Prompt layer that pulls in function result, then replies DONE"
    inputs:
      msg:
        description: Message to pass further down
    prompt: |
        The function result is {{ .Input "msg" | .Get "funcret" }}.
        Reply with exactly the word DONE.
`
	reg, err := NewRegistry(spec)
	require.NoError(t, err)

	reg.RegisterAgent(&agents.Agent{
		Name:        "funcret",
		Description: "Function agent that returns FUNC: <input>",
		Inputs: map[string]*agents.Argument{
			"input": {Description: "The input to the function"},
		},
		Function: agents.AgentFn(func(ctx context.Context, input map[string]any, agent *agents.Agent) (string, error) {
			return fmt.Sprintf("FUNC: %v", input["input"]), nil
		}),
	})

	out, card := reg.Run(context.Background(), "start", "hello")
	assert.Equal(t, "DONE", strings.TrimSpace(out))
	require.NotNil(t, card)
	card.SaveMarkdown("complex_trace.md")
	// var buf strings.Builder
	// card.WriteMarkdown(&buf)
	// t.Logf("Trace card:\n%s", buf.String())
}
func TestInputs(t *testing.T) {
	requireAPI(t)

	const spec = `
agents:
  foo:
     inputs:
          value: 
              description: a string that looks like a value.
     template: "Your number: {{ .Input \"value\" }}"
  greet:
    inputs:
        name:
            description: name of the current user.
    template: |
      {{ .Inputs }}
      {{ .Inputs "foo" }}
      Hello, {{ .Input "name" }}!
      {{ .Get "foo" }}
`
	reg, err := NewRegistry(spec)
	require.NoError(t, err)

	out, card := reg.Run(context.Background(), "greet", "My name is Zaphod and my number is 42")
	// assert.Equal(t, "DONE", strings.TrimSpace(out))
	require.NotNil(t, card)
	assert.Contains(t, out, "Zaphod", "expected extracted name in greeting")
	assert.Contains(t, out, "42", "expected foo inputs in greeting")
	card.SaveMarkdown("trace.md")
}
