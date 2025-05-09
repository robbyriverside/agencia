package agencia

import (
	"strings"
	"testing"
)

func TestLintSpecFile_ValidSpec(t *testing.T) {
	yaml := `---
agents:
  greet:
    description: Greet the user
    prompt: |
      Say hello to {{ .Input }}
    inputs:
      name:
        description: The name of the user
  intro:
    description: Introduce service
    template: |
      {{ .Get "greet" }}
`
	result := LintSpecFile([]byte(yaml))
	t.Logf("### Response: %s", result.Summary)
	if len(result.Errors) > 0 {
		for _, err := range result.Errors {
			t.Logf("Error: %s", err)
		}
	}
	if len(result.Warnings) > 0 {
		for _, warn := range result.Warnings {
			t.Logf("Warning: %s", warn)
		}
	}
	if !result.Valid {
		t.Errorf("Expected valid spec, got errors: %v", result.Errors)
	}
}

func TestLintSpecFile_MissingAgentInGet(t *testing.T) {
	yaml := `---
agents:
  intro:
    description: Introduce service
    template: |
      {{ .Get "missing" }}
`
	result := LintSpecFile([]byte(yaml))
	t.Logf("### Response: %s", result.Summary)
	if len(result.Errors) > 0 {
		for _, err := range result.Errors {
			t.Logf("Error: %s", err)
		}
	}
	if len(result.Warnings) > 0 {
		for _, warn := range result.Warnings {
			t.Logf("Warning: %s", warn)
		}
	}
	if result.Valid {
		t.Error("Expected spec to be invalid due to missing agent reference")
	}
	if len(result.Errors) == 0 {
		t.Error("Expected error for missing agent in .Get")
	}
}

func TestLintSpecFile_TemplateWithListeners(t *testing.T) {
	yaml := `---
agents:
  badagent:
    description: This is bad
    template: |
      Hello
    listeners:
      - greet
  greet:
    description: Greets
    prompt: |
      Hi
`
	result := LintSpecFile([]byte(yaml))
	t.Logf("### Response: %s", result.Summary)
	if len(result.Errors) > 0 {
		for _, err := range result.Errors {
			t.Logf("Error: %s", err)
		}
	}
	if len(result.Warnings) > 0 {
		for _, warn := range result.Warnings {
			t.Logf("Warning: %s", warn)
		}
	}
	if !result.Valid {
		t.Logf("Received expected error: %v", result.Errors)
	}
	if len(result.Warnings) == 0 {
		t.Error("Expected warning about listeners on a template agent")
	}
}

func TestLintSpecFile_UnusedAgent(t *testing.T) {
	yaml := `---
agents:
  one:
    description: Used
    template: |
      {{ .Get "two" }}
  two:
    description: Used
    template: |
      Hello
  unused:
    description: Not used
    template: |
      Ignored
`
	result := LintSpecFile([]byte(yaml))
	t.Logf("### Response: %s", result.Summary)
	if len(result.Errors) > 0 {
		for _, err := range result.Errors {
			t.Logf("Error: %s", err)
		}
	}
	if len(result.Warnings) > 0 {
		for _, warn := range result.Warnings {
			t.Logf("Warning: %s", warn)
		}
	}
	if result.Valid && len(result.Warnings) == 0 {
		t.Error("Expected warning for unused agent")
	}
}

func TestLintSpecFile_RecursiveAgent(t *testing.T) {
	yaml := `---
agents:
  loop:
    description: Looping
    prompt: |
      {{ .Get "loop" }}
    inputs:
      value:
        description: recursion trigger
`
	result := LintSpecFile([]byte(yaml))
	t.Logf("### Response: %s", result.Summary)
	for _, err := range result.Errors {
		t.Logf("Error: %s", err)
	}
	for _, warn := range result.Warnings {
		t.Logf("Warning: %s", warn)
	}
	if result.Valid {
		t.Error("Expected invalid spec due to recursion")
	}
	found := false
	for _, err := range result.Errors {
		if strings.Contains(err, "references itself") {
			found = true
		}
	}
	if !found {
		t.Error("Expected recursion error for agent")
	}
}

func TestLintSpecFile_MissingAction(t *testing.T) {
	yaml := `---
agents:
  empty:
    description: Has no action
`
	result := LintSpecFile([]byte(yaml))
	t.Logf("### Response: %s", result.Summary)
	if len(result.Errors) > 0 {
		for _, err := range result.Errors {
			t.Logf("Error: %s", err)
		}
	}
	if len(result.Warnings) > 0 {
		for _, warn := range result.Warnings {
			t.Logf("Warning: %s", warn)
		}
	}
	if result.Valid {
		t.Error("Expected invalid spec due to missing prompt/template/alias/function")
	}
}

