package agencia

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"gopkg.in/yaml.v3"
)

type LintResult struct {
	Errors   []string
	Warnings []string
	Valid    bool
	Summary  string
}

func LintSpecFile(source []byte) LintResult {
	var errors []string
	duplicateErrors := checkDuplicateAgentNames(source)
	errors = append(errors, duplicateErrors...)

	var root yaml.Node
	err := yaml.Unmarshal(source, &root)
	if err != nil {
		errors = append(errors, fmt.Sprintf("YAML parsing error: %v", err))
		return LintResult{
			Errors:  errors,
			Valid:   false,
			Summary: "Problem: Failed to parse YAML. Spec is invalid.  Details: " + err.Error(),
		}
	}

	var warnings []string
	agentNames := map[string]bool{}
	usedAgents := map[string]bool{}
	definedAgents := map[string]*yaml.Node{}

	// Locate top-level "agents" mapping with defensive traversal
	var agentsNode *yaml.Node
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		rootMap := root.Content[0]
		if rootMap.Kind == yaml.MappingNode {
			for i := 0; i < len(rootMap.Content)-1; i += 2 {
				keyNode := rootMap.Content[i]
				if keyNode.Value == "agents" {
					agentsNode = rootMap.Content[i+1]
					break
				}
			}
		}
	}
	if agentsNode == nil {
		errors = append(errors, "Problem: The 'agents' section is missing from the YAML root. Please add an 'agents' mapping to define your agents.")
		return LintResult{
			Errors:  errors,
			Valid:   false,
			Summary: "Problem: 'agents' section is missing.",
		}
	}
	if agentsNode.Kind != yaml.MappingNode {
		errors = append(errors, "Problem: The 'agents' section exists but is not a mapping node. Please ensure 'agents' is a mapping (dictionary) of agent definitions.")
		return LintResult{
			Errors:  errors,
			Valid:   false,
			Summary: "Problem: 'agents' section has invalid structure.",
		}
	}

	// Collect all defined agents
	for i := 0; i < len(agentsNode.Content)-1; i += 2 {
		agentNameNode := agentsNode.Content[i]
		agentValueNode := agentsNode.Content[i+1]
		name := agentNameNode.Value
		if strings.Contains(name, ".") {
			continue // ignore library agents
		}
		agentNames[name] = true
		definedAgents[name] = agentValueNode
	}

	// Regex to find .Get "agentname" and .Start "agentname"
	referenceRegex := regexp.MustCompile(`\.(Get|Start)\s+"([^"]+)"`)

	// Validate each agent
	for name, node := range definedAgents {
		kindSet := map[string]bool{}
		hasDescription := false
		var hasInputs bool
		var listenersNode *yaml.Node
		for i := 0; i < len(node.Content)-1; i += 2 {
			key := node.Content[i].Value
			val := node.Content[i+1]
			switch key {
			case "prompt", "template", "alias", "function":
				kindSet[key] = true
				// For prompt and template, check for .Get and .Start references
				if key == "prompt" || key == "template" {
					matches := referenceRegex.FindAllStringSubmatch(val.Value, -1)
					for _, match := range matches {
						refAgent := match[2]
						if !agentNames[refAgent] && !strings.Contains(refAgent, ".") {
							errors = append(errors, fmt.Sprintf("Problem: Line %d: Agent '%s' references undefined agent '%s' via .%s. Please ensure all referenced agents exist.", val.Line, name, refAgent, match[1]))
						} else {
							usedAgents[refAgent] = true
						}
					}
				}
			case "description":
				hasDescription = true
			case "inputs":
				hasInputs = true
			case "listeners":
				listenersNode = val
			case "job":
				for _, item := range val.Content {
					if !agentNames[item.Value] && !strings.Contains(item.Value, ".") {
						errors = append(errors, fmt.Sprintf("Problem: Line %d: The job step '%s' in agent '%s' does not reference a valid agent. Please ensure all job steps refer to existing agents.", item.Line, item.Value, name))
					} else {
						usedAgents[item.Value] = true
					}
				}
			case "facts":
				// Validate scope field of declared facts
				if val.Kind == yaml.SequenceNode {
					for _, factNode := range val.Content {
						if factNode.Kind == yaml.MappingNode {
							for j := 0; j < len(factNode.Content)-1; j += 2 {
								factKey := factNode.Content[j].Value
								factVal := factNode.Content[j+1]
								if factKey == "scope" {
									scopeVal := factVal.Value
									if scopeVal != "global" && scopeVal != "local" {
										errors = append(errors, fmt.Sprintf("Problem: Line %d: Agent '%s' has a fact with invalid scope '%s'. Only 'global' and 'local' are allowed.", factVal.Line, name, scopeVal))
									}
								}
							}
						}
					}
				}
			}
		}

		if len(kindSet) == 0 {
			errors = append(errors, fmt.Sprintf("Problem: Line %d: Agent '%s' missing: prompt, template, or alias.", node.Line, name))
		} else if len(kindSet) > 1 {
			errors = append(errors, fmt.Sprintf("Problem: Line %d: Agent '%s' defines multiple action types: %v. Please specify only one of: prompt, template, or alias.", node.Line, name, keys(kindSet)))
		}

		if !hasDescription {
			// Only add warning if agent is used as a listener
			if usedAgents[name] {
				warnings = append(warnings, fmt.Sprintf("Reminder: Line %d: Agent '%s' is missing a description. Consider adding a description to clarify the agent's purpose.", node.Line, name))
			}
		}

		if listenersNode != nil {
			if kindSet["template"] {
				warnings = append(warnings, fmt.Sprintf("Reminder: Line %d: Agent '%s' is a template so listeners don't get called' field from templates.", listenersNode.Line, name))
			}
			if !hasInputs {
				warnings = append(warnings, fmt.Sprintf("Reminder: Line %d: Agent '%s' is used as a listener but has no inputs defined. Consider adding inputs to clarify expected data.", node.Line, name))
			}
			for _, item := range listenersNode.Content {
				listener := item.Value
				if !agentNames[listener] && !strings.Contains(listener, ".") {
					errors = append(errors, fmt.Sprintf("Problem: Line %d: Agent '%s' declares unknown listener '%s'. Please ensure all listeners refer to defined agents.", item.Line, name, listener))
				}
				usedAgents[listener] = true
			}
		}
	}

	// After processing agents, check usedAgents that are listeners for description and inputs
	for usedAgentName := range usedAgents {
		if agentNode, ok := definedAgents[usedAgentName]; ok {
			hasDescription := false
			hasInputs := false
			for i := 0; i < len(agentNode.Content)-1; i += 2 {
				key := agentNode.Content[i].Value
				if key == "description" {
					hasDescription = true
				}
				if key == "inputs" {
					hasInputs = true
				}
			}
			if !hasDescription || !hasInputs {
				errors = append(errors, fmt.Sprintf("Problem: Agent '%s' is used as a listener but is missing description or inputs, making it invalid as a listener.", usedAgentName))
			}
		}
	}

	// Detect unused agents
	for name := range agentNames {
		if !usedAgents[name] {
			warnings = append(warnings, fmt.Sprintf("Reminder: Agent '%s' is defined but never used. Ignore if starting agent.", name))
		}
	}

	// Detect self-recursive agents
	for name, node := range definedAgents {
		for i := 0; i < len(node.Content)-1; i += 2 {
			key := node.Content[i].Value
			val := node.Content[i+1]
			if (key == "prompt" || key == "template") && strings.Contains(val.Value, name) {
				errors = append(errors, fmt.Sprintf("Problem: on line %d: Agent '%s' references itself (perhaps indirectly). This can cause an infinite loop that never returns.", val.Line, name))
			}
		}
	}

	// Only run schema validation if no errors so far
	if len(errors) == 0 {
		schemaErrors := validateAgainstSchema(source)
		errors = append(errors, schemaErrors...)
	}

	summary := fmt.Sprintf("Linting complete. Found %d error(s), %d warning(s).", len(errors), len(warnings))
	return LintResult{
		Errors:   errors,
		Warnings: warnings,
		Valid:    len(errors) == 0,
		Summary:  summary,
	}
}

