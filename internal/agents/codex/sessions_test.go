package codex

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func sessionServer(t *testing.T, handler func(string, json.RawMessage) (any, any)) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "motley-codex-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socket := filepath.Join(dir, "rpc.sock")
	ln, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{ReadHeaderTimeout: time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		initialized := false
		for {
			var req struct {
				ID     int             `json:"id"`
				Method string          `json:"method"`
				Params json.RawMessage `json:"params"`
			}
			if conn.ReadJSON(&req) != nil {
				return
			}
			if req.Method == "initialize" {
				initialized = true
				_ = conn.WriteJSON(map[string]any{"id": req.ID, "result": map[string]any{}})
				continue
			}
			if req.Method == "initialized" {
				continue
			}
			if !initialized {
				t.Error("missing handshake")
				return
			}
			result, rpcError := handler(req.Method, req.Params)
			response := map[string]any{"id": req.ID}
			if rpcError != nil {
				response["error"] = rpcError
			} else {
				response["result"] = result
			}
			if conn.WriteJSON(response) != nil {
				return
			}
		}
	})}
	go func() { _ = server.Serve(ln) }()
	t.Cleanup(func() { _ = server.Close() })
	return socket
}

func TestSessionsReadOnlyPaginationAndFiltering(t *testing.T) {
	var methods []string
	socket := sessionServer(t, func(method string, params json.RawMessage) (any, any) {
		methods = append(methods, method)
		var p map[string]any
		_ = json.Unmarshal(params, &p)
		switch method {
		case "thread/loaded/list":
			if p["cursor"] == "next" {
				return map[string]any{"data": []string{"child", "ephemeral", "closed", "unavailable"}}, nil
			}
			return map[string]any{"data": []string{"one"}, "nextCursor": "next"}, nil
		case "thread/read":
			if p["includeTurns"] != false {
				t.Error("discovery requested conversation content")
			}
			id := p["threadId"].(string)
			thread := map[string]any{"id": id, "cwd": "/repo", "name": "Existing work", "status": map[string]any{"type": "active", "activeFlags": []string{"waitingOnApproval"}}}
			switch id {
			case "child":
				thread["parentThreadId"] = "root"
			case "ephemeral":
				thread["ephemeral"] = true
			case "closed":
				thread["status"] = map[string]any{"type": "notLoaded"}
			case "unavailable":
				thread["canAcceptDirectInput"] = false
			}
			return map[string]any{"thread": thread}, nil
		default:
			t.Errorf("discovery used mutating or unexpected method %s", method)
			return nil, map[string]any{"code": -1, "message": "unexpected"}
		}
	})
	sessions, err := Sessions(socket)
	if err != nil || len(sessions) != 1 {
		t.Fatal(sessions, err)
	}
	if sessions[0].ID != "one" || sessions[0].Socket != socket || sessions[0].MotleyStatus() != "permission" {
		t.Fatal(sessions)
	}
	if len(methods) != 7 {
		t.Fatal(methods)
	}
}
func TestSessionsRejectErrorsAndRepeatedPagination(t *testing.T) {
	for _, kind := range []string{"error", "cursor", "identity"} {
		t.Run(kind, func(t *testing.T) {
			socket := sessionServer(t, func(method string, _ json.RawMessage) (any, any) {
				if kind == "error" {
					return nil, map[string]any{"code": -32601, "message": "unsupported interface"}
				}
				if method == "thread/loaded/list" {
					if kind == "cursor" {
						return map[string]any{"data": []string{}, "nextCursor": "same"}, nil
					}
					return map[string]any{"data": []string{"one"}}, nil
				}
				return map[string]any{"thread": map[string]any{"id": "other", "cwd": "/repo", "status": map[string]any{"type": "idle"}}}, nil
			})
			if _, err := Sessions(socket); err == nil {
				t.Fatal("invalid discovery succeeded")
			}
		})
	}
}
func TestSessionStatus(t *testing.T) {
	for _, tc := range []struct {
		state string
		flags []string
		want  string
	}{{"idle", nil, "idle"}, {"active", nil, "working"}, {"active", []string{"waitingOnUserInput"}, "question"}, {"active", []string{"waitingOnUserInput", "waitingOnApproval"}, "permission"}, {"systemError", nil, "alive"}} {
		s := Session{}
		s.Status.Type = tc.state
		s.Status.ActiveFlags = tc.flags
		if s.MotleyStatus() != tc.want {
			t.Fatal(tc)
		}
	}
}
func TestSessionDeadlineAndMissingSocket(t *testing.T) {
	old := discoveryTimeout
	discoveryTimeout = 50 * time.Millisecond
	t.Cleanup(func() { discoveryTimeout = old })
	socket := sessionServer(t, func(_ string, _ json.RawMessage) (any, any) {
		time.Sleep(100 * time.Millisecond)
		return map[string]any{}, nil
	})
	if _, err := Sessions(socket); err == nil {
		t.Fatal("discovery did not time out")
	}
	if _, err := ReadSession("relative", "one"); err == nil {
		t.Fatal("accepted relative socket")
	}
}
func TestReadSessionPreservesUnloadedIdentity(t *testing.T) {
	socket := sessionServer(t, func(method string, params json.RawMessage) (any, any) {
		if method != "thread/read" {
			t.Error(method)
		}
		var p map[string]any
		_ = json.Unmarshal(params, &p)
		if !reflect.DeepEqual(p, map[string]any{"threadId": "one", "includeTurns": false}) {
			t.Error(p)
		}
		return map[string]any{"thread": map[string]any{"id": "one", "cwd": "/repo", "status": map[string]any{"type": "notLoaded"}}}, nil
	})
	s, err := ReadSession(socket, "one")
	if err != nil || s.Loaded() || s.ID != "one" {
		t.Fatal(s, err)
	}
}
