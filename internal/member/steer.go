package member

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/thomashartm/motley/internal/state"
	"github.com/thomashartm/motley/internal/tmux"
)

// Send rechecks both endpoints at action time and protects the monitor client.
func Send(id, client string) error {
	if strings.TrimSpace(client) == "" {
		return fmt.Errorf("choose a tab from motley tabs")
	}
	if err := RequireLive(id); err != nil {
		return err
	}
	rows, err := List()
	if err != nil {
		return err
	}
	for _, r := range rows {
		if r.ID == id && r.External {
			return fmt.Errorf("session runs in its original terminal; reply there")
		}
	}
	clients, err := tmux.Clients()
	if err != nil {
		return err
	}
	for _, c := range clients {
		if c.TTY == client || c.Name == client {
			if c.Session == tmux.MonitorSession {
				return fmt.Errorf("choose a work tab; the monitor must stay in place")
			}
			return tmux.SwitchClient(c.TTY, id)
		}
	}
	return fmt.Errorf("tab %q is no longer attached", client)
}
func Reply(id, text string) error {
	if strings.TrimSpace(text) == "" || strings.IndexFunc(text, unicode.IsControl) >= 0 {
		return fmt.Errorf("reply must be a non-empty single line without control characters")
	}
	if err := CheckID(id); err != nil {
		return err
	}
	sessions, err := tmux.Sessions()
	if err != nil {
		return err
	}
	for _, s := range sessions {
		if s.MemberID == id && s.Name == tmux.SessionName(id) {
			if s.Status == "permission" {
				return fmt.Errorf("permission needs a decision in the agent; jump to the member")
			}
			if s.Status == "ended" {
				return fmt.Errorf("agent has ended; jump to its shell instead")
			}
			dir, err := state.MembersDir()
			if err != nil {
				return err
			}
			m, err := Load(dir, id)
			if err != nil {
				return err
			}
			if m.CodexSession != "" {
				thread, err := validatedCodexSession(m)
				if err != nil {
					return err
				}
				if !thread.Loaded() {
					return fmt.Errorf("codex session is no longer loaded; open the member first")
				}
				if thread.MotleyStatus() == "permission" {
					return fmt.Errorf("permission needs a decision in the agent; jump to the member")
				}
			}
			return tmux.SendText(id, text)
		}
	}
	return fmt.Errorf("member %s has no managed terminal; open its original terminal to reply", id)
}
