package agencia

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/robbyriverside/agencia/agents"
	"gopkg.in/yaml.v3"
)

type chatSessionStore struct {
	mu        sync.RWMutex
	sessions  map[string]*Chat
	connIndex map[*websocket.Conn]*Chat
}

func newChatSessionStore() *chatSessionStore {
	return &chatSessionStore{
		sessions:  make(map[string]*Chat),
		connIndex: make(map[*websocket.Conn]*Chat),
	}
}

func (s *chatSessionStore) startSession(conn *websocket.Conn, agent string, registry *Registry) *Chat {
	s.mu.Lock()
	defer s.mu.Unlock()

	if chat, ok := s.connIndex[conn]; ok && chat != nil {
		s.sessions[chat.ChatID] = chat
		chat.SetStartAgent(agent)
		chat.Registry = registry
		chat.Conn = conn
		chat.Closed = false
		return chat
	}

	chat := NewChat(agent, registry)
	chat.Conn = conn
	s.sessions[chat.ChatID] = chat
	s.connIndex[conn] = chat
	return chat
}

func (s *chatSessionStore) resumeSession(chatID string, conn *websocket.Conn) (*Chat, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	chat, ok := s.sessions[chatID]
	if !ok || chat == nil {
		return nil, fmt.Errorf("chat not found")
	}
	if chat.Closed {
		return nil, fmt.Errorf("chat is closed")
	}
	if chat.Conn != nil {
		delete(s.connIndex, chat.Conn)
	}
	chat.Conn = conn
	s.connIndex[conn] = chat
	return chat, nil
}

func (s *chatSessionStore) endSession(conn *websocket.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if chat, ok := s.connIndex[conn]; ok {
		delete(s.connIndex, conn)
		if chat != nil {
			if chat.Conn == conn {
				chat.Conn = nil
			}
			if chat.Closed && chat.ChatID != "" {
				delete(s.sessions, chat.ChatID)
			}
		}
	}
}

func (s *chatSessionStore) closeChat(chatID string) (*Chat, error) {
	s.mu.Lock()
	chat, ok := s.sessions[chatID]
	if !ok || chat == nil {
		s.mu.Unlock()
		return nil, fmt.Errorf("chat not found")
	}
	chat.Closed = true
	conn := chat.Conn
	if conn != nil {
		delete(s.connIndex, conn)
	}
	delete(s.sessions, chatID)
	chat.Conn = nil
	s.mu.Unlock()
	if conn != nil && conn.UnderlyingConn() != nil {
		_ = conn.Close()
	}
	return chat, nil
}

func (s *chatSessionStore) get(chatID string) (*Chat, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	chat, ok := s.sessions[chatID]
	return chat, ok
}

var chatSessions = newChatSessionStore()

type Chat struct {
	*Registry
	ChatID       string
	StartAgent   string
	Facts        map[string]any
	Observations map[string]map[string][]string
	Cards        []*TraceCard
	Conn         *websocket.Conn
	Closed       bool
}

func (c *Chat) SetStartAgent(name string) {
	c.StartAgent = name
}

func (c *Chat) IsValidStartAgent(name string) bool {
	if c.Registry == nil {
		return false
	}
	_, ok := c.Registry.Agents[name]
	return ok
}

func NewChat(agent string, registry *Registry) *Chat {
	return &Chat{
		Registry:     registry,
		ChatID:       uuid.NewString(),
		StartAgent:   agent,
		Facts:        make(map[string]any),
		Observations: make(map[string]map[string][]string),
	}
}

func (c *Chat) Fact(name string) any {
	if c == nil {
		return nil
	}
	if v, ok := c.Facts[name]; ok {
		return v
	}
	// Fallback: check if the requested fact is global but asked for with agent prefix
	if strings.Contains(name, ".") && c.Registry != nil {
		parts := strings.SplitN(name, ".", 2)
		agentName := parts[0]
		factName := parts[1]
		if agent, err := c.Registry.LookupAgent(agentName); err == nil {
			if factDef, ok := agent.Facts[factName]; ok {
				// Default scope is global
				if factDef.Scope == "global" || factDef.Scope == "" {
					if v, ok := c.Facts[factName]; ok {
						return v
					}
				}
			}
		}
	}
	return nil
}

// LoadFacts replaces the chat's facts map with the provided one.
func (c *Chat) LoadFacts(facts map[string]any) {
	if c == nil {
		return
	}
	c.Facts = facts
}

// AddObservation stores an observation for the given role and key.
// Observations are unstructured pieces of knowledge gathered during chat.
func (c *Chat) AddObservation(role, key, observation string) {
	if c == nil {
		return
	}
	if c.Observations == nil {
		c.Observations = make(map[string]map[string][]string)
	}
	if _, ok := c.Observations[role]; !ok {
		c.Observations[role] = make(map[string][]string)
	}
	c.Observations[role][key] = append(c.Observations[role][key], observation)
}

