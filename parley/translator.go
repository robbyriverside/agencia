package parley

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var directivePattern = regexp.MustCompile(`(?s){{(.*?)}}`)

// Translate converts a Parley template into an equivalent Go template string.
func Translate(source string) (string, error) {
	var builder strings.Builder
	last := 0
	matches := directivePattern.FindAllStringSubmatchIndex(source, -1)
	for _, match := range matches {
		start, end := match[0], match[1]
		contentStart, contentEnd := match[2], match[3]
		builder.WriteString(source[last:start])
		content := strings.TrimSpace(source[contentStart:contentEnd])
		translated, err := translateDirective(content)
		if err != nil {
			return "", err
		}
		builder.WriteString(translated)
		last = end
	}
	builder.WriteString(source[last:])
	return builder.String(), nil
}

func translateDirective(content string) (string, error) {
	if content == "" {
		return "{{}}", nil
	}
	upper := strings.ToUpper(content)
	switch {
	case upper == "END":
		return "{{ end }}", nil
	case upper == "ELSE":
		return "{{ else }}", nil
	case strings.HasPrefix(upper, "ELSE "):
		// Allow "ELSE ... END" inline form for completeness.
		body := strings.TrimSpace(content[4:])
		if strings.HasSuffix(strings.ToUpper(body), "END") {
			body = strings.TrimSpace(body[:len(body)-3])
		}
		valueExpr, err := translateValue(body)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("{{ else }}{{ %s }}{{ end }}", valueExpr), nil
	case strings.HasPrefix(upper, "INPUT"):
		return translateInput(content)
	case strings.HasPrefix(upper, "CALL "):
		return translateCall(content)
	case strings.HasPrefix(upper, "FACT"):
		return translateFactDirective(content)
	case strings.HasPrefix(upper, "IF"):
		return translateIf(content)
	case strings.HasPrefix(upper, "LIST"):
		return translateList(content)
	case strings.HasPrefix(upper, "USING"):
		return translateUsing(content)
	case strings.HasPrefix(upper, "THE "):
		return translateLookup(content[4:])
	case strings.HasPrefix(upper, "USE "):
		return translateLookup(content[4:])
	case upper == "THE":
		return translateLookup("")
	case upper == "USE":
		return translateLookup("")
	default:
		// Pass through unknown directives to maintain Go template compatibility.
		return "{{ " + content + " }}", nil
	}
}

func translateInput(content string) (string, error) {
	fields := strings.Fields(content)
	if len(fields) == 1 {
		return "{{ .Input }}", nil
	}
	return fmt.Sprintf("{{ .Input %q }}", fields[1]), nil
}

func translateCall(content string) (string, error) {
	body := strings.TrimSpace(content[len("CALL "):])
	mode := ""
	var suffix string

	if idx := indexCaseInsensitive(body, " WITH"); idx >= 0 {
		mode = "WITH"
		suffix = strings.TrimSpace(body[idx+5:])
		body = strings.TrimSpace(body[:idx])
	} else if idx := strings.Index(strings.ToUpper(body), " ON LIST"); idx >= 0 {
		mode = "ONLIST"
		suffix = strings.TrimSpace(body[idx+8:])
		body = strings.TrimSpace(body[:idx])
	}

	agent, err := parseAgentReference(body)
	if err != nil {
		return "", err
	}

	switch mode {
	case "":
		return fmt.Sprintf("{{ .Call %q }}", agent), nil
	case "WITH":
		if isBlockContent(suffix) {
			blockText, err := extractBlockText(suffix)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("{{ .CallWith %q %s }}", agent, quoteString(blockText)), nil
		}
		valueExpr, err := translateValue(suffix)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("{{ .CallWith %q (%s) }}", agent, valueExpr), nil
	case "ONLIST":
		if isBlockContent(suffix) {
			blockText, err := extractBlockText(suffix)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("{{ .CallOnList %q %s }}", agent, quoteString(blockText)), nil
		}
		valueExpr, err := translateValue(suffix)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("{{ .CallOnList %q (%s) }}", agent, valueExpr), nil
	default:
		return "", fmt.Errorf("unsupported CALL form: %s", content)
	}
}

