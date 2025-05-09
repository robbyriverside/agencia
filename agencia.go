// package agencia
// agencia is a package for building and running agent-based applications
// package management:
//
//	 To avoid package loops:
//		   agencia uses the libraries in agencia/lib
//		   agencia and agencia/lib sub packages use the agents in agencia/agents
//
// agencia management: agencia
// agent management:   agencia/agents
// libraries:          agencia/lib
package agencia

import (
	"errors"
	"fmt"
	"os"

	"github.com/robbyriverside/agencia/agents"
	"gopkg.in/yaml.v3"
)

type ConfigOptions struct {
	Verbose bool `short:"v" long:"verbose" required:"false" description:"Verbose messages"`
}

var Options *ConfigOptions

func GetOptions() *ConfigOptions {
	if Options == nil {
		Options = &ConfigOptions{}
	}
	return Options
}

func IsVerbose() bool {
	return Options != nil && Options.Verbose
}

type AgentSpec struct {
	Agents map[string]*agents.Agent `yaml:"agents,omitempty"`
}

type AgentResult struct {
	Output    string
	Ran       bool
	Error     error
	AgentName string
}

func (s *AgentSpec) String() string {
	b, _ := yaml.Marshal(s)
	return string(b)
}

func LoadRegistry(specfile string) (*Registry, error) {
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

func NewRegistry(spec string) (*Registry, error) {
	specBytes := []byte(spec)
	res := LintSpecFile(specBytes)
	if !res.Valid {
		return nil, errors.New(res.Summary)
	}
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

func loadAgentSpecFile(filename string) (*AgentSpec, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("cannot read agent file %s: %w", filename, err)
	}
	return loadAgentSpec(data)
}

func loadAgentSpec(specbytes []byte) (*AgentSpec, error) {
	var spec AgentSpec
	err := yaml.Unmarshal(specbytes, &spec)
	if err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}
	return &spec, nil
}

func RegisterAgents(spec *AgentSpec) (*Registry, error) {
	registry := &Registry{Agents: make(map[string]*agents.Agent)}
	if spec.Agents != nil {
		for name, agent := range spec.Agents {
			agent.Name = name
			if agent.Inputs != nil {
				for k, v := range agent.Inputs {
					if v.Type == "" {
						v.Type = "string"
					}
					v.Name = k
				}
			}
			registry.Agents[name] = agent
			if !agent.IsValid() {
				return nil, fmt.Errorf("agent '%s' must be one of Function, Alias, Template, and Prompt", name)
			}
		}
	}
	if len(registry.Agents) == 0 {
		return nil, errors.New("spec did not contain any agents - be sure you have the agents key in the spec file")
	}
	return registry, nil
}