func keys(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

func checkDuplicateAgentNames(source []byte) []string {
	lines := strings.Split(string(source), "\n")
	agentNames := map[string]int{}
	var errors []string
	inAgentsSection := false
	agentsIndent := -1
	firstAgentIndent := -1

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "agents:") {
			inAgentsSection = true
			agentsIndent = len(line) - len(strings.TrimLeft(line, " "))
			continue
		}
		if inAgentsSection {
			// Check if line is empty or a comment
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
			// Determine current line indent
			currentIndent := len(line) - len(strings.TrimLeft(line, " "))
			if currentIndent <= agentsIndent {
				// End of agents section
				break
			}
			// Determine first agent indent if not set and line looks like an agent name key
			if firstAgentIndent == -1 && strings.Contains(trimmed, ":") {
				parts := strings.SplitN(trimmed, ":", 2)
				key := strings.TrimSpace(parts[0])
				if !strings.Contains(key, ".") {
					firstAgentIndent = currentIndent
				}
			}
			// Only treat lines at the firstAgentIndent as agent names
			if currentIndent == firstAgentIndent && strings.Contains(trimmed, ":") {
				parts := strings.SplitN(trimmed, ":", 2)
				key := strings.TrimSpace(parts[0])
				// Ignore keys with dots (library agents)
				if strings.Contains(key, ".") {
					continue
				}
				if prevLine, exists := agentNames[key]; exists {
					errors = append(errors, fmt.Sprintf("Problem: Duplicate agent name '%s' found at line %d (previously defined at line %d). Agent names must be unique.", key, i+1, prevLine))
				} else {
					agentNames[key] = i + 1
				}
			}
		}
	}
	return errors
}

