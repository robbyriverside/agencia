package agents

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type AgentSpec struct {
	Agents map[string]Agent `yaml:"agents,omitempty"`
}

type Registry map[string]Agent

func (s *AgentSpec) String() string {
	b, _ := yaml.Marshal(s)
	return string(b)
}

func CompileFile(specfile string) (Registry, error) {
	spec, err := loadAgentSpecFile(specfile)
	if err != nil {
		return nil, fmt.Errorf("[LOAD ERROR] %w", err)
	}
	registry, err := RegisterAgents(spec)
	if err != nil {
		return nil, fmt.Errorf("[REGISTER ERROR] %w", err)
	}
	return registry, nil
}

func Compile(spec string) (Registry, error) {
	specBytes := []byte(spec)
	agentSpec, err := loadAgentSpec(specBytes)
	if err != nil {
		return nil, fmt.Errorf("[LOAD ERROR] %w", err)
	}
	registry, err := RegisterAgents(agentSpec)
	if err != nil {
		return nil, fmt.Errorf("[REGISTER ERROR] %w", err)
	}
	return registry, nil
}

func loadAgentSpecFile(filename string) (AgentSpec, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return AgentSpec{}, fmt.Errorf("cannot read agent file %s: %w", filename, err)
	}
	// fmt.Printf("[INFO] Loading agent file %s\n", filename)
	return loadAgentSpec(data)
}

func loadAgentSpec(specbytes []byte) (AgentSpec, error) {
	var spec AgentSpec
	err := yaml.Unmarshal(specbytes, &spec)
	if err != nil {
		return AgentSpec{}, fmt.Errorf("invalid YAML: %w", err)
	}
	return spec, nil
}

func RegisterAgents(spec AgentSpec) (Registry, error) {
	// fmt.Println("[INFO] Registering agents...", spec)
	registry := map[string]Agent{}
	if spec.Agents != nil {
		for name, agent := range spec.Agents {
			agent.Name = name
			registry[name] = agent
			if (agent.Function != nil && agent.Template != "") || (agent.Function != nil && agent.Prompt != "") || (agent.Template != "" && agent.Prompt != "") {
				return nil, fmt.Errorf("agent '%s' has more than one of Function, Template, and Prompt set", name)
			}
		}
	}
	if len(registry) == 0 {
		return nil, errors.New("no agents defined")
	}
	return registry, nil
}
