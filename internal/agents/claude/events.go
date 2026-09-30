// Package claude maps Claude Code's hook payloads to colony attention states.
package claude

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/thomashartm/colony/internal/state"
)

type payload struct {
	Event            string          `json:"hook_event_name"`
	SessionID        string          `json:"session_id"`
	Prompt           string          `json:"prompt"`
	Tool             string          `json:"tool_name"`
	Input            json.RawMessage `json:"tool_input"`
	NotificationType string          `json:"notification_type"`
	Message          string          `json:"message"`
	LastAssistant    string          `json:"last_assistant_message"`
	Transcript       string          `json:"transcript_path"`
}

func Parse(data []byte, eventName string) (state.Event, error) {
	var p payload
	if err := json.Unmarshal(data, &p); err != nil {
		return state.Event{}, fmt.Errorf("claude hook payload: %w", err)
	}
	if eventName != "" {
		p.Event = eventName
	}
	e := state.Event{Agent: "claude", Event: p.Event, AgentSessionID: p.SessionID, Detail: map[string]string{}}
	switch p.Event {
	case "SessionStart":
		e.Status = "idle"
	case "UserPromptSubmit":
		e.Status = "working"
		e.Summary = strings.SplitN(p.Prompt, "\n", 2)[0]
	case "PreToolUse", "PostToolUse":
		e.Status = "working"
		var input map[string]json.RawMessage
		_ = json.Unmarshal(p.Input, &input)
		var compact bytes.Buffer
		_ = json.Compact(&compact, p.Input)
		text := compact.String()
		for _, key := range []string{"command", "file_path", "path", "pattern"} {
			var value string
			if json.Unmarshal(input[key], &value) == nil && value != "" {
				text = value
				break
			}
		}
		e.Summary = p.Tool + ": " + text
		e.Detail["tool"] = p.Tool
		e.Detail["input"] = text
		if p.Event == "PreToolUse" && p.Tool == "AskUserQuestion" {
			e.Status = "question"
			var in struct {
				Questions []struct {
					Question string `json:"question"`
					Options  []struct {
						Label       string `json:"label"`
						Description string `json:"description"`
					} `json:"options"`
				} `json:"questions"`
			}
			if err := json.Unmarshal(p.Input, &in); err != nil {
				return state.Event{}, fmt.Errorf("question payload: %w", err)
			}
			var lines []string
			for _, q := range in.Questions {
				lines = append(lines, q.Question)
				for i, o := range q.Options {
					lines = append(lines, fmt.Sprintf("  %d) %s — %s", i+1, o.Label, o.Description))
				}
			}
			e.Summary = strings.Join(lines, "\n")
			e.Detail = map[string]string{"question": e.Summary}
		}
	case "Notification":
		e.Summary = p.Message
		e.Detail["message"] = p.Message
		switch p.NotificationType {
		case "permission_prompt":
			e.Status = "permission"
		case "idle_prompt":
			e.Status = "idle"
		case "":
			text := strings.ToLower(p.Message)
			if strings.Contains(text, "permission") || strings.Contains(text, "approval") {
				e.Status = "permission"
			} else if strings.Contains(text, "waiting") || strings.Contains(text, "input") || strings.Contains(text, "idle") {
				e.Status = "idle"
			}
		}
	case "Stop":
		e.Status = "ready"
		e.Summary = p.LastAssistant
		if e.Summary == "" && p.Transcript != "" {
			var err error
			e.Summary, err = state.TranscriptMessage(p.Transcript)
			if err != nil {
				return e, err
			}
		}
	case "SessionEnd":
		e.Status = "ended"
	case "":
		return e, fmt.Errorf("claude hook payload has no hook_event_name")
	}
	return e, nil
}
