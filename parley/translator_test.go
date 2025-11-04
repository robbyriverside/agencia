package parley

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"text/template"
)

type stubContext struct {
	inputs   map[string]string
	facts    map[string]any
	bindings map[string]any
}

func newStubContext() *stubContext {
	return &stubContext{
		inputs: map[string]string{
			"":      "Hello world",
			"topic": "Roadmap",
		},
		facts: map[string]any{
			"profile.owner":             "Zhou",
			"last_contact":              "123-555-2121",
			"ticket.status":             "Resolved",
			"ticket.resolution_summary": "Ticket closed",
			"ticket.escalation_note":    "Escalate to networking team",
			"ticket.priority":           "High",
			"ticket.follow_up_needed":   "TRUE",
			"tasks":                     []any{"Align roadmap", "Confirm staffing"},
		},
		bindings: make(map[string]any),
	}
}

func (c *stubContext) Input(args ...string) any {
	if len(args) == 0 {
		return c.inputs[""]
	}
	return c.inputs[args[0]]
}

func (c *stubContext) Call(agent string) string {
	return "call:" + agent
}

func (c *stubContext) CallWith(agent string, value any) string {
	return "call_with:" + agent + ":" + c.toString(value)
}

func (c *stubContext) CallOnList(agent string, values any) string {
	items := c.toStringSlice(values)
	return "call_on_list:" + agent + ":[" + strings.Join(items, ",") + "]"
}

func (c *stubContext) List(value any, style string) string {
	items := c.toStringSlice(value)
	switch style {
	case "bullets":
		for i, item := range items {
			items[i] = "- " + item
		}
		return strings.Join(items, "\n")
	case "sentences":
		for i, item := range items {
			if !strings.HasSuffix(item, ".") {
				item += "."
			}
			items[i] = item
		}
		return strings.Join(items, " ")
	default:
		return "[" + strings.Join(items, " ") + "]"
	}
}

func (c *stubContext) Bind(label string, value any) string {
	c.bindings[label] = value
	return ""
}

func (c *stubContext) Lookup(label string) any {
	if label == "" {
		return ""
	}
	if val, ok := c.bindings[label]; ok {
		return val
	}
	if val, ok := c.inputs[label]; ok {
		return val
	}
	if val, ok := c.facts[label]; ok {
		return val
	}
	return ""
}

func (c *stubContext) Fact(name string, _ ...any) any {
	if val, ok := c.facts[name]; ok {
		return val
	}
	return ""
}

func (c *stubContext) Equals(value any, expected string) bool {
	return strings.EqualFold(c.toString(value), expected)
}

func (c *stubContext) Has(value any, expected string) bool {
	expected = strings.ToLower(expected)
	for _, item := range c.toStringSlice(value) {
		if strings.Contains(strings.ToLower(item), expected) {
			return true
		}
	}
	return false
}

func (c *stubContext) IsEmpty(value any) bool {
	switch v := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(v) == ""
	case []string:
		return len(v) == 0
	case []any:
		return len(v) == 0
	default:
		return strings.TrimSpace(c.toString(v)) == ""
	}
}

func (c *stubContext) toString(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", v))
	}
}

func (c *stubContext) toStringSlice(value any) []string {
	switch v := value.(type) {
	case nil:
		return nil
	case []string:
		out := make([]string, len(v))
		copy(out, v)
		for i := range out {
			out[i] = strings.TrimSpace(out[i])
		}
		return out
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			out = append(out, c.toString(item))
		}
		return out
	case string:
		lines := strings.Split(v, "\n")
		out := make([]string, 0, len(lines))
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			trimmed = strings.TrimPrefix(trimmed, "- ")
			trimmed = strings.TrimPrefix(trimmed, "-")
			trimmed = strings.TrimPrefix(trimmed, "* ")
			trimmed = strings.TrimPrefix(trimmed, "*")
			trimmed = strings.TrimSpace(trimmed)
			if trimmed == "" {
				continue
			}
			out = append(out, trimmed)
		}
		return out
	default:
		return []string{c.toString(v)}
	}
}

