package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	listPanel = iota
	detailPanel
	actionsPanel
)

type navigationAction struct{ label, key string }

func (m Model) actions() []navigationAction {
	actions := []navigationAction{}
	if m.selectedID() != "" {
		actions = append(actions, navigationAction{"Open agent (o)", "o"}, navigationAction{"Edit member", "e"})
	}
	actions = append(actions, navigationAction{"Manage crews", "G"}, navigationAction{"Spawn member", "s"}, navigationAction{"Add existing Claude (a)", "a"})
	if m.selectedID() != "" {
		if !m.selectedRow().External {
			actions = append(actions, navigationAction{"Reply", "i"}, navigationAction{"Send to work tab", "t"})
		}
		retire := "Retire member + worktree (x)"
		if m.selectedRow().ClaudeSession != "" {
			retire = "Retire member; keep files (x)"
		}
		actions = append(actions, navigationAction{"Terminate agent (X)", "X"}, navigationAction{retire, "x"}, navigationAction{"Revive member", "r"})
	}
	actions = append(actions, navigationAction{"Change grouping", "g"}, navigationAction{"Filter members", "/"})
	if m.group == "crew" {
		actions = append(actions, navigationAction{"Show/hide inactive crews", "H"})
	}
	if m.monitor {
		actions = append(actions, navigationAction{"Pin work tab", "T"})
	}
	return actions
}

// navigationKey runs after modal editors, so arrows in text remain caret keys.
func (m Model) navigationKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	key := msg.String()
	// Direct panel keys are handled after text inputs and confirmation dialogs.
	switch key {
	case "1":
		m.panel, m.tableFocus = listPanel, false
		return m, nil, true
	case "2":
		m.panel = detailPanel
		m.tableFocus = m.group == "crew" && m.currentEntry().id == "" && len(m.members(m.currentEntry().crew)) > 0
		return m, nil, true
	case "3":
		m.panel, m.actionCursor = actionsPanel, 0
		return m, nil, true
	}
	if m.panel == actionsPanel {
		actions := m.actions()
		m.actionCursor = max(0, min(m.actionCursor, len(actions)-1))
		switch key {
		case "up", "k":
			m.actionCursor = max(0, m.actionCursor-1)
		case "down", "j":
			m.actionCursor = min(len(actions)-1, m.actionCursor+1)
		case "left", "esc", "shift+tab":
			m.panel = detailPanel
		case "tab":
			m.panel, m.tableFocus = listPanel, false
		case "enter":
			next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(actions[m.actionCursor].key)})
			return next, cmd, true
		case "right":
		default:
			return m, nil, false
		}
		return m, nil, true
	}
	if m.tableFocus {
		m.panel = detailPanel
	}
	switch key {
	case "right":
		if m.panel == detailPanel {
			m.panel, m.actionCursor = actionsPanel, 0
		} else {
			e := m.currentEntry()
			if m.group == "crew" && e.id == "" && e.crew != "" && !m.expanded[e.crew] {
				return m, nil, false // retain the first Right's expand behavior
			}
			m.panel = detailPanel
			m.tableFocus = m.group == "crew" && e.id == "" && len(m.members(e.crew)) > 0
		}
		return m, nil, true
	case "left", "esc":
		if m.panel == detailPanel {
			m.panel, m.tableFocus = listPanel, false
			return m, nil, true
		}
	case "tab", "shift+tab":
		if m.group == "crew" {
			return m, nil, false
		} // preserve crew table's Tab toggle
		if key == "tab" {
			m.panel = (m.panel + 1) % 3
		} else {
			m.panel = (m.panel + 2) % 3
		}
		m.actionCursor = 0
		return m, nil, true
	case "up", "down", "j", "k":
		if m.panel == detailPanel && !m.tableFocus {
			m.detail, _ = m.detail.Update(msg)
			return m, nil, true
		}
	}
	return m, nil, false
}

func (m Model) actionsView(height int) string {
	actions := m.actions()
	cursor := max(0, min(m.actionCursor, len(actions)-1))
	title := "Actions / settings"
	if id := m.selectedID(); id != "" {
		title = "Actions: " + clean(id)
	}
	lines := []string{fit(title, m.detailWidth())}
	start := max(0, cursor-max(1, height-1)+1)
	for i := start; i < len(actions) && len(lines) < height; i++ {
		lines = append(lines, control(actions[i].label, i == cursor))
	}
	return strings.Join(lines, "\n")
}

func control(label string, focused bool) string {
	if focused {
		return "> " + label
	}
	return "  " + label
}