func translateFactDirective(content string) (string, error) {
	valueExpr, err := translateValue(content)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("{{ %s }}", valueExpr), nil
}

func translateIf(content string) (string, error) {
	trimmed := strings.TrimSpace(content[len("IF"):])
	upper := strings.ToUpper(trimmed)
	// Inline form with THEN ... ELSE ... END
	if strings.Contains(upper, " THEN ") && strings.Contains(upper, " ELSE ") && strings.HasSuffix(upper, " END") {
		predicatePart, remainder, _ := strings.Cut(trimmed, "THEN")
		thenPart, elsePart, _ := strings.Cut(remainder, "ELSE")
		elsePart = strings.TrimSuffix(elsePart, "END")
		predicateExpr, err := translatePredicate(predicatePart)
		if err != nil {
			return "", err
		}
		thenExpr, err := translateValue(strings.TrimSpace(thenPart))
		if err != nil {
			return "", err
		}
		elseExpr, err := translateValue(strings.TrimSpace(elsePart))
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("{{ if %s }}{{ %s }}{{ else }}{{ %s }}{{ end }}", predicateExpr, thenExpr, elseExpr), nil
	}

	// Block form: IF predicate THEN
	if strings.HasSuffix(upper, " THEN") {
		predicatePart := strings.TrimSpace(trimmed[:len(trimmed)-len("THEN")])
		predicateExpr, err := translatePredicate(predicatePart)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("{{ if %s }}", predicateExpr), nil
	}

	return "", fmt.Errorf("unsupported IF directive: %s", content)
}

func translateList(content string) (string, error) {
	body := strings.TrimSpace(content[len("LIST"):])
	if body == "" {
		return "", fmt.Errorf("LIST requires a value")
	}
	style := "default"
	upper := strings.ToUpper(body)
	if strings.Contains(upper, " OF BULLETS") {
		style = "bullets"
		body = strings.TrimSpace(body[:len(body)-len("OF BULLETS")])
	} else if strings.Contains(upper, " OF SENTENCES") {
		style = "sentences"
		body = strings.TrimSpace(body[:len(body)-len("OF SENTENCES")])
	}
	valueExpr, err := translateValue(body)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("{{ .List (%s) %q }}", valueExpr, style), nil
}

func translateUsing(content string) (string, error) {
	body := strings.TrimSpace(content[len("USING"):])
	parts := strings.Fields(body)
	if len(parts) < 3 || strings.ToUpper(parts[1]) != "FROM" {
		return "", fmt.Errorf("invalid USING directive: %s", content)
	}
	label := parts[0]
	tail := strings.TrimSpace(body[len(label)+len(" FROM "):])
	if isBlockContent(tail) {
		blockText, err := extractBlockText(tail)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("{{ .Bind %q %s }}", label, quoteString(blockText)), nil
	}
	valueExpr, err := translateValue(tail)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("{{ .Bind %q (%s) }}", label, valueExpr), nil
}

func translateLookup(label string) (string, error) {
	label = strings.TrimSpace(label)
	if label == "" {
		return "{{ .Lookup \"\" }}", nil
	}
	return fmt.Sprintf("{{ .Lookup %q }}", label), nil
}

func translateValue(content string) (string, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return "", fmt.Errorf("missing value")
	}
	upper := strings.ToUpper(content)
	switch {
	case upper == "INPUT":
		return ".Input", nil
	case strings.HasPrefix(upper, "INPUT "):
		fields := strings.Fields(content)
		return fmt.Sprintf(".Input %q", fields[1]), nil
	case strings.HasPrefix(upper, "FACT "):
		return factExpression(strings.TrimSpace(content[len("FACT "):]))
	case upper == "THE" || upper == "USE":
		return `.Lookup ""`, nil
	case strings.HasPrefix(upper, "THE "):
		return fmt.Sprintf(".Lookup %q", strings.TrimSpace(content[4:])), nil
	case strings.HasPrefix(upper, "USE "):
		return fmt.Sprintf(".Lookup %q", strings.TrimSpace(content[4:])), nil
	case strings.HasPrefix(upper, "CALL "):
		// Nested call treated as string value.
		callDirective, err := translateCall(content)
		if err != nil {
			return "", err
		}
		// Remove surrounding braces to embed inside other expressions.
		return strings.TrimSuffix(strings.TrimPrefix(callDirective, "{{ "), " }}"), nil
	default:
		return factExpression(content)
	}
}