// validateAgainstSchema validates the source against the spec_schema.json schema.
func validateAgainstSchema(source []byte) []string {
	var errors []string
	schemaData, err := os.ReadFile("spec_schema.json")
	if err != nil {
		errors = append(errors, fmt.Sprintf("Problem: The spec is invalid. : Failed to read spec_schema.json: %v", err))
		return errors
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("spec_schema.json", strings.NewReader(string(schemaData))); err != nil {
		errors = append(errors, fmt.Sprintf("Problem: The spec is invalid. : Failed to load schema resource: %v", err))
		return errors
	}
	schema, err := compiler.Compile("spec_schema.json")
	if err != nil {
		errors = append(errors, fmt.Sprintf("Problem: The spec is invalid. : Failed to compile schema: %v", err))
		return errors
	}
	var jsonData map[string]interface{}
	if err := yaml.Unmarshal(source, &jsonData); err != nil {
		errors = append(errors, fmt.Sprintf("Problem: The spec is invalid. : Failed to unmarshal YAML: %v", err))
		return errors
	}
	jsonBytes, err := json.Marshal(jsonData)
	if err != nil {
		errors = append(errors, fmt.Sprintf("Problem: The spec is invalid. : Failed to marshal YAML as JSON: %v", err))
		return errors
	}
	var jsonObj interface{}
	if err := json.Unmarshal(jsonBytes, &jsonObj); err != nil {
		errors = append(errors, fmt.Sprintf("Problem: The spec is invalid. : Failed to parse YAML as JSON: %v", err))
		return errors
	}
	if err := schema.Validate(jsonObj); err != nil {
		errors = append(errors, fmt.Sprintf("Problem: The spec is invalid. : %v", err))
	}
	return errors
}
