package agencia

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// SpecRunnerConfig describes how to bootstrap a SpecRunner instance.
type SpecRunnerConfig struct {
	Spec              string
	SpecFile          string
	StartAgent        string
	PrintFacts        bool
	PrintObservations bool
	Output            io.Writer
}

// SpecRunner orchestrates one-off and interactive runs for a single spec.
type SpecRunner struct {
	Registry *Registry
	Session  *Chat

	printFacts        bool
	printObservations bool
	writer            io.Writer
}

// NewSpecRunner builds a runnable spec harness from inline spec content or a file path.
func NewSpecRunner(cfg SpecRunnerConfig) (*SpecRunner, error) {
	spec, err := cfg.resolveSpec()
	if err != nil {
		return nil, err
	}
	registry, err := NewRegistry(spec)
	if err != nil {
		return nil, err
	}
	startAgent := strings.TrimSpace(cfg.StartAgent)
	if startAgent == "" {
		if len(registry.Agents) == 1 {
			for name := range registry.Agents {
				startAgent = name
			}
		} else {
			return nil, errors.New("spec runner requires a start agent when multiple agents are defined")
		}
	}
	if _, ok := registry.Agents[startAgent]; !ok {
		return nil, fmt.Errorf("start agent %q not found in spec", startAgent)
	}
	chat := NewChat(startAgent, registry)
	writer := cfg.Output
	if writer == nil {
		writer = os.Stdout
	}
	return &SpecRunner{
		Registry:          registry,
		Session:           chat,
		printFacts:        cfg.PrintFacts,
		printObservations: cfg.PrintObservations,
		writer:            writer,
	}, nil
}

func (cfg SpecRunnerConfig) resolveSpec() (string, error) {
	if strings.TrimSpace(cfg.Spec) != "" {
		return cfg.Spec, nil
	}
	if strings.TrimSpace(cfg.SpecFile) == "" {
		return "", errors.New("spec runner requires Spec or SpecFile")
	}
	data, err := os.ReadFile(cfg.SpecFile)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

type runOptions struct {
	agent string
}

// RunOption customizes a single SpecRunner Run invocation.
type RunOption func(*runOptions)

// WithRunAgent overrides the agent used for a specific Run invocation.
func WithRunAgent(agent string) RunOption {
	return func(opts *runOptions) {
		opts.agent = strings.TrimSpace(agent)
	}
}

// Run executes a one-off prompt against the configured spec and returns the agent output.
func (s *SpecRunner) Run(ctx context.Context, prompt string, opts ...RunOption) (string, error) {
	if s == nil || s.Registry == nil || s.Session == nil {
		return "", errors.New("spec runner is not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	settings := runOptions{agent: s.Session.StartAgent}
	for _, opt := range opts {
		opt(&settings)
	}
	if settings.agent == "" {
		return "", errors.New("start agent is required")
	}
	if _, ok := s.Registry.Agents[settings.agent]; !ok {
		return "", fmt.Errorf("agent %q not found in registry", settings.agent)
	}
	s.Session.SetStartAgent(settings.agent)
	input := normalizeChatInput(prompt)
	result, _ := s.Registry.runAgent(ctx, s.Session, settings.agent, input, false)
	if result.Error != nil {
		return "", result.Error
	}
	if !result.Ran {
		return "", fmt.Errorf("agent %s did not run", result.AgentName)
	}
	output := sanitizeAgentOutput(result.Output)
	if s.printFacts || s.printObservations {
		s.printState()
	}
	return output, nil
}

// Chat starts an interactive CLI loop backed by the spec runner.
func (s *SpecRunner) Chat(ctx context.Context) error {
	return s.chatInteractive(ctx, os.Stdin, s.writer)
}

func (s *SpecRunner) chatInteractive(ctx context.Context, in io.Reader, out io.Writer) error {
	if s == nil || s.Registry == nil || s.Session == nil {
		return errors.New("spec runner is not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if in == nil {
		return errors.New("chat input source cannot be nil")
	}
	if out == nil {
		out = os.Stdout
	}
	fmt.Fprintf(out, "Starting Agencia chat with agent %s. Type 'exit' to quit.\n", s.Session.StartAgent)
	scanner := bufio.NewScanner(in)
	for {
		fmt.Fprint(out, "> ")
		if !scanner.Scan() {
			break
		}
		text := scanner.Text()
		if strings.EqualFold(strings.TrimSpace(text), "exit") {
			break
		}
		if strings.TrimSpace(text) == "" {
			continue
		}
		response, err := s.Run(ctx, text)
		if err != nil {
			fmt.Fprintf(out, "[error] %v\n", err)
			continue
		}
		fmt.Fprintf(out, "%s\n", response)
	}
	return scanner.Err()
}

func (s *SpecRunner) printState() {
	if s.writer == nil || s.Session == nil {
		return
	}
	if s.printFacts {
		fmt.Fprintln(s.writer, "\n[facts]")
		if len(s.Session.Facts) == 0 {
			fmt.Fprintln(s.writer, "(none)")
		} else {
			keys := sortedFactKeys(s.Session.Facts)
			for _, key := range keys {
				fmt.Fprintf(s.writer, "%s: %v\n", key, s.Session.Facts[key])
			}
		}
	}
	if s.printObservations {
		fmt.Fprintln(s.writer, "\n[observations]")
		if len(s.Session.Observations) == 0 {
			fmt.Fprintln(s.writer, "(none)")
		} else {
			roleKeys := make([]string, 0, len(s.Session.Observations))
			for role := range s.Session.Observations {
				roleKeys = append(roleKeys, role)
			}
			sort.Strings(roleKeys)
			for _, role := range roleKeys {
				fmt.Fprintf(s.writer, "%s:\n", role)
				keys := sortedObservationKeys(s.Session.Observations[role])
				for _, key := range keys {
					values := s.Session.Observations[role][key]
					for _, val := range values {
						fmt.Fprintf(s.writer, "  %s: %s\n", key, val)
					}
				}
			}
		}
	}
}

func sortedFactKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedObservationKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
