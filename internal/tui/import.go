package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/thomashartm/motley/internal/member"
)

type importDialog struct {
	agent    string
	sessions []member.ImportCandidate
	cursor   int
}
type importLoaded struct {
	agent    string
	sessions []member.ImportCandidate
	err      error
}
type importDone struct {
	member member.Manifest
	err    error
}

func (m Model) beginImport() (tea.Model, tea.Cmd) {
	m.menu = &menuDialog{title: "Add existing agent", items: []menuItem{
		{label: "Claude", run: func(m Model) (tea.Model, tea.Cmd) { return m.beginImportAgent("claude") }},
		{label: "Codex", run: func(m Model) (tea.Model, tea.Cmd) { return m.beginImportAgent("codex") }},
	}}
	m.message = ""
	return m, nil
}
func (m Model) beginImportAgent(agent string) (tea.Model, tea.Cmd) {
	m.importing = &importDialog{agent: agent}
	m.busy, m.busyText = true, "Finding "+importAgentLabel(agent)+" sessions…"
	return m, func() tea.Msg {
		s, err := member.DiscoverImports(agent)
		return importLoaded{agent: agent, sessions: s, err: err}
	}
}
func (m Model) importMessage(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.busy, m.busyText = false, ""
	switch msg := msg.(type) {
	case importLoaded:
		if msg.err != nil {
			m.importing = nil
			m.message = msg.err.Error()
			return m, nil
		}
		m.importing = &importDialog{agent: msg.agent, sessions: msg.sessions}
	case importDone:
		if msg.err != nil {
			m.message = msg.err.Error()
			return m, nil
		}
		m.importing = nil
		m.message = "Added " + msg.member.Name + "; Claude is still running in its original terminal."
		if msg.member.CodexSession != "" {
			m.message = "Added " + msg.member.Name + "; Open agent connects to its existing Codex conversation."
		}
		m.focusID = msg.member.ID
		m.group = "attention"
		m.panel = listPanel
		m.tableFocus = false
		m.query.SetValue("")
		return m, m.poll
	}
	return m, nil
}
func (m Model) updateImport(key string) (tea.Model, tea.Cmd) {
	d := *m.importing
	m.importing = &d
	switch key {
	case "esc", "q":
		m.importing = nil
		m.message = ""
	case "up", "k":
		d.cursor = max(0, d.cursor-1)
	case "down", "j":
		d.cursor = min(max(0, len(d.sessions)-1), d.cursor+1)
	case "enter":
		if len(d.sessions) == 0 {
			return m, nil
		}
		id := d.sessions[d.cursor].SessionID
		m.busy, m.busyText = true, "Adding "+d.agent+" session…"
		return m, func() tea.Msg { member, err := member.Import(d.agent, id, "", ""); return importDone{member, err} }
	}
	return m, nil
}
func (m Model) importStart(height int) int { return max(0, m.importing.cursor-max(1, (height-1)/2)+1) }
func (m Model) importView(height int) string {
	d := m.importing
	lines := []string{"Add existing " + importAgentLabel(d.agent) + " — select to add"}
	if len(d.sessions) == 0 {
		lines = append(lines, "No unregistered "+d.agent+" sessions.")
	}
	for i := m.importStart(height); i < len(d.sessions) && len(lines) < height; i++ {
		s := d.sessions[i]
		name := s.Name
		if name == "" {
			name = s.SessionID
		}
		lines = append(lines, control(clean(name)+" · "+clean(s.Status), i == d.cursor))
		if len(lines) < height {
			lines = append(lines, "  "+clean(s.Cwd))
		}
	}
	for i := range lines {
		lines[i] = fit(lines[i], m.detailWidth())
	}
	return strings.Join(lines[:min(len(lines), height)], "\n")
}

func importAgentLabel(agent string) string {
	if agent == "codex" {
		return "Codex"
	}
	return "Claude"
}
