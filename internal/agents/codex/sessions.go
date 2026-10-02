package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// Session contains metadata only. Discovery never reads turns or resumes a thread.
type Session struct {
	ID                   string `json:"id"`
	Name                 string `json:"name"`
	Cwd                  string `json:"cwd"`
	ParentThreadID       string `json:"parentThreadId"`
	Ephemeral            bool   `json:"ephemeral"`
	CanAcceptDirectInput *bool  `json:"canAcceptDirectInput"`
	Status               struct {
		Type        string   `json:"type"`
		ActiveFlags []string `json:"activeFlags"`
	} `json:"status"`
	Socket string `json:"-"`
}

func (s Session) Loaded() bool { return s.Status.Type != "" && s.Status.Type != "notLoaded" }
func (s Session) Importable() bool {
	return s.ID != "" && filepath.IsAbs(s.Cwd) && !s.Ephemeral && s.ParentThreadID == "" && (s.CanAcceptDirectInput == nil || *s.CanAcceptDirectInput)
}
func (s Session) MotleyStatus() string {
	switch s.Status.Type {
	case "active":
		for _, flag := range s.Status.ActiveFlags {
			if flag == "waitingOnApproval" {
				return "permission"
			}
		}
		for _, flag := range s.Status.ActiveFlags {
			if flag == "waitingOnUserInput" {
				return "question"
			}
		}
		return "working"
	case "idle":
		return "idle"
	default:
		return "alive"
	}
}

var discoveryTimeout = 5 * time.Second

// DefaultSocket asks the installed CLI instead of guessing private storage paths.
func DefaultSocket() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), discoveryTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "codex", "app-server", "daemon", "version").Output()
	if err != nil {
		return "", fmt.Errorf("codex discovery requires app-server daemon version support: %w", err)
	}
	var info struct {
		Status string `json:"status"`
		Socket string `json:"socketPath"`
	}
	if err := json.Unmarshal(out, &info); err != nil {
		return "", fmt.Errorf("invalid Codex daemon information: %w", err)
	}
	if info.Status != "running" || !filepath.IsAbs(info.Socket) {
		return "", fmt.Errorf("no running local Codex app-server; start Codex with its shared daemon (sessions using --no-daemon are not discoverable)")
	}
	return info.Socket, nil
}

type sessionClient struct {
	conn *websocket.Conn
	next int
}

func connectSessions(socket string) (*sessionClient, error) {
	if !filepath.IsAbs(socket) || strings.ContainsAny(socket, "\x00\r\n") {
		return nil, fmt.Errorf("codex socket must be an absolute local path")
	}
	ctx, cancel := context.WithTimeout(context.Background(), discoveryTimeout)
	defer cancel()
	dialer := websocket.Dialer{
		HandshakeTimeout: discoveryTimeout,
		NetDialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socket)
		},
	}
	conn, response, err := dialer.DialContext(ctx, "ws://localhost/", nil)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if err != nil {
		return nil, fmt.Errorf("connect to Codex app-server: %w", err)
	}
	conn.SetReadLimit(2 << 20)
	deadline := time.Now().Add(discoveryTimeout)
	_ = conn.SetReadDeadline(deadline)
	_ = conn.SetWriteDeadline(deadline)
	c := &sessionClient{conn: conn}
	if err := c.call("initialize", map[string]any{"clientInfo": map[string]string{"name": "motley", "title": "Motley", "version": "1"}}, &json.RawMessage{}); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := conn.WriteJSON(map[string]any{"method": "initialized"}); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return c, nil
}
func (c *sessionClient) call(method string, params any, result any) error {
	c.next++
	if err := c.conn.WriteJSON(map[string]any{"id": c.next, "method": method, "params": params}); err != nil {
		return err
	}
	for {
		var reply struct {
			ID     int             `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := c.conn.ReadJSON(&reply); err != nil {
			return fmt.Errorf("codex %s: %w", method, err)
		}
		if reply.ID != c.next {
			continue
		}
		if reply.Error != nil {
			return fmt.Errorf("codex %s (%d): %s", method, reply.Error.Code, reply.Error.Message)
		}
		if len(reply.Result) == 0 {
			return fmt.Errorf("codex %s returned no result", method)
		}
		return json.Unmarshal(reply.Result, result)
	}
}
func (c *sessionClient) read(socket, id string) (Session, error) {
	var reply struct {
		Thread Session `json:"thread"`
	}
	if err := c.call("thread/read", map[string]any{"threadId": id, "includeTurns": false}, &reply); err != nil {
		return Session{}, err
	}
	s := reply.Thread
	if s.ID != id || !s.Importable() || s.Status.Type == "" {
		return Session{}, fmt.Errorf("codex thread %s has unsupported or incomplete metadata", id)
	}
	s.Socket = socket
	return s, nil
}

// Sessions queries only loaded threads on an existing local daemon. No server,
// thread or turn is created, resumed, subscribed to or interrupted.
func Sessions(socket string) ([]Session, error) {
	if socket == "" {
		var err error
		socket, err = DefaultSocket()
		if err != nil {
			return nil, err
		}
	}
	c, err := connectSessions(socket)
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.conn.Close() }()
	var sessions []Session
	cursor := ""
	seen := map[string]bool{}
	for {
		var page struct {
			Data []string `json:"data"`
			Next string   `json:"nextCursor"`
		}
		params := map[string]any{"limit": 100}
		if cursor != "" {
			params["cursor"] = cursor
		}
		if err := c.call("thread/loaded/list", params, &page); err != nil {
			return nil, err
		}
		for _, id := range page.Data {
			// Skip unsupported thread kinds without fetching their transcript.
			var reply struct {
				Thread Session `json:"thread"`
			}
			if err := c.call("thread/read", map[string]any{"threadId": id, "includeTurns": false}, &reply); err != nil {
				return nil, err
			}
			s := reply.Thread
			if s.ID != id || s.Status.Type == "" {
				return nil, fmt.Errorf("codex returned incomplete thread metadata")
			}
			if s.Importable() && s.Loaded() {
				s.Socket = socket
				sessions = append(sessions, s)
			}
		}
		if page.Next == "" {
			break
		}
		if seen[page.Next] {
			return nil, fmt.Errorf("codex session pagination repeated a cursor")
		}
		seen[page.Next] = true
		cursor = page.Next
	}
	return sessions, nil
}

func ReadSession(socket, id string) (Session, error) {
	c, err := connectSessions(socket)
	if err != nil {
		return Session{}, err
	}
	defer func() { _ = c.conn.Close() }()
	return c.read(socket, id)
}