func TestLintSpecFile_MultipleActions(t *testing.T) {
	yaml := `---
agents:
  bad:
    description: Has too many
    prompt: Say hi
    template: Hello
`
	result := LintSpecFile([]byte(yaml))
	t.Logf("### Response: %s", result.Summary)
	if len(result.Errors) > 0 {
		for _, err := range result.Errors {
			t.Logf("Error: %s", err)
		}
	}
	if len(result.Warnings) > 0 {
		for _, warn := range result.Warnings {
			t.Logf("Warning: %s", warn)
		}
	}
	if result.Valid {
		t.Error("Expected invalid spec due to multiple action types")
	}
}

func TestLintSpecFile_LibraryAgentReference(t *testing.T) {
	yaml := `---
agents:
  greet:
    description: Greet the user
    prompt: |
      Say hello to {{ .Input }}
  intro:
    description: Introduce service
    template: |
      {{ .Get "lib.agent" }}
libraries:
  lib:
    agents:
      agent:
        description: Library agent
        prompt: Hi from library
`
	result := LintSpecFile([]byte(yaml))
	t.Logf("### Response: %s", result.Summary)
	if len(result.Errors) > 0 {
		for _, err := range result.Errors {
			t.Logf("Error: %s", err)
		}
	}
	if len(result.Warnings) > 0 {
		for _, warn := range result.Warnings {
			t.Logf("Warning: %s", warn)
		}
	}
	if !result.Valid {
		t.Errorf("Expected valid spec with library agent reference, got errors: %v", result.Errors)
	}
}

func TestLintSpecFile_ListenerToLibraryAgent(t *testing.T) {
	yaml := `---
agents:
  main:
    description: Main agent
    prompt: Hello
    listeners:
      - lib.agent
libraries:
  lib:
    agents:
      agent:
        description: Library agent
        prompt: Hi from library
`
	result := LintSpecFile([]byte(yaml))
	t.Logf("### Response: %s", result.Summary)
	if len(result.Errors) > 0 {
		for _, err := range result.Errors {
			t.Logf("Error: %s", err)
		}
	}
	if len(result.Warnings) > 0 {
		for _, warn := range result.Warnings {
			t.Logf("Warning: %s", warn)
		}
	}
	if !result.Valid {
		t.Errorf("Expected valid spec with listener to library agent, got errors: %v", result.Errors)
	}
}

func TestLintSpecFile_InvalidFactScope(t *testing.T) {
	yaml := `---
agents:
  agent1:
    description: Agent with invalid fact scope
    prompt: Hello
    facts:
      - name: testFact
        scope: invalidScope
`
	result := LintSpecFile([]byte(yaml))
	t.Logf("### Response: %s", result.Summary)
	if len(result.Errors) > 0 {
		for _, err := range result.Errors {
			t.Logf("Error: %s", err)
		}
	}
	if len(result.Warnings) > 0 {
		for _, warn := range result.Warnings {
			t.Logf("Warning: %s", warn)
		}
	}
	if result.Valid {
		t.Error("Expected invalid spec due to invalid fact scope")
	}
	if len(result.Errors) == 0 {
		t.Error("Expected error for invalid fact scope")
	}
}

func TestLintSpecFile_DuplicateAgents(t *testing.T) {
	yaml := `---
agents:
  greet:
    description: First greeting
    prompt: |
      Hello
  greet:
    description: Duplicate greeting
    template: |
      Hi again
`
	result := LintSpecFile([]byte(yaml))
	t.Logf("### Response: %s", result.Summary)
	if len(result.Errors) > 0 {
		for _, err := range result.Errors {
			t.Logf("Error: %s", err)
		}
	}
	if len(result.Warnings) > 0 {
		for _, warn := range result.Warnings {
			t.Logf("Warning: %s", warn)
		}
	}
	if result.Valid {
		t.Error("Expected invalid spec due to duplicate agent names")
	}
	found := false
	for _, err := range result.Errors {
		if strings.Contains(err, "Duplicate agent name") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected error for duplicate agent name")
	}
}

func TestLintSpecFile_SchemaViolation(t *testing.T) {
	yaml := `---
agents:
  example:
    inputs:
      name:
        description: The name
    prompt: 12345 # should be a string, not an integer
`
	result := LintSpecFile([]byte(yaml))
	t.Logf("### Response: %s", result.Summary)
	if len(result.Errors) > 0 {
		for _, err := range result.Errors {
			t.Logf("Error: %s", err)
		}
	}
	if len(result.Warnings) > 0 {
		for _, warn := range result.Warnings {
			t.Logf("Warning: %s", warn)
		}
	}
	if result.Valid {
		t.Error("Expected invalid spec due to schema validation failure")
	}
	found := false
	for _, err := range result.Errors {
		if strings.Contains(err, "The spec is invalid") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected schema validation error")
	}
}
