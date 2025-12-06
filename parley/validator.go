package parley

import (
	"fmt"
	"strings"
)

// ValidationContext holds the set of defined agents and their inputs/facts for validation.
type ValidationContext struct {
	Agents       map[string]AgentInfo
	Roles        map[string]bool
	CurrentAgent string // The name of the agent whose template is being validated
}

type AgentInfo struct {
	Inputs    map[string]bool
	Facts     map[string]bool
	IsLibrary bool // True if this is a library agent (contains dot)
}

// Validator checks a Parley template string for syntax errors and invalid references.
type Validator struct {
	ctx        ValidationContext
	errors     []string
	blockStack []string
}

func NewValidator(ctx ValidationContext) *Validator {
	return &Validator{
		ctx:        ctx,
		errors:     []string{},
		blockStack: []string{},
	}
}

func (v *Validator) Validate(source string) []string {
	v.errors = []string{}
	v.blockStack = []string{} // Reset stack for new validation

	// Reuse directive pattern from translator
	// directivePattern = regexp.MustCompile(`(?s){{(.*?)}}`)
	// (Note: we need to access the unexported one or duplicate it. Let's duplicate for now to avoid dependency on internal implementation details if possible, though they are in same package so we can use it if exported?
	// Wait, translator.go is in package parley. directivePattern is unexported var in parley.
	// So we can access it if we are in package parley.)

	matches := directivePattern.FindAllStringSubmatchIndex(source, -1)
	for i := 0; i < len(matches); i++ {
		match := matches[i]
		// start, end := match[0], match[1]
		contentStart, contentEnd := match[2], match[3]
		content := strings.TrimSpace(source[contentStart:contentEnd])

		// Basic Syntax Check & Reference Verification
		if err := v.validateDirective(content); err != nil {
			// Add error with line context if possible?
			// For now just list the error. Use source index to find line number?
			line := strings.Count(source[:match[0]], "\n") + 1
			v.errors = append(v.errors, fmt.Sprintf("Line %d: %v", line, err))
		}
	}

	// Check for unclosed blocks
	if len(v.blockStack) > 0 {
		v.errors = append(v.errors, fmt.Sprintf("Unclosed blocks at end of template: %v", v.blockStack))
	}

	return v.errors
}

func (v *Validator) validateDirective(content string) error {
	if content == "" {
		return nil // Empty directive is valid (comment or placeholder)
	}
	upper := strings.ToUpper(content)

	// Handle Block Starts
	// Handle Block Starts
	if strings.HasPrefix(upper, "SEND ") || strings.HasPrefix(upper, "HIDE SEND ") {
		normalizedUpper := upper
		toValidate := content
		if strings.HasPrefix(upper, "HIDE ") {
			normalizedUpper = strings.TrimSpace(upper[5:]) // strip "HIDE "
			toValidate = strings.TrimSpace(content[5:])
		}

		// Check for block form
		// SEND ... MESSAGE (implied newline/block follows)
		// OR SEND ... LIST
		if strings.HasSuffix(normalizedUpper, " MESSAGE") || strings.HasSuffix(normalizedUpper, " LIST") {
			v.blockStack = append(v.blockStack, "SEND")
		}
		// Check syntax using translator's logic or custom
		return v.validateSend(toValidate)
	}
	if strings.HasPrefix(upper, "LET ") {
		if strings.HasSuffix(upper, " BE") {
			v.blockStack = append(v.blockStack, "LET")
		}
		return v.validateLet(content)
	}

	switch {
	case upper == "END":
		if len(v.blockStack) == 0 {
			return fmt.Errorf("unexpected END command")
		}
		// Pop stack
		v.blockStack = v.blockStack[:len(v.blockStack)-1]
		return nil
	case upper == "ELSE":
		if len(v.blockStack) == 0 || v.blockStack[len(v.blockStack)-1] != "IF" {
			return fmt.Errorf("unexpected ELSE command (not inside IF block)")
		}
		return nil
	case strings.HasPrefix(upper, "ELSE IF "):
		if len(v.blockStack) == 0 || v.blockStack[len(v.blockStack)-1] != "IF" {
			return fmt.Errorf("unexpected ELSE IF command (not inside IF block)")
		}
		return v.validateElseIf(content)
	case strings.HasPrefix(upper, "ELSE "):
		// Inline ELSE ...
		body := strings.TrimSpace(content[4:])
		if strings.HasSuffix(strings.ToUpper(body), "END") {
			body = strings.TrimSpace(body[:len(body)-3])
		}
		return v.validateValue(body)
	case strings.HasPrefix(upper, "INPUT"):
		return v.validateInput(content)
	case strings.HasPrefix(upper, "FACT"):
		return v.validateFact(content)
	case strings.HasPrefix(upper, "IF"):
		// IF ... THEN -> Block
		// IF ... THEN ... ELSE ... -> Statement
		// Simplest check: does it end with THEN?
		if strings.HasSuffix(upper, " THEN") {
			v.blockStack = append(v.blockStack, "IF")
		}
		return v.validateIf(content)
	case strings.HasPrefix(upper, "LIST"):
		return v.validateList(content)
	case strings.HasPrefix(upper, "USE "):
		return v.validateUse(strings.TrimSpace(content[3:])) // Ensure binding exists? Hard to track scope statically perfectly without full pass.
		// For static verification requested: "Only worry about mistakes that can be statically verified."
		// Checking bindings requires tracking state through the template.

	case upper == "USE":
		return nil
	default:
		// Unknown directive
		return fmt.Errorf("unknown Parley directive: %s", content)
	}
}

