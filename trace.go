package agencia

import (
	"fmt"
	"io"
	"log"
	"os"
	"time"
)

type LogMessage struct {
	Message   string
	Timestamp time.Time
}

// Warning: DO NOT JSON Encode TraceCard directly, as it
//
//	may contain a cycle (e.g. recursive calls)
type TraceCard struct {
	AgentName        string
	Input            string
	Inputs           map[string]any
	Output           string
	Prompt           string
	Error            error
	Ran              bool
	PriorCard        *TraceCard
	BranchCards      []*TraceCard
	Logs             []*LogMessage
	Facts            map[string]any // facts set by this agent
	LocalFacts       map[string]any // local facts set by this agent
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

func (c *TraceCard) String() string {
	errstr := "no error"
	if c.Error != nil {
		errstr = fmt.Sprintf("Error: %s\n", c.Error)
	}
	ranstr := "did not run"
	if c.Ran {
		ranstr = "agent ran"
	}
	inputs := fmt.Sprintf("%v", c.Inputs)
	inputs = inputs[4 : len(inputs)-1]
	facts := fmt.Sprintf("%v", c.Facts)
	facts = facts[4 : len(facts)-1]
	locals := fmt.Sprintf("%v", c.LocalFacts)
	locals = locals[4 : len(locals)-1]
	if len(locals) == 0 {
		locals = "none"
	} else {
		locals = fmt.Sprintf("%q", locals)
	}
	prompt := c.Prompt
	if len(prompt) > 0 {
		prompt = fmt.Sprintf("\nPrompt: %q\n", prompt)
	}
	if len(facts) == 0 {
		facts = "none"
	} else {
		facts = fmt.Sprintf("%q", facts)
	}
	if len(inputs) == 0 {
		inputs = "none"
	} else {
		inputs = fmt.Sprintf("%q", inputs)
	}

	results := fmt.Sprintf("Agent: %s\nInput: \"%s\"\nOutput: \"%s\"\n%s%s\n%s\nInputs: %s\nFacts: %s\nLocalFacts: %s\nTokens: %d (Prompt: %d, Completion: %d)",
		c.AgentName, c.Input, c.Output, prompt, ranstr, errstr, inputs, facts, locals, c.TotalTokens, c.PromptTokens, c.CompletionTokens)

	if len(c.Logs) == 0 {
		results += "\nno logs"
	} else {
		results += "\nLogs:"
	}
	for _, log := range c.Logs {
		results += fmt.Sprintf("\n  %s: %s", log.Timestamp.Format(time.RFC3339), log.Message)
	}
	return results
}

func (c *TraceCard) ShortString() string {
	errstr := "no error"
	if c.Error != nil {
		errstr = fmt.Sprintf("Error: %s\n", c.Error)
	}
	prior := "none"
	if c.PriorCard != nil {
		prior = c.PriorCard.AgentName
	}
	return fmt.Sprintf("Agent: %s\nFrom: %s\nInput: \"%s\"\nOutput: \"%s\"\n%s\nTokens: %d",
		c.AgentName, prior, c.Input, c.Output, errstr, c.TotalTokens)
}

func (r *RunContext) NewTraceCard(agent, input string) *TraceCard {
	card := &TraceCard{
		AgentName:   agent,
		Input:       input,
		Inputs:      make(map[string]any),
		PriorCard:   r.Card,
		BranchCards: make([]*TraceCard, 0),
		Logs:        make([]*LogMessage, 0),
		Facts:       make(map[string]any, 0),
		LocalFacts:  make(map[string]any, 0),
	}
	return card
}

func (c *TraceCard) SaveMarkdown(filename string, short ...bool) error {
	log.Println("Saving trace to", filename)
	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("[TRACE CARD ERROR] Failed to create file: %v", err)
	}
	defer f.Close()

	c.WriteMarkdown(f, short...)
	return nil
}

func (c *TraceCard) WriteMarkdown(w io.Writer, short ...bool) {
	if c == nil {
		return
	}
	if len(short) > 0 && short[0] {
		c.WriteMarkdownShort(w, 1, 1)
		return
	}
	fmt.Fprintf(w, "# Agent Trace: %s\n", c.AgentName)

	c.WriteMarkdownLevel(w, 1, 1)
}

func (c *TraceCard) WriteMarkdownLevel(w io.Writer, index, level int) {
	if c == nil {
		return
	}

	var from string
	if level > 1 {
		from = fmt.Sprintf(" From: %s\n", c.PriorCard.AgentName)
	}
	fmt.Fprintf(w, "\n## %d.%d: %s%s\n", level, index, c.AgentName, from)

	fmt.Fprintf(w, "\n```%s\n```\n", c.String()) // TODO: add from, level, index

	for i, card := range c.BranchCards {
		card.WriteMarkdownLevel(w, i+1, level+1)
	}
}

func (c *TraceCard) WriteMarkdownShort(w io.Writer, index, level int) {
	if c == nil {
		return
	}
	fmt.Fprintf(w, "\n```\nLevel: %d.%d\n%s\n```", level, index, c.ShortString()) // TODO: add from, level, index

	for i, card := range c.BranchCards {
		card.WriteMarkdownShort(w, i+1, level+1)
	}
}
