package agencia

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"

	"encoding/json"

	"github.com/gorilla/websocket"
	"github.com/robbyriverside/agencia/agents"
	"gopkg.in/yaml.v3"
)

var defaultChat *Chat

type Chat struct {
	Agent string
	Facts map[string]any
	// Observations       map[string][]string
	// TaggedObservations map[string][]string
	TaggedFacts map[string][]string // tag => list of agent.fact keys
	Registry    *Registry
	Cards       []*TraceCard
}

func NewChat(agent string) *Chat {
	return &Chat{
		Agent: agent,
		Facts: make(map[string]any),
		// Observations:      make(map[string][]string),
		// TaggedObservations: make(map[string][]string),
		TaggedFacts: make(map[string][]string),
	}
}

func ChatWebSocketHandler(w http.ResponseWriter, r *http.Request) {
	type ChatInitRequest struct {
		Agent string `json:"agent"`
		Spec  string `json:"spec"` // optionally store or use this
	}

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket upgrade failed:", err)
		return
	}
	defer conn.Close()

	_, initMsg, err := conn.ReadMessage()
	if err != nil {
		log.Printf("WebSocket init message error: %v", err)
		return
	}

	var initReq ChatInitRequest
	if err := json.Unmarshal(initMsg, &initReq); err != nil {
		log.Printf("Failed to decode chat init request: %v", err)
		return
	}
	// log.Println("Spec received:\n", initReq.Spec)

	if defaultChat == nil {
		defaultChat = NewChat(initReq.Agent)
	} else {
		defaultChat.Agent = initReq.Agent
	}
	registry, err := NewRegistry(initReq.Spec)
	if err != nil {
		log.Println("Failed to create registry:", err)
		http.Error(w, "failed to create registry", http.StatusInternalServerError)
		return
	}
	defaultChat.Registry = registry

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				if !(strings.Contains(err.Error(), "1005") || strings.Contains(err.Error(), "1006")) { // Ignore these close codes
					log.Println("Unexpected WebSocket error:", err)
				}
			}
			break
		}

		// fmt.Printf("Received message for agent '%s': %s\n", defaultChat.Agent, msg)

		// Optionally echo the message back
		input := string(msg)
		ctx := context.Background()
		// run := NewChatRun(registry, defaultChat)
		resp, _ := registry.Run(ctx, defaultChat.Agent, input)

		if err := conn.WriteMessage(websocket.TextMessage, []byte(resp)); err != nil {
			log.Println("WebSocket write error:", err)
			conn.Close()
			break
		}
	}
}

// ExtractAgentMemory is called after an agent runs to allow post-processing of input/output for memory storage.
// Extracts facts using AI and stores them in chat memory.
func (r *RunContext) ExtractAgentMemory(ctx context.Context, agent *agents.Agent, input, output string) {
	if len(agent.Facts) == 0 {
		return
	}
	c := r.Chat
	if c == nil {
		return
	}

	// Create prompt
	prompt := "Given the following interaction, extract the following facts as YAML:\n\n"
	prompt += "Input:\n" + input + "\n\n"
	prompt += "Output:\n" + output + "\n\n"
	prompt += "Facts to extract:\n"
	for k, arg := range agent.Facts {
		typ := arg.Type
		if typ == "" {
			typ = "string"
		}
		if arg.Description == "" {
			log.Printf("[FACTS] Warning: fact '%s' has no description", k)
		} else {
			// log.Printf("[FACTS] Preparing to extract: %s = %s (type: %s)", k, arg.Description, typ)
		}
		prompt += fmt.Sprintf("%s: %s (type: %s)\n", k, arg.Description, typ)
	}
	prompt += "\nRespond ONLY with a valid YAML block and no explanation or markdown."

	// Use agent description and mock function to call AI
	resp, err := r.CallAI(ctx, &agents.Agent{
		Description: "Extract structured facts from input and output text.",
	}, prompt)
	if err != nil {
		log.Printf("[FACTS] AI call failed: %v", err)
		return
	}

	// Parse YAML into map
	result := make(map[string]any)
	cleanResp := ExtractYAMLFromMarkdown(resp)
	if err := yaml.Unmarshal([]byte(cleanResp), &result); err != nil {
		log.Printf("[FACTS] Failed to parse YAML: %v\nAI Output:\n%s", err, resp)
		return
	}

	// Store each fact and tag, with checks for missing/empty/null
	for k, arg := range agent.Facts {
		key := fmt.Sprintf("%s.%s", agent.Name, k)

		v, ok := result[k]
		if !ok {
			log.Printf("[FACTS] AI did not return value for: %s", key)
			continue
		}
		if v == nil || (fmt.Sprintf("%v", v) == "") {
			log.Printf("[FACTS] Value for %s is empty or null", key)
			continue
		}

		c.Facts[key] = v
		// log.Printf("[FACTS] Stored: %s = %v", key, v)

		if arg.Tags != nil {
			for _, tag := range arg.Tags {
				c.TaggedFacts[tag] = append(c.TaggedFacts[tag], key)
			}
		}
	}
}

// FactsHandler serves the facts and Observations of the current chat session.
func FactsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(map[string]any{
		"facts": defaultChat.Facts,
		// "Observations": defaultChat.Observations,
	})
	if err != nil {
		http.Error(w, "failed to encode facts", http.StatusInternalServerError)
	}
}

// ExtractYAMLFromMarkdown locates a ```yaml ... ``` block and extracts its content.
func ExtractYAMLFromMarkdown(s string) string {
	if strings.HasPrefix(s, "```yaml") {
		start := strings.Index(s, "\n")
		end := strings.LastIndex(s, "```")
		if start != -1 && end != -1 && end > start {
			return s[start+1 : end]
		}
	}
	return s
}
