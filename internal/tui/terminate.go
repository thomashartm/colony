package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/thomashartm/motley/internal/member"
)

type terminateDialog struct {
	id       string
	confirm  bool
	external bool
}

func (m Model) beginTerminate() (tea.Model, tea.Cmd) {
	id := m.selectedID()
	if id == "" {
		return m, nil
	}
	if !m.selectedRow().Alive {
		m.message = "This agent session is already stopped. Use Revive to restart it."
		return m, nil
	}
	m.terminating = &terminateDialog{id: id, external: m.selectedRow().External}
	m.message = ""
	return m, nil
}

func (m Model) updateTerminate(key string) (tea.Model, tea.Cmd) {
	d := *m.terminating
	m.terminating = &d
	switch key {
	case "up", "down", "left", "right", "tab", "shift+tab":
		d.confirm = !d.confirm
		return m, nil
	case "enter":
		key = "esc"
		if d.confirm {
			key = "y"
		}
	}
	switch key {
	case "esc", "q", "n":
		m.terminating = nil
	case "y":
		m.busy = true
		m.busyText = "Terminating…"
		return m, func() tea.Msg { return lifecycleDone{id: d.id, action: "Terminated", err: member.Terminate(d.id)} }
	}
	return m, nil
}

func (m Model) terminateButtons() string {
	if m.terminating.confirm {
		return "  [Cancel: esc] > [Terminate: y]"
	}
	return "> [Cancel: esc]   [Terminate: y]"
}

func (m Model) terminateView(height int) string {
	action := "Stops all processes in this agent session."
	if m.terminating.external {
		action = "Stops Claude in its original terminal."
	}
	text := "Terminate " + clean(m.terminating.id) + "?\n" + action + "\nKeeps worktree, branch and history. Use Revive to restart."
	lines := strings.Split(ansi.Hardwrap(text, m.detailWidth(), true), "\n")
	return strings.Join(lines[:min(len(lines), height)], "\n")
}
