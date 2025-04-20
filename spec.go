package agencia

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type AgentSpec struct {
	Agents map[string]Agent `yaml:"agents,omitempty"`
}

func (s *AgentSpec) String() string {
	b, _ := yaml.Marshal(s)
	return string(b)
}

func Compile(specfile string) (*AgentSpec, error) {
	spec, err := loadAgentSpec(specfile)
	if err != nil {
		return nil, fmt.Errorf("[LOAD ERROR] %w", err)
	}
	if err := RegisterAgents(spec); err != nil {
		return nil, fmt.Errorf("[REGISTER ERROR] %w", err)
	}
	return &spec, nil
}

func loadAgentSpec(filename string) (AgentSpec, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return AgentSpec{}, fmt.Errorf("cannot read agent file %s: %w", filename, err)
	}
	// fmt.Printf("[INFO] Loading agent file %s\n", filename)
	var spec AgentSpec
	err = yaml.Unmarshal(data, &spec)
	if err != nil {
		return AgentSpec{}, fmt.Errorf("invalid YAML in %s: %w", filename, err)
	}
	return spec, nil
}

func RegisterAgents(spec AgentSpec) error {
	// fmt.Println("[INFO] Registering agents...", spec)
	if spec.Agents != nil {
		for name, agent := range spec.Agents {
			agent.Name = name
			agentRegistry[name] = agent
			if (agent.Function != "" && agent.Template != "") || (agent.Function != "" && agent.Prompt != "") || (agent.Template != "" && agent.Prompt != "") {
				return fmt.Errorf("agent '%s' has more than one of Function, Template, and Prompt set", name)
			}
		}
	}
	if len(agentRegistry) == 0 {
		return errors.New("no agents defined")
	}
	return nil
}