func (v *Validator) validateInput(content string) error {
	fields := strings.Fields(content)
	if len(fields) == 1 {
		return nil // {{ INPUT }} - valid, refers to implicit prompt/input
	}
	if len(fields) > 2 {
		return fmt.Errorf("invalid INPUT syntax: %s", content)
	}
	inputName := fields[1]

	// Check if inputName is defined for current agent
	currentAgent, ok := v.ctx.Agents[v.ctx.CurrentAgent]
	if !ok {
		// If we don't know the current agent, we can't strict validate inputs?
		// Or maybe CurrentAgent is optional.
		return nil
	}
	if !currentAgent.Inputs[inputName] {
		return fmt.Errorf("input '%s' is not defined for agent '%s'", inputName, v.ctx.CurrentAgent)
	}

	return nil
}

func (v *Validator) validateSend(content string) error {
	// "SEND <agent> ..."
	body := strings.TrimSpace(content[len("SEND "):])
	agentToken, remainder := splitFirstToken(body)
	if agentToken == "" {
		return fmt.Errorf("invalid SEND directive: missing agent")
	}

	// Check if agent exists
	if !v.agentExists(agentToken) {
		// Check if it's "IN library"
		if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(remainder)), "IN ") {
			// Handle library agent check later
		} else {
			return fmt.Errorf("undefined agent: %s", agentToken)
		}
	}

	// Handle "IN library"
	// "SEND agent IN library ..."
	agentToCheck := agentToken
	remainder = strings.TrimSpace(remainder)
	upper := strings.ToUpper(remainder)
	if strings.HasPrefix(upper, "IN ") {
		remainder = strings.TrimSpace(remainder[3:])
		libraryToken, rest := splitFirstToken(remainder)
		if libraryToken == "" {
			return fmt.Errorf("invalid SEND directive: missing library name")
		}
		agentToCheck = fmt.Sprintf("%s.%s", libraryToken, agentToken)
		remainder = strings.TrimSpace(rest)

		// For library agents, we might not have them in Agents map if they are external?
		// But linter populates them if they are in the project?
		// The instructions said "only worry about ... statically verified".
		// Use v.agentExists(agentToCheck)
		if !v.agentExists(agentToCheck) {
			return fmt.Errorf("undefined library agent: %s", agentToCheck)
		}
	}

	// Recursively validate payload if it exists?
	// SEND ... MESSAGE <value>
	modeToken, payload := splitFirstToken(remainder)
	if modeToken != "" {
		// MESSAGE or LIST
		if payload != "" {
			// Check payload value string
			// But payload might be a block "..." which is not easily validated here without looking ahead for END?
			// The loop in Validate() handles parsing blocks.
			// Here we just see the opening tag.
			// If payload is literal "INPUT", validate it?
			// If payload is "INPUT name", validate "INPUT name".
		}
	}

	return nil
}

