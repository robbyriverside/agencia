package agencia

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

const simpleSpec = `
agents:
  echo:
    description: echoes input
    template: |
      You said: {{ INPUT }}
`

func TestSpecRunnerRun(t *testing.T) {
	runner, err := NewSpecRunner(SpecRunnerConfig{Spec: simpleSpec})
	if err != nil {
		t.Fatalf("NewSpecRunner error: %v", err)
	}
	got, err := runner.Run(context.Background(), "Hello")
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if got != "You said: Hello" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestSpecRunnerRunWithAgentOverride(t *testing.T) {
	spec := `
agents:
  alpha:
    template: |
      Alpha: {{ INPUT }}
  beta:
    template: |
      Beta: {{ INPUT }}
`
	runner, err := NewSpecRunner(SpecRunnerConfig{
		Spec:       spec,
		StartAgent: "alpha",
	})
	if err != nil {
		t.Fatalf("runner init error: %v", err)
	}
	got, err := runner.Run(context.Background(), "test", WithRunAgent("beta"))
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if got != "Beta: test" {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestSpecRunnerPrintState(t *testing.T) {
	var buf bytes.Buffer
	runner, err := NewSpecRunner(SpecRunnerConfig{
		Spec:              simpleSpec,
		PrintFacts:        true,
		PrintObservations: true,
		Output:            &buf,
	})
	if err != nil {
		t.Fatalf("runner init error: %v", err)
	}
	runner.Session.Facts["echo.result"] = "ok"
	runner.Session.Observations["observer"] = map[string][]string{
		"note": {"logged"},
	}
	if _, err := runner.Run(context.Background(), "Hello"); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "echo.result: ok") {
		t.Fatalf("expected facts in output, got: %s", output)
	}
	if !strings.Contains(output, "observer:") || !strings.Contains(output, "note: logged") {
		t.Fatalf("expected observations in output, got: %s", output)
	}
}

func TestSpecRunnerChatInteractive(t *testing.T) {
	var buf bytes.Buffer
	runner, err := NewSpecRunner(SpecRunnerConfig{
		Spec:   simpleSpec,
		Output: &buf,
	})
	if err != nil {
		t.Fatalf("runner init error: %v", err)
	}
	input := "Hello\nexit\n"
	if err := runner.chatInteractive(context.Background(), strings.NewReader(input), &buf); err != nil {
		t.Fatalf("chatInteractive error: %v", err)
	}
	if !strings.Contains(buf.String(), "You said: Hello") {
		t.Fatalf("chat output missing response: %s", buf.String())
	}
}
