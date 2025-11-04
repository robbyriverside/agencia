package agencia

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var ErrChatNotConnected = errors.New("chat client not connected")

const defaultChatInactivityTimeout = 30 * time.Minute

var chatInactivityTimeout = defaultChatInactivityTimeout

// ChatClient manages a websocket chat session and handles reconnect/closure semantics.
type ChatClient struct {
	WSURL    string
	HTTPBase string
	Agent    string
	Spec     string
	ChatID   string

	httpClient *http.Client
	conn       *websocket.Conn

	activityMu     sync.Mutex
	lastActivity   time.Time
	inactivityCh   chan struct{}
	inactivityStop context.CancelFunc
}

// Connect starts or resumes a chat session. If ChatID is already populated it will
// be reused to resume the existing session.
func (c *ChatClient) Connect(ctx context.Context) error {
	if c.Agent == "" {
		return errors.New("chat client requires agent")
	}
	if c.WSURL == "" {
		return errors.New("chat client requires websocket URL")
	}

	if c.httpClient == nil {
		c.httpClient = http.DefaultClient
	}

	dialer := websocket.Dialer{}
	conn, _, err := dialer.DialContext(ctx, c.WSURL, nil)
	if err != nil {
		return err
	}

	initPayload := map[string]string{
		"agent": c.Agent,
	}
	if c.ChatID != "" {
		initPayload["chat_id"] = c.ChatID
	} else {
		initPayload["spec"] = c.Spec
	}

	if err := conn.WriteJSON(initPayload); err != nil {
		_ = conn.Close()
		return err
	}

	chatID, err := awaitChatInit(ctx, conn)
	if err != nil {
		_ = conn.Close()
		return err
	}

	c.ChatID = chatID
	c.conn = conn
	c.touchActivity()
	c.startInactivityWatcher()
	return nil
}

func awaitChatInit(ctx context.Context, conn *websocket.Conn) (string, error) {
	type result struct {
		chatID string
		err    error
	}
	ch := make(chan result, 1)

	go func() {
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				ch <- result{"", err}
				return
			}
			var payload map[string]any
			if err := json.Unmarshal(data, &payload); err != nil {
				continue
			}
			if typ, _ := payload["type"].(string); typ == "chat_init" {
				if id, _ := payload["chat_id"].(string); id != "" {
					ch <- result{id, nil}
					return
				}
				ch <- result{"", errors.New("chat init missing chat_id")}
				return
			}
		}
	}()

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case res := <-ch:
		return res.chatID, res.err
	case <-time.After(10 * time.Second):
		return "", errors.New("timeout waiting for chat init")
	}
}

// Send transmits a user message over the active websocket connection.
func (c *ChatClient) Send(message string) error {
	if c.conn == nil {
		return ErrChatNotConnected
	}
	payload := map[string]any{
		"message": message,
	}
	if err := c.conn.WriteJSON(payload); err != nil {
		return err
	}
	c.touchActivity()
	return nil
}

// Receive blocks until a message arrives from the websocket connection.
func (c *ChatClient) Receive(ctx context.Context) (map[string]any, error) {
	if c.conn == nil {
		return nil, ErrChatNotConnected
	}

	type result struct {
		data map[string]any
		err  error
	}
	ch := make(chan result, 1)

	go func() {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			ch <- result{nil, err}
			return
		}
		var payload map[string]any
		if err := json.Unmarshal(data, &payload); err != nil {
			ch <- result{nil, err}
			return
		}
		ch <- result{payload, nil}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-ch:
		if res.err == nil {
			c.touchActivity()
		}
		return res.data, res.err
	}
}

// Disconnect closes the websocket connection without closing the chat session.
func (c *ChatClient) Disconnect() error {
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}

// CloseChat invokes the REST API to fully close the chat session and clears the stored chat ID.
func (c *ChatClient) CloseChat(ctx context.Context) error {
	if c.ChatID == "" {
		return nil
	}

	c.stopInactivityWatcher()
	_ = c.Disconnect()

	base := strings.TrimRight(c.HTTPBase, "/")
	if base == "" {
		return errors.New("chat client requires HTTP base URL")
	}

	endpoint := fmt.Sprintf("%s/api/closechat?chat_id=%s", base, url.QueryEscape(c.ChatID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("unexpected status closing chat: %d", resp.StatusCode)
	}

	c.ChatID = ""
	return nil
}

func (c *ChatClient) touchActivity() {
	c.activityMu.Lock()
	c.lastActivity = time.Now()
	ch := c.inactivityCh
	c.activityMu.Unlock()

	if ch != nil {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (c *ChatClient) startInactivityWatcher() {
	c.activityMu.Lock()
	if c.inactivityStop != nil {
		c.inactivityStop()
	}
	if c.lastActivity.IsZero() {
		c.lastActivity = time.Now()
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.inactivityStop = cancel
	ch := make(chan struct{}, 1)
	c.inactivityCh = ch
	c.activityMu.Unlock()

	if chatInactivityTimeout <= 0 {
		return
	}

	go c.watchInactivity(ctx, ch)
}

func (c *ChatClient) stopInactivityWatcher() {
	c.activityMu.Lock()
	if c.inactivityStop != nil {
		c.inactivityStop()
		c.inactivityStop = nil
	}
	c.inactivityCh = nil
	c.activityMu.Unlock()
}

func (c *ChatClient) watchInactivity(ctx context.Context, activityCh <-chan struct{}) {
	timer := time.NewTimer(chatInactivityTimeout)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-activityCh:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(chatInactivityTimeout)
		case <-timer.C:
			c.activityMu.Lock()
			since := time.Since(c.lastActivity)
			c.activityMu.Unlock()
			if since >= chatInactivityTimeout {
				if err := c.CloseChat(context.Background()); err != nil {
					return
				}
			}
			return
		}
	}
}
