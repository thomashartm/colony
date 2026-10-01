// Package codex maps native Codex lifecycle hooks to member status.
package codex

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/thomashartm/motley/internal/state"
)

func Parse(data []byte, eventName string) (state.Event, error) {
	var p struct {
		Event         string          `json:"hook_event_name"`
		SessionID     string          `json:"session_id"`
		Source        string          `json:"source"`
		Prompt        string          `json:"prompt"`
		Tool          string          `json:"tool_name"`
		Input         json.RawMessage `json:"tool_input"`
		LastAssistant string          `json:"last_assistant_message"`
	}
	if err := json.Unmarshal(data, &p); err != nil {
		return state.Event{}, fmt.Errorf("codex hook: %w", err)
	}
	if eventName != "" {
		p.Event = eventName
	}
	e := state.Event{Agent: "codex", Event: p.Event, AgentSessionID: p.SessionID, Detail: map[string]string{}}
	switch p.Event {
	case "SessionStart":
		e.Status = "idle"
		if p.Source == "compact" {
			e.Status = "working"
		}
	case "UserPromptSubmit":
		e.Status = "working"
		e.Summary = strings.SplitN(p.Prompt, "\n", 2)[0]
	case "PreToolUse", "PostToolUse", "PermissionRequest":
		e.Status = "working"
		input := string(p.Input)
		var obj struct {
			Command string `json:"command"`
		}
		_ = json.Unmarshal(p.Input, &obj)
		if obj.Command != "" {
			input = obj.Command
		}
		e.Detail["tool"], e.Detail["input"] = p.Tool, input
		e.Summary = p.Tool + ": " + input
		if p.Event == "PermissionRequest" {
			e.Status = "permission"
		}
		if p.Event == "PreToolUse" && (p.Tool == "request_user_input" || p.Tool == "functions.request_user_input" || p.Tool == "request_user_input_async") {
			e.Status = "question"
			var questions struct {
				Questions []struct {
					Question string            `json:"question"`
					Title    string            `json:"title"`
					Options  []json.RawMessage `json:"options"`
				} `json:"questions"`
			}
			if err := json.Unmarshal(p.Input, &questions); err != nil {
				return state.Event{}, err
			}
			var lines []string
			for _, q := range questions.Questions {
				text := q.Question
				if text == "" {
					text = q.Title
				}
				lines = append(lines, text)
				for i, option := range q.Options {
					var label string
					if json.Unmarshal(option, &label) != nil {
						var o struct {
							Label string `json:"label"`
						}
						_ = json.Unmarshal(option, &o)
						label = o.Label
					}
					lines = append(lines, fmt.Sprintf("  %d) %s", i+1, label))
				}
			}
			e.Summary = strings.Join(lines, "\n")
			e.Detail["question"] = e.Summary
		}
	case "Stop":
		e.Status, e.Summary = "ready", p.LastAssistant
	case "Interrupt":
		e.Status, e.Summary = "idle", "Turn interrupted"
	case "SessionEnd":
		e.Status = "ended"
	case "":
		return e, fmt.Errorf("codex hook has no hook_event_name")
	}
	return e, nil
}
