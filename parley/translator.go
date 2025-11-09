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
	for i := 0; i < len(matches); i++ {
		match := matches[i]
		start, end := match[0], match[1]
		contentStart, contentEnd := match[2], match[3]
		builder.WriteString(source[last:start])
		content := strings.TrimSpace(source[contentStart:contentEnd])
		upper := strings.ToUpper(content)

		if strings.HasPrefix(upper, "SEND ") || strings.HasPrefix(upper, "LET ") {
			blockEnd := -1
			for j := i + 1; j < len(matches); j++ {
				nextContent := strings.TrimSpace(source[matches[j][2]:matches[j][3]])
				if strings.EqualFold(nextContent, "END") {
					blockEnd = j
					break
				}
			}
			if blockEnd != -1 {
				blockSegment := source[end:matches[blockEnd][0]]
				if strings.TrimSpace(blockSegment) != "" {
					combined := content + "\n" + strings.Trim(blockSegment, "\n") + "\nEND"
					var translated string
					var err error
					if strings.HasPrefix(upper, "SEND ") {
						translated, err = translateSend(combined)
					} else {
						translated, err = translateLet(combined)
					}
					if err != nil {
						return "", err
					}
					builder.WriteString(translated)
					last = matches[blockEnd][1]
					i = blockEnd
					continue
				}
			}
		}

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
	case strings.HasPrefix(upper, "ELSE IF "):
		return translateElseIf(content)
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
	case strings.HasPrefix(upper, "SEND "):
		return translateSend(content)
	case strings.HasPrefix(upper, "FACT"):
		return translateFact(content)
	case strings.HasPrefix(upper, "IF"):
		return translateIf(content)
	case strings.HasPrefix(upper, "LIST"):
		return translateList(content)
	case strings.HasPrefix(upper, "LET "):
		return translateLet(content)
	case strings.HasPrefix(upper, "USE "):
		return translateLookup(strings.TrimSpace(content[3:]))
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

func translateSend(content string) (string, error) {
	body := strings.TrimSpace(content[len("SEND "):])
	agentToken, remainder := splitFirstToken(body)
	if agentToken == "" {
		return "", fmt.Errorf("invalid SEND directive: %s", content)
	}
	agent := agentToken
	remainder = strings.TrimSpace(remainder)
	upper := strings.ToUpper(remainder)
	if strings.HasPrefix(upper, "IN ") {
		remainder = strings.TrimSpace(remainder[3:])
		libraryToken, rest := splitFirstToken(remainder)
		if libraryToken == "" {
			return "", fmt.Errorf("invalid SEND directive: %s", content)
		}
		agent = fmt.Sprintf("%s.%s", libraryToken, agent)
		remainder = strings.TrimSpace(rest)
	}
	modeToken, payload := splitFirstToken(remainder)
	if modeToken == "" {
		return "", fmt.Errorf("invalid SEND directive: %s", content)
	}
	switch strings.ToUpper(modeToken) {
	case "MESSAGE":
		return translateSendMessage(agent, payload)
	case "LIST":
		return translateSendList(agent, payload)
	default:
		return "", fmt.Errorf("unsupported SEND directive: %s", content)
	}
}

func translateSendMessage(agent string, payload string) (string, error) {
	if payload == "" {
		return "", fmt.Errorf("SEND %s MESSAGE requires content", agent)
	}
	if isBlockContent(payload) {
		blockText, err := extractBlockText(payload)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("{{ .CallWith %q (.Eval %s) }}", agent, quoteString(blockText)), nil
	}
	valueExpr, err := translateValue(payload)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("{{ .CallWith %q (%s) }}", agent, valueExpr), nil
}

func translateSendList(agent string, payload string) (string, error) {
	if payload == "" {
		return "", fmt.Errorf("SEND %s LIST requires content", agent)
	}
	if isBlockContent(payload) {
		blockText, err := extractBlockText(payload)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("{{ .CallOnList %q (.Eval %s) }}", agent, quoteString(blockText)), nil
	}
	valueExpr, err := translateValue(payload)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("{{ .CallOnList %q (%s) }}", agent, valueExpr), nil
}

func translateFact(content string) (string, error) {
	valueExpr, err := translateValue(strings.TrimSpace(content[len("FACT"):]))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("{{ %s }}", valueExpr), nil
}

func translateIf(content string) (string, error) {
	trimmed := strings.TrimSpace(content[len("IF"):])
	inlineCandidate := trimTrailingEndKeyword(trimmed)
	inlineUpper := strings.ToUpper(inlineCandidate)
	if inlineCandidate != "" && strings.Contains(inlineUpper, " THEN ") && strings.Contains(inlineUpper, " ELSE ") {
		return translateInlineIf(inlineCandidate)
	}

	upper := strings.ToUpper(trimmed)
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

func translateElseIf(content string) (string, error) {
	body := strings.TrimSpace(content[len("ELSE IF"):])
	bodyUpper := strings.ToUpper(body)
	if strings.HasSuffix(bodyUpper, " THEN") {
		body = strings.TrimSpace(body[:len(body)-len("THEN")])
	}
	if body == "" {
		return "", fmt.Errorf("ELSE IF requires a predicate")
	}
	predicateExpr, err := translatePredicate(body)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("{{ else if %s }}", predicateExpr), nil
}

func translateInlineIf(body string) (string, error) {
	body = trimTrailingEndKeyword(body)
	parts := strings.Split(body, " ELSE ")
	if len(parts) < 2 {
		return "", fmt.Errorf("unsupported IF expression: missing ELSE branch")
	}
	type branch struct {
		predicate string
		value     string
	}
	branches := make([]branch, 0, len(parts)-1)
	for i := 0; i < len(parts)-1; i++ {
		clause := strings.TrimSpace(parts[i])
		if clause == "" {
			return "", fmt.Errorf("missing IF clause before ELSE")
		}
		if i > 0 {
			clauseUpper := strings.ToUpper(clause)
			if !strings.HasPrefix(clauseUpper, "IF ") {
				return "", fmt.Errorf("ELSE branch must begin with IF predicate")
			}
			clause = strings.TrimSpace(clause[len("IF "):])
		}
		predicatePart, valuePart, found := strings.Cut(clause, "THEN")
		if !found {
			return "", fmt.Errorf("missing THEN in IF clause")
		}
		predicateExpr, err := translatePredicate(strings.TrimSpace(predicatePart))
		if err != nil {
			return "", err
		}
		valueText := strings.TrimSpace(valuePart)
		if valueText == "" {
			return "", fmt.Errorf("missing THEN branch value in IF clause")
		}
		valueExpr, err := translateValue(valueText)
		if err != nil {
			return "", err
		}
		branches = append(branches, branch{
			predicate: predicateExpr,
			value:     valueExpr,
		})
	}
	elsePart := strings.TrimSpace(parts[len(parts)-1])
	elsePart = trimTrailingEndKeyword(elsePart)
	if elsePart == "" {
		return "", fmt.Errorf("missing ELSE branch value")
	}
	elseExpr, err := translateValue(elsePart)
	if err != nil {
		return "", err
	}

	var builder strings.Builder
	for i, clause := range branches {
		if i == 0 {
			builder.WriteString(fmt.Sprintf("{{ if %s }}{{ %s }}", clause.predicate, clause.value))
			continue
		}
		builder.WriteString(fmt.Sprintf("{{ else if %s }}{{ %s }}", clause.predicate, clause.value))
	}
	builder.WriteString(fmt.Sprintf("{{ else }}{{ %s }}{{ end }}", elseExpr))
	return builder.String(), nil
}

func translateList(content string) (string, error) {
	body := strings.TrimSpace(content[len("LIST"):])
	if body == "" {
		return "", fmt.Errorf("LIST requires a value")
	}
	upper := strings.ToUpper(body)
	fromIdx := strings.Index(upper, " FROM ")
	asIdx := strings.Index(upper, " AS ")
	valueEnd := len(body)
	if fromIdx != -1 && (asIdx == -1 || fromIdx < asIdx) {
		valueEnd = fromIdx
	} else if asIdx != -1 && (fromIdx == -1 || asIdx < fromIdx) {
		valueEnd = asIdx
	}
	valuePart := strings.TrimSpace(body[:valueEnd])
	if valuePart == "" {
		return "", fmt.Errorf("LIST requires a value")
	}
	readStyle := "bullets"
	writeStyle := "bullets"
	if fromIdx != -1 {
		fromStart := fromIdx + len(" FROM ")
		fromEnd := len(body)
		if asIdx != -1 && asIdx > fromIdx {
			fromEnd = asIdx
		}
		readStyle = normalizeListStyle(strings.TrimSpace(body[fromStart:fromEnd]))
	}
	if asIdx != -1 {
		writeStyle = normalizeListStyle(strings.TrimSpace(body[asIdx+len(" AS "):]))
	}
	valueExpr, err := translateValue(valuePart)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("{{ .ListFormat (%s) %q %q }}", valueExpr, readStyle, writeStyle), nil
}

func translateLet(content string) (string, error) {
	body := strings.TrimSpace(content[len("LET "):])
	label, remainder := splitFirstToken(body)
	if label == "" {
		return "", fmt.Errorf("invalid LET directive: %s", content)
	}
	remainderUpper := strings.ToUpper(remainder)
	if !strings.HasPrefix(remainderUpper, "BE") {
		return "", fmt.Errorf("invalid LET directive: %s", content)
	}
	payload := strings.TrimSpace(remainder[len("BE"):])
	if payload == "" {
		return "", fmt.Errorf("LET %s BE requires a value", label)
	}
	if isBlockContent(payload) {
		blockText, err := extractBlockText(payload)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("{{ .Bind %q (.Eval %s) }}", label, quoteString(blockText)), nil
	}
	valueExpr, err := translateValue(payload)
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
	case upper == "USE":
		return `.Lookup ""`, nil
	case strings.HasPrefix(upper, "USE "):
		return fmt.Sprintf(".Lookup %q", strings.TrimSpace(content[3:])), nil
	case strings.HasPrefix(upper, "SEND "):
		sendDirective, err := translateSend(content)
		if err != nil {
			return "", err
		}
		return strings.TrimSuffix(strings.TrimPrefix(sendDirective, "{{ "), " }}"), nil
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
	if strings.Contains(upper, " IN ") {
		parts := strings.SplitN(body, "IN", 2)
		label := strings.TrimSpace(parts[0])
		agent := strings.TrimSpace(parts[1])
		if label == "" || agent == "" {
			return "", fmt.Errorf("invalid FACT reference %q", body)
		}
		return fmt.Sprintf(".Fact %q", fmt.Sprintf("%s.%s", agent, label)), nil
	}
	if strings.Contains(upper, " FROM ") {
		parts := strings.SplitN(body, "FROM", 2)
		label := strings.TrimSpace(parts[0])
		agent := strings.TrimSpace(parts[1])
		return fmt.Sprintf(".Fact %q", fmt.Sprintf("%s.%s", agent, label)), nil
	}
	return fmt.Sprintf(".Fact %q", body), nil
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

func splitFirstToken(s string) (string, string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ""
	}
	fields := strings.Fields(s)
	token := fields[0]
	rest := strings.TrimSpace(s[len(token):])
	return token, rest
}

func normalizeListStyle(style string) string {
	s := strings.ToLower(strings.TrimSpace(style))
	switch s {
	case "", "bullet", "bullets":
		return "bullets"
	case "line", "lines":
		return "lines"
	case "paragraph", "paragraphs":
		return "paragraphs"
	default:
		return "bullets"
	}
}

func trimTrailingEndKeyword(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	upper := strings.ToUpper(trimmed)
	if upper == "END" {
		return ""
	}
	if strings.HasSuffix(upper, " END") && len(trimmed) > 3 {
		return strings.TrimSpace(trimmed[:len(trimmed)-3])
	}
	return trimmed
}