func (v *Validator) validateFact(content string) error {
	// FACT <label> [IN/FROM <agent>]
	// Parsing logic similar to translator.factExpression
	body := strings.TrimSpace(content[len("FACT"):])
	if body == "" {
		return fmt.Errorf("invalid FACT directive: missing label")
	}

	var label, agent string
	upper := strings.ToUpper(body)
	if strings.Contains(upper, " IN ") {
		idx := indexCaseInsensitive(body, " IN ")
		if idx != -1 {
			label = strings.TrimSpace(body[:idx])
			agent = strings.TrimSpace(body[idx+4:])
		}
	} else if strings.Contains(upper, " FROM ") {
		idx := indexCaseInsensitive(body, " FROM ")
		if idx != -1 {
			label = strings.TrimSpace(body[:idx])
			agent = strings.TrimSpace(body[idx+6:])
		}
	} else {
		// Local fact? or error?
		// "FACT <label>" -> implicit local fact? Or not supported?
		// documentation says: "FACT <label>".
		// Assuming global/context fact?
		// Linter: checks if agent is defined if explicit agent used.
		return nil
	}

	if agent != "" {
		if !v.agentExists(agent) {
			return fmt.Errorf("undefined agent in fact lookup: %s", agent)
		}
		// Verify fact exists on agent?
		agentInfo, ok := v.ctx.Agents[agent]
		if ok {
			// If agent defined, check facts
			if !agentInfo.Facts[label] {
				// Warning or Error? "assumes that myinput is a defined agent input" implies strictness.
				return fmt.Errorf("fact '%s' is not defined for agent '%s'", label, agent)
			}
		}
	}

	return nil
}

func (v *Validator) validateIf(content string) error {
	// IF <predicate> THEN ...
	// parse predicate
	trimmed := strings.TrimSpace(content[len("IF"):])
	// Find THEN
	idx := indexCaseInsensitive(trimmed, " THEN")
	if idx == -1 {
		return fmt.Errorf("IF directive missing THEN")
	}
	predicate := strings.TrimSpace(trimmed[:idx])
	return v.validatePredicate(predicate)
}

func (v *Validator) validateElseIf(content string) error {
	trimmed := strings.TrimSpace(content[len("ELSE IF"):])
	idx := indexCaseInsensitive(trimmed, " THEN")
	if idx != -1 {
		trimmed = strings.TrimSpace(trimmed[:idx])
	}
	return v.validatePredicate(trimmed)
}

func (v *Validator) validateLet(content string) error {
	// LET <label> BE ...
	body := strings.TrimSpace(content[len("LET "):])
	label, remainder := splitFirstToken(body)
	if label == "" {
		return fmt.Errorf("LET missing label")
	}

	upper := strings.ToUpper(remainder)
	if !strings.HasPrefix(upper, "BE") {
		return fmt.Errorf("LET expects BE")
	}
	return nil // Payload validation complex due to block possibility
}

func (v *Validator) validateList(content string) error {
	// LIST <value> [AS/FROM ...]
	// Basic check: start must be value
	return nil
}

func (v *Validator) validateUse(label string) error {
	if label == "" {
		return nil
	} // USE implicit (last result?)
	return nil
}

func (v *Validator) validateValue(content string) error {
	content = strings.TrimSpace(content)
	if content == "" {
		return fmt.Errorf("missing value")
	}
	// Recursive checks...
	// Reuse validateDirective logic sort of?
	// Value can be INPUT ..., FACT ..., SEND ..., USE ...
	// Translator handles this by switching.
	upper := strings.ToUpper(content)
	switch {
	case strings.HasPrefix(upper, "INPUT"):
		return v.validateInput(content)
	case strings.HasPrefix(upper, "FACT"):
		return v.validateFact(content)
	case strings.HasPrefix(upper, "SEND"):
		return v.validateSend(content)
	case strings.HasPrefix(upper, "HIDE SEND"):
		return v.validateSend(strings.TrimSpace(content[5:]))
	}
	// Could be implicit fact name?
	// "parley_forms.md": <value> Any evaluable source...
	// If it's just a word, treat as implicit fact/binding?
	return nil
}

func (v *Validator) validatePredicate(content string) error {
	// <value> IS <value>
	// Just ensure structure is roughly sane?
	// Check keywords IS, IS NOT, HAS, EMPTY?
	upper := strings.ToUpper(content)
	if strings.Contains(upper, " IS ") || strings.Contains(upper, " HAS ") || strings.HasSuffix(upper, " EMPTY") {
		return nil
	}
	// Allow simple value as predicate (truthy check)?
	// Docs say: "Predicates usually read like ... IS ..."
	// Translator supports IS, IS NOT, HAS, IS EMPTY.
	// Assuming these are required patterns.
	return fmt.Errorf("invalid predicate format: %s", content)
}

func (v *Validator) agentExists(name string) bool {
	_, ok := v.ctx.Agents[name]
	return ok
}

// Utils

func indexCaseInsensitive(s, substr string) int {
	return strings.Index(strings.ToUpper(s), strings.ToUpper(substr))
}

// ValidationContext builder from Linter structures?
// Current linter.go has checkDuplicateAgentNames etc.
// But validation happens inside LintSpecFile.
// We should update LintSpecFile to build ValidationContext and call Validator.validate()