// ObservationsByRole returns the observations for the specified role.
func (c *Chat) ObservationsByRole(role string) map[string][]string {
	if c == nil {
		return nil
	}
	return c.Observations[role]
}

func (c *Chat) SendMessageFromAgent(agentName, context, msg string) error {
	if c == nil || c.Conn == nil {
		return fmt.Errorf("no websocket connection")
	}
	response := map[string]string{
		"id":      "", // jobs can override this
		"sender":  agentName,
		"context": context,
		"message": msg,
	}
	data, err := json.Marshal(response)
	if err != nil {
		return err
	}
	return c.Conn.WriteMessage(websocket.TextMessage, data)
}

func ChatSessionHandler(w http.ResponseWriter, r *http.Request) {
	type ChatInitRequest struct {
		Agent  string `json:"agent"`
		Spec   string `json:"spec"` // optionally store or use this
		ChatID string `json:"chat_id"`
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

	var (
		chat     *Chat
		registry *Registry
	)

	if chatID := strings.TrimSpace(initReq.ChatID); chatID != "" {
		var resumeErr error
		chat, resumeErr = chatSessions.resumeSession(chatID, conn)
		if resumeErr != nil {
			log.Printf("Failed to resume chat %s: %v", chatID, resumeErr)
			_ = conn.WriteJSON(map[string]any{
				"type":  "error",
				"error": "chat not found or closed",
			})
			return
		}
		if initReq.Agent != "" {
			chat.SetStartAgent(initReq.Agent)
		}
		registry = chat.Registry
		if registry == nil {
			log.Printf("Chat %s has no registry", chatID)
			_ = conn.WriteJSON(map[string]any{
				"type":  "error",
				"error": "chat has no registry",
			})
			return
		}
	} else {
		var regErr error
		registry, regErr = NewRegistry(initReq.Spec)
		if regErr != nil {
			log.Println("Failed to create registry:", regErr)
			http.Error(w, "failed to create registry", http.StatusInternalServerError)
			return
		}
		chat = chatSessions.startSession(conn, initReq.Agent, registry)
	}

	defer chatSessions.endSession(conn)
	registry = chat.Registry

	if err := conn.WriteJSON(map[string]any{
		"type":    "chat_init",
		"chat_id": chat.ChatID,
	}); err != nil {
		log.Printf("Failed to send chat init message: %v", err)
		return
	}

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

		// Optionally echo the message back
		input := string(msg)
		ctx := context.Background()
		respondingAgent := chat.StartAgent
		resp, _ := registry.Run(ctx, chat, chat.StartAgent, input)

		chat.SendMessageFromAgent(respondingAgent, "update", resp)
	}
}

// ExtractAgentMemory is called after an agent runs to allow post-processing of input/output for memory storage.
// Extracts facts using AI and stores them in chat memory.
func (r *RunContext) ExtractAgentMemory(ctx context.Context, agent *agents.Agent, input, output string) {
	if r == nil || len(agent.Facts) == 0 {
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
	for k := range agent.Facts {
		key := k
		// Determine scope
		scope := "global"
		if f, ok := agent.Facts[k]; ok && f.Scope == "local" {
			scope = "local"
		}
		if scope == "local" {
			key = fmt.Sprintf("%s.%s", agent.Name, k)
		}

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
	}
}

// FactsHandler serves the facts and Observations of the current chat session.
func FactsHandler(w http.ResponseWriter, r *http.Request) {
	chatID := r.URL.Query().Get("chat_id")
	if strings.TrimSpace(chatID) == "" {
		http.Error(w, "chat_id is required", http.StatusBadRequest)
		return
	}

	chat, ok := chatSessions.get(chatID)
	if !ok || chat == nil {
		http.Error(w, "chat not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(map[string]any{
		"facts":        chat.Facts,
		"observations": chat.Observations,
	})
	if err != nil {
		http.Error(w, "failed to encode facts", http.StatusInternalServerError)
	}
}

// CloseChatHandler marks the chat as closed and removes it from the session store.
func CloseChatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	chatID := r.URL.Query().Get("chat_id")
	if strings.TrimSpace(chatID) == "" {
		http.Error(w, "chat_id is required", http.StatusBadRequest)
		return
	}

	if _, err := chatSessions.closeChat(chatID); err != nil {
		http.Error(w, "chat not found", http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// LoadFactsHandler replaces the chat facts with the provided map.
func LoadFactsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	chatID := r.URL.Query().Get("chat_id")
	if strings.TrimSpace(chatID) == "" {
		http.Error(w, "chat_id is required", http.StatusBadRequest)
		return
	}

	chat, ok := chatSessions.get(chatID)
	if !ok || chat == nil {
		http.Error(w, "chat not found", http.StatusNotFound)
		return
	}
	var payload struct {
		Facts map[string]any `json:"facts"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	chat.LoadFacts(payload.Facts)
	w.WriteHeader(http.StatusOK)
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