func translatePredicate(content string) (string, error) {
	content = strings.TrimSpace(content)
	upper := strings.ToUpper(content)
	switch {
	case strings.HasSuffix(upper, " IS EMPTY"):
		value := strings.TrimSpace(content[:len(content)-len("IS EMPTY")])
		valueExpr, err := translateValue(value)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("(.IsEmpty (%s))", valueExpr), nil
	case strings.HasSuffix(upper, " IS NOT EMPTY"):
		value := strings.TrimSpace(content[:len(content)-len("IS NOT EMPTY")])
		valueExpr, err := translateValue(value)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("(not (.IsEmpty (%s)))", valueExpr), nil
	case strings.Contains(upper, " IS NOT "):
		parts := strings.SplitN(content, "IS NOT", 2)
		valueExpr, err := translateValue(strings.TrimSpace(parts[0]))
		if err != nil {
			return "", err
		}
		expect := strings.TrimSpace(parts[1])
		return fmt.Sprintf("(not (.Equals (%s) %s))", valueExpr, quoteString(expect)), nil
	case strings.Contains(upper, " IS "):
		parts := strings.SplitN(content, "IS", 2)
		valueExpr, err := translateValue(strings.TrimSpace(parts[0]))
		if err != nil {
			return "", err
		}
		expect := strings.TrimSpace(parts[1])
		return fmt.Sprintf("(.Equals (%s) %s)", valueExpr, quoteString(expect)), nil
	case strings.Contains(upper, " HAS "):
		parts := strings.SplitN(content, "HAS", 2)
		valueExpr, err := translateValue(strings.TrimSpace(parts[0]))
		if err != nil {
			return "", err
		}
		expect := strings.TrimSpace(parts[1])
		return fmt.Sprintf("(.Has (%s) %s)", valueExpr, quoteString(expect)), nil
	default:
		return "", fmt.Errorf("unsupported predicate: %s", content)
	}
}

func factExpression(body string) (string, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return "", fmt.Errorf("invalid FACT reference")
	}
	upper := strings.ToUpper(body)
	if strings.Contains(upper, " FROM ") {
		parts := strings.SplitN(body, "FROM", 2)
		label := strings.TrimSpace(parts[0])
		agent := strings.TrimSpace(parts[1])
		return fmt.Sprintf(".Fact %q", fmt.Sprintf("%s.%s", agent, label)), nil
	}
	return fmt.Sprintf(".Fact %q", body), nil
}

func parseAgentReference(body string) (string, error) {
	upper := strings.ToUpper(body)
	if strings.Contains(upper, " FROM ") {
		parts := strings.SplitN(body, "FROM", 2)
		agent := strings.TrimSpace(parts[0])
		lib := strings.TrimSpace(parts[1])
		if agent == "" || lib == "" {
			return "", fmt.Errorf("invalid CALL reference %q", body)
		}
		return fmt.Sprintf("%s.%s", lib, agent), nil
	}
	return strings.TrimSpace(body), nil
}

func isBlockContent(value string) bool {
	return strings.Contains(value, "\n")
}

func extractBlockText(value string) (string, error) {
	text := strings.TrimSpace(value)
	if strings.HasSuffix(strings.ToUpper(text), "END") {
		text = strings.TrimSpace(text[:len(text)-3])
	}
	return text, nil
}

func quoteString(s string) string {
	return strconv.Quote(s)
}

func indexCaseInsensitive(haystack, needle string) int {
	upperHaystack := strings.ToUpper(haystack)
	upperNeedle := strings.ToUpper(needle)
	return strings.Index(upperHaystack, upperNeedle)
}
