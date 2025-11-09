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
			"tasks":                     []any{"Align roadmap", "Confirm staffing", "Publish summary"},
			"lines":                     "Align roadmap\nConfirm staffing",
			"paragraphs":                "Align roadmap status\n\nConfirm staffing actions",
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
	items := c.listWithStyle(values, "bullets")
	if len(items) == 0 {
		return "call_on_list:" + agent + ":"
	}
	return "call_on_list:" + agent + ":\n- " + strings.Join(items, "\n- ")
}

func (c *stubContext) List(value any, style string) string {
	return c.ListFormat(value, style, style)
}

func (c *stubContext) ListFormat(value any, readStyle, writeStyle string) string {
	items := c.listWithStyle(value, readStyle)
	switch writeStyle {
	case "lines":
		return strings.Join(items, "\n")
	case "paragraphs":
		return strings.Join(items, "\n\n")
	default:
		bullets := make([]string, len(items))
		for i, item := range items {
			bullets[i] = "- " + item
		}
		return strings.Join(bullets, "\n")
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
	for _, item := range c.listWithStyle(value, "bullets") {
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

func (c *stubContext) listWithStyle(value any, style string) []string {
	style = strings.ToLower(style)
	switch v := value.(type) {
	case nil:
		return nil
	case []string:
		out := make([]string, 0, len(v))
		for _, item := range v {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			out = append(out, item)
		}
		return out
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			text := strings.TrimSpace(c.toString(item))
			if text == "" {
				continue
			}
			out = append(out, text)
		}
		return out
	case string:
		return parseStubListString(v, style)
	default:
		text := strings.TrimSpace(c.toString(v))
		if text == "" {
			return nil
		}
		return []string{text}
	}
}

func parseStubListString(input, style string) []string {
	input = strings.ReplaceAll(input, "\r\n", "\n")
	switch style {
	case "lines":
		lines := strings.Split(input, "\n")
		out := make([]string, 0, len(lines))
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			out = append(out, trimmed)
		}
		return out
	case "paragraphs":
		return splitStubParagraphs(input)
	default:
		lines := strings.Split(input, "\n")
		out := make([]string, 0, len(lines))
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			if !strings.HasPrefix(trimmed, "-") {
				continue
			}
			trimmed = strings.TrimSpace(trimmed[1:])
			if trimmed == "" {
				continue
			}
			out = append(out, trimmed)
		}
		return out
	}
}

func splitStubParagraphs(input string) []string {
	lines := strings.Split(input, "\n")
	var paragraphs []string
	var current []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if len(current) > 0 {
				paragraphs = append(paragraphs, strings.Join(current, " "))
				current = nil
			}
			continue
		}
		current = append(current, trimmed)
	}
	if len(current) > 0 {
		paragraphs = append(paragraphs, strings.Join(current, " "))
	}
	return paragraphs
}

func (c *stubContext) Eval(block string) string {
	translated, err := Translate(block)
	if err != nil {
		return fmt.Sprintf("[parley error: %v]", err)
	}
	tmpl, err := template.New("inline").Parse(translated)
	if err != nil {
		return fmt.Sprintf("[parley parse error: %v]", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, c); err != nil {
		return fmt.Sprintf("[parley exec error: %v]", err)
	}
	return strings.TrimSpace(buf.String())
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
			name:     "SendMessage",
			parley:   "{{ SEND greet MESSAGE INPUT }}",
			expected: "call_with:greet:Hello world",
		},
		{
			name:     "SendMessageBlock",
			parley:   `{{ SEND greet MESSAGE }}Thank you!{{ END }}`,
			expected: "call_with:greet:Thank you!",
		},
		{
			name:     "SendMessageLibrary",
			parley:   "{{ SEND summarize IN profile MESSAGE INPUT }}",
			expected: "call_with:profile.summarize:Hello world",
		},
		{
			name:     "SendListValue",
			parley:   "{{ SEND notify LIST tasks }}",
			expected: "call_on_list:notify:\n- Align roadmap\n- Confirm staffing\n- Publish summary",
		},
		{
			name: "SendListBlock",
			parley: `{{ SEND notify LIST }}
- Align roadmap
- Confirm staffing
{{ END }}`,
			expected: "call_on_list:notify:\n- Align roadmap\n- Confirm staffing",
		},
		{
			name:     "FactExplicit",
			parley:   "Owner: {{ FACT owner IN profile }}",
			expected: "Owner: Zhou",
		},
		{
			name:     "FactBare",
			parley:   "Contact: {{ FACT last_contact }}",
			expected: "Contact: 123-555-2121",
		},
		{
			name:     "IfInline",
			parley:   "{{ IF status IN ticket IS Resolved THEN FACT resolution_summary IN ticket ELSE FACT escalation_note IN ticket }}",
			expected: "Ticket closed",
		},
		{
			name:     "IfInlineElseIf",
			parley:   "{{ IF status IN ticket IS Closed THEN FACT escalation_note IN ticket ELSE IF priority IN ticket IS High THEN FACT resolution_summary IN ticket ELSE FACT escalation_note IN ticket }}",
			expected: "Ticket closed",
		},
		{
			name:     "IfInlineElseFallback",
			parley:   "{{ IF status IN ticket IS Closed THEN FACT resolution_summary IN ticket ELSE IF priority IN ticket IS Low THEN FACT owner IN profile ELSE FACT escalation_note IN ticket }}",
			expected: "Escalate to networking team",
		},
		{
			name:     "IfBlock",
			parley:   `{{ IF priority IN ticket IS High THEN }}Escalate now.{{ ELSE }}Monitor.{{ END }}`,
			expected: "Escalate now.",
		},
		{
			name:     "IfBlockElseIf",
			parley:   `{{ IF status IN ticket IS Closed THEN }}Closed.{{ ELSE IF priority IN ticket IS High THEN }}High priority.{{ ELSE }}Fallback.{{ END }}`,
			expected: "High priority.",
		},
		{
			name:     "IfThenOnly",
			parley:   `{{ IF follow_up_needed IN ticket IS TRUE THEN }}Schedule follow-up.{{ END }}`,
			expected: "Schedule follow-up.",
		},
		{
			name:     "ListDefault",
			parley:   "{{ LIST tasks }}",
			expected: "- Align roadmap\n- Confirm staffing\n- Publish summary",
		},
		{
			name:     "ListFromLines",
			parley:   "{{ LIST FACT lines FROM LINES AS LINES }}",
			expected: "Align roadmap\nConfirm staffing",
		},
		{
			name:     "ListAsParagraphs",
			parley:   "{{ LIST FACT paragraphs FROM PARAGRAPHS AS PARAGRAPHS }}",
			expected: "Align roadmap status\n\nConfirm staffing actions",
		},
		{
			name:     "LetMessage",
			parley:   "{{ LET summary BE SEND summarize IN profile MESSAGE INPUT }}Summary: {{ USE summary }}",
			expected: "Summary: call_with:profile.summarize:Hello world",
		},
		{
			name:     "LetBlock",
			parley:   "{{ LET note BE }}Thanks team!{{ END }}Note: {{ USE note }}",
			expected: "Note: Thanks team!",
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
