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
	math_lib "github.com/robbyriverside/agencia/lib/math"
	mcp_lib "github.com/robbyriverside/agencia/lib/mcp"
	"github.com/robbyriverside/agencia/lib/rag"
	text_lib "github.com/robbyriverside/agencia/lib/text"
	time_lib "github.com/robbyriverside/agencia/lib/time"
	web_lib "github.com/robbyriverside/agencia/lib/web"
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
}

type Libraries map[string]Registry

var libraries Libraries = map[string]Registry{
	"rag":  {Agents: rag.Agents},
	"time": {Agents: time_lib.Agents},
	"math": {Agents: math_lib.Agents},
	"text": {Agents: text_lib.Agents},
	"web":  {Agents: web_lib.Agents},
	"mcp":  {Agents: mcp_lib.Agents},
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

func normalizeChatInput(input string) string {
	trimmed := strings.TrimSpace(input)
	if strings.HasPrefix(trimmed, "{") {
		var tmp map[string]any
		if err := json.Unmarshal([]byte(trimmed), &tmp); err == nil {
			if msg, ok := tmp["message"].(string); ok {
				return msg
			}
		}
	}
	return input
}

func sanitizeAgentOutput(out string) string {
	if utf8.ValidString(out) {
		return out
	}
	return strings.ToValidUTF8(out, "�")
}

func (r *Registry) runAgent(ctx context.Context, chat *Chat, name string, input string, isPrint bool) (AgentResult, *TraceCard) {
	run := NewRun(r, chat)
	run.IsPrint = isPrint
	res := run.CallAgent(ctx, name, input)
	if chat != nil {
		if res.Ran && res.Error == nil && chat.Registry != nil {
			if agent := chat.Registry.Agents[chat.StartAgent]; agent != nil {
				run.ExtractAgentMemory(ctx, agent, input, sanitizeAgentOutput(res.Output))
			}
		}
		chat.Cards = append(chat.Cards, run.Card)
	}
	return res, run.Card
}

// Run is the main entrypoint for calling an agent
func (r *Registry) Run(ctx context.Context, chat *Chat, name string, input string) (string, *TraceCard) {
	input = normalizeChatInput(input)
	res, card := r.runAgent(ctx, chat, name, input, false)
	if res.Error != nil {
		logs.Error("[AGENT ERROR]", res.Error)
		return res.Error.Error(), card
	}
	if !res.Ran {
		logs.Info("[INFO] Agent did not run (skipped).", res.AgentName)
		return "did not run", card
	}
	return sanitizeAgentOutput(res.Output), card
}

// RunPrint is the main entrypoint for calling an agent from the CLI
func (r *Registry) RunPrint(ctx context.Context, chat *Chat, name string, input string) error {
	input = normalizeChatInput(input)
	res, card := r.runAgent(ctx, chat, name, input, true)
	if res.Error != nil {
		return fmt.Errorf("[AGENT ERROR] %v", res.Error)
	}
	if !res.Ran {
		fmt.Printf("[INFO] Agent '%s' did not run (skipped).\n", res.AgentName)
		return nil
	}
	out := sanitizeAgentOutput(res.Output)
	fmt.Println(out)
	if card != nil {
		card.SaveMarkdown("trace.md", !IsVerbose())
	}

	return nil
}
