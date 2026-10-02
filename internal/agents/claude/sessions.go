package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"time"
)

// Session is the supported `claude agents --json` record. We deliberately do
// not read private transcripts, credentials or daemon socket protocols.
type Session struct {
	ID         string `json:"id"`
	SessionID  string `json:"sessionId"`
	PID        int    `json:"pid"`
	Cwd        string `json:"cwd"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	State      string `json:"state"`
	WaitingFor string `json:"waitingFor"`
	StartedAt  int64  `json:"startedAt"`
}

func Sessions() ([]Session, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "claude", "agents", "--json").Output()
	if err != nil {
		return nil, fmt.Errorf("cannot discover Claude sessions: update Claude Code to a version supporting claude agents --json: %w", err)
	}
	var sessions []Session
	if err := json.Unmarshal(out, &sessions); err != nil {
		return nil, fmt.Errorf("invalid Claude session list: %w", err)
	}
	var valid []Session
	for _, s := range sessions {
		if s.SessionID != "" && filepath.IsAbs(s.Cwd) && (s.Kind == "interactive" || s.Kind == "background") {
			valid = append(valid, s)
		}
	}
	return valid, nil
}

func (s Session) MotleyStatus() string {
	switch s.Status {
	case "busy":
		return "working"
	case "waiting":
		if s.WaitingFor == "permission prompt" || s.WaitingFor == "sandbox request" {
			return "permission"
		}
		return "question"
	case "idle":
		return "idle"
	}
	return "alive"
}
