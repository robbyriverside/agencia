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
	Agents map[string]agents.Agent `yaml:"agents,omitempty"`
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
	registry, err := RegisterAgents(*spec)
	if err != nil {
		return nil, fmt.Errorf("[REGISTER ERROR] %w", err)
	}
	return registry, nil
}

func NewRegistry(spec string) (*Registry, error) {
	specBytes := []byte(spec)
	agentSpec, err := loadAgentSpec(specBytes)
	if err != nil {
		return nil, fmt.Errorf("[LOAD ERROR] %w", err)
	}
	registry, err := RegisterAgents(*agentSpec)
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

func RegisterAgents(spec AgentSpec) (*Registry, error) {
	registry := &Registry{Agents: make(map[string]*agents.Agent)}
	if spec.Agents != nil {
		for name, agent := range spec.Agents {
			agent.Name = name
			registry.Agents[name] = &agent
			if (agent.Function != nil && agent.Template != "") || (agent.Function != nil && agent.Prompt != "") || (agent.Template != "" && agent.Prompt != "") {
				return nil, fmt.Errorf("agent '%s' has more than one of Function, Template, and Prompt set", name)
			}
		}
	}
	if len(registry.Agents) == 0 {
		return nil, errors.New("no agents defined")
	}
	return registry, nil
}
