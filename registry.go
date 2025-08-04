package agencia

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/robbyriverside/agencia/agents"
	"github.com/robbyriverside/agencia/lib/rag"
	"github.com/robbyriverside/agencia/logs"
)

const maxCallDepth = 6 // safeguard against runaway .Get or alias recursion

type AgentNotFoundError struct {
	AgentName string
}

func (e *AgentNotFoundError) Error() string {
	return fmt.Sprintf("could not find agent: %s", e.AgentName)
}

type RegistryCaller interface {
	CallAI(ctx context.Context, agent *agents.Agent, prompt string, tmplCtx *TemplateContext) (string, error)
}

type Registry struct {
	Agents map[string]*agents.Agent
	Roles  map[string]*agents.AgentRole
	Chat   *Chat
}

type Libraries map[string]Registry

var libraries Libraries = map[string]Registry{
	"rag": {Agents: rag.Agents},
}

// LookupAgent resolves both unqualified and qualified agent names
func (r *Registry) LookupAgent(name string) (*agents.Agent, error) {
	if !strings.Contains(name, ".") {
		agent, ok := r.Agents[name]
		if !ok {
			return nil, &AgentNotFoundError{AgentName: name}
		}
		return agent, nil
	}

	parts := strings.SplitN(name, ".", 2)
	pkgName, agentName := parts[0], parts[1]
	pkg, ok := libraries[pkgName]
	if !ok {
		return nil, &AgentNotFoundError{AgentName: name}
	}
	agent, ok := pkg.Agents[agentName]
	if !ok {
		return nil, &AgentNotFoundError{AgentName: name}
	}
	return agent, nil
}

func (r *Registry) LookupRole(name string) (*agents.AgentRole, bool) {
	val, ok := r.Roles[name]
	return val, ok
}

func (r *RunContext) Errorf(format string, args ...any) {
	r.Logf(fmt.Sprintf("[ERROR] %s", format), args...)
}

func (r *RunContext) Logf(format string, args ...any) {
	if r.IsPrint {
		log.Printf(format, args...)
	}
	r.Card.Logs = append(r.Card.Logs, &LogMessage{
		Message:   fmt.Sprintf(format, args...),
		Timestamp: time.Now(),
	})
}

func (r *Registry) RegisterAgent(agent *agents.Agent) {
	if r.Agents == nil {
		r.Agents = make(map[string]*agents.Agent)
	}
	r.Agents[agent.Name] = agent
}

// Run is the main entrypoint for calling an agent
func (r *Registry) Run(ctx context.Context, name string, input string) (string, *TraceCard) {
	// If input is a JSON object with a "message" key, extract and use only that value
	if strings.HasPrefix(input, "{") {
		var tmp map[string]interface{}
		if err := json.Unmarshal([]byte(input), &tmp); err == nil {
			if msg, ok := tmp["message"].(string); ok {
				input = msg
			}
		}
	}
	run := NewRun(r, defaultChat)
	res := run.CallAgent(ctx, name, input)
	if res.Error != nil {
		logs.Error("[AGENT ERROR]", res.Error)
		return res.Error.Error(), run.Card
	}
	if !res.Ran {
		logs.Info("[INFO] Agent did not run (skipped).", res.AgentName)
		return "did not run", run.Card
	}
	out := res.Output
	// logs.Info("[AGENT OUTPUT]", out)
	if !utf8.ValidString(out) {
		out = strings.ToValidUTF8(out, "�")
	}
	if strings.TrimSpace(out) == "" {
		out = "no output"
	}

	if defaultChat != nil {
		agent := defaultChat.Registry.Agents[defaultChat.StartAgent]
		if agent != nil {
			run.ExtractAgentMemory(ctx, agent, input, out)
		}
		defaultChat.Cards = append(defaultChat.Cards, run.Card)
	}
	return out, run.Card
}

// RunPrint is the main entrypoint for calling an agent from the CLI
func (r *Registry) RunPrint(ctx context.Context, name string, input string) error {
	run := NewRun(r, defaultChat)
	run.IsPrint = true
	res := run.CallAgent(ctx, name, input)
	if res.Error != nil {
		return fmt.Errorf("[AGENT ERROR] %v", res.Error)
	}
	if !res.Ran {
		fmt.Printf("[INFO] Agent '%s' did not run (skipped).\n", res.AgentName)
		return nil
	}
	out := res.Output
	if !utf8.ValidString(out) {
		out = strings.ToValidUTF8(out, "�")
	}
	fmt.Println(out)
	card := run.Card
	if card != nil {
		card.SaveMarkdown("trace.md", !IsVerbose())
	}

	return nil
}
