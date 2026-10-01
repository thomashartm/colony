// Package opencode maps plugin events to member status.
package opencode

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/thomashartm/motley/internal/state"
)

func Parse(data []byte) (state.Event, error) {
	var p struct {
		Type       string `json:"type"`
		Properties struct {
			SessionID string `json:"sessionID"`
			Info      struct {
				ID       string `json:"id"`
				ParentID string `json:"parentID"`
			} `json:"info"`
			Status struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"status"`
			Permission string          `json:"permission"`
			Patterns   []string        `json:"patterns"`
			Metadata   json.RawMessage `json:"metadata"`
			Questions  []struct {
				Question string `json:"question"`
				Options  []struct {
					Label       string `json:"label"`
					Description string `json:"description"`
				} `json:"options"`
			} `json:"questions"`
			Error         json.RawMessage `json:"error"`
			Prompt        string          `json:"prompt"`
			LastAssistant string          `json:"lastAssistantMessage"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(data, &p); err != nil {
		return state.Event{}, fmt.Errorf("opencode event: %w", err)
	}
	x := p.Properties
	e := state.Event{Agent: "opencode", Event: p.Type, AgentSessionID: x.SessionID, Detail: map[string]string{}}
	switch p.Type {
	case "session.created":
		if x.Info.ParentID != "" {
			return e, nil
		}
		e.Event, e.Status, e.AgentSessionID = "SessionStart", "idle", x.Info.ID
	case "chat.message":
		e.Status, e.Summary = "working", strings.SplitN(x.Prompt, "\n", 2)[0]
	case "session.status":
		switch x.Status.Type {
		case "busy", "retry":
			e.Status = "working"
			e.Summary = x.Status.Message
		case "idle":
			e.Event, e.Status, e.Summary = "Stop", "ready", x.LastAssistant
		}
	case "session.idle":
		e.Event, e.Status, e.Summary = "Stop", "ready", x.LastAssistant
	case "permission.asked":
		e.Event, e.Status = "Notification", "permission"
		e.Summary = x.Permission + ": " + strings.Join(x.Patterns, ", ")
		e.Detail["tool"], e.Detail["input"] = x.Permission, strings.Join(x.Patterns, "\n")
		e.Detail["metadata"] = string(x.Metadata)
	case "question.asked":
		e.Event, e.Status = "Notification", "question"
		var lines []string
		for _, q := range x.Questions {
			lines = append(lines, q.Question)
			for i, o := range q.Options {
				lines = append(lines, fmt.Sprintf("  %d) %s — %s", i+1, o.Label, o.Description))
			}
		}
		e.Summary = strings.Join(lines, "\n")
		e.Detail["question"] = e.Summary
	case "permission.replied", "question.replied", "question.rejected":
		e.Status = "working"
	case "session.error":
		e.Event, e.Status, e.Summary = "Notification", "idle", string(x.Error)
	case "session.deleted":
		e.Event, e.Status, e.AgentSessionID = "SessionEnd", "ended", x.Info.ID
	case "":
		return e, fmt.Errorf("opencode payload has no type")
	}
	if e.Status != "" && e.AgentSessionID == "" {
		return state.Event{}, fmt.Errorf("opencode %s has no session id", p.Type)
	}
	return e, nil
}