func TestTranslateAndExecute(t *testing.T) {
	ctx := newStubContext()
	cases := []struct {
		name     string
		parley   string
		expected string
	}{
		{
			name:     "InputDefault",
			parley:   "Hello {{ INPUT }}",
			expected: "Hello Hello world",
		},
		{
			name:     "InputNamed",
			parley:   "Topic: {{ INPUT topic }}",
			expected: "Topic: Roadmap",
		},
		{
			name:     "CallSimple",
			parley:   "{{ CALL greet }}",
			expected: "call:greet",
		},
		{
			name:     "CallFrom",
			parley:   "{{ CALL summarize FROM profile }}",
			expected: "call:profile.summarize",
		},
		{
			name:     "CallWithValue",
			parley:   "{{ CALL summarize WITH INPUT topic }}",
			expected: "call_with:summarize:Roadmap",
		},
		{
			name: "CallWithBlock",
			parley: `{{ CALL summarize WITH
Line one
Line two
END }}`,
			expected: "call_with:summarize:Line one\nLine two",
		},
		{
			name:     "CallOnListValue",
			parley:   "{{ CALL notify ON LIST tasks }}",
			expected: "call_on_list:notify:[Align roadmap,Confirm staffing]",
		},
		{
			name: "CallOnListBlock",
			parley: `{{ CALL notify ON LIST
- Align roadmap
- Confirm staffing
END }}`,
			expected: "call_on_list:notify:[Align roadmap,Confirm staffing]",
		},
		{
			name:     "FactExplicit",
			parley:   "Owner: {{ FACT owner FROM profile }}",
			expected: "Owner: Zhou",
		},
		{
			name:     "FactBare",
			parley:   "Contact: {{ FACT last_contact }}",
			expected: "Contact: 123-555-2121",
		},
		{
			name:     "IfInline",
			parley:   "{{ IF status FROM ticket IS Resolved THEN resolution_summary FROM ticket ELSE escalation_note FROM ticket END }}",
			expected: "Ticket closed",
		},
		{
			name:     "IfBlock",
			parley:   `{{ IF priority FROM ticket IS High THEN }}Escalate now.{{ ELSE }}Monitor.{{ END }}`,
			expected: "Escalate now.",
		},
		{
			name:     "IfThenOnly",
			parley:   `{{ IF follow_up_needed FROM ticket IS TRUE THEN }}Schedule follow-up.{{ END }}`,
			expected: "Schedule follow-up.",
		},
		{
			name:     "ListDefault",
			parley:   "{{ LIST tasks }}",
			expected: "[Align roadmap Confirm staffing]",
		},
		{
			name:     "ListBullets",
			parley:   "{{ LIST tasks OF BULLETS }}",
			expected: "- Align roadmap\n- Confirm staffing",
		},
		{
			name:     "ListSentences",
			parley:   "{{ LIST tasks OF SENTENCES }}",
			expected: "Align roadmap. Confirm staffing.",
		},
		{
			name:     "UsingThe",
			parley:   `{{ USING summary FROM CALL summarize }}Summary: {{ THE summary }}`,
			expected: "Summary: call:summarize",
		},
		{
			name: "UsingBlock",
			parley: `{{ USING note FROM
Thanks team!
END }}{{ USE note }}`,
			expected: "Thanks team!",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			translated, err := Translate(tc.parley)
			if err != nil {
				t.Fatalf("Translate error: %v", err)
			}
			tmpl, err := template.New("test").Parse(translated)
			if err != nil {
				t.Fatalf("Parse error: %v", err)
			}
			var buf bytes.Buffer
			if err := tmpl.Execute(&buf, ctx); err != nil {
				t.Fatalf("Execute error: %v", err)
			}
			result := strings.TrimSpace(buf.String())
			if result != tc.expected {
				t.Fatalf("expected %q, got %q (translated: %s)", tc.expected, result, translated)
			}
		})
	}
}
