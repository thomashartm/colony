package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

const navigationBar = " [List] [Details] [Actions] [Enter] [q]"

func (m Model) navigationAvailable() bool {
	return !m.busy && !m.searching && m.spawn == nil && m.editor == nil && !m.manager && m.retiring == nil && !m.picking
}

func (m Model) mouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if !m.navigationAvailable() || m.width < 60 || m.height < 10 || msg.X < 0 || msg.X >= m.width || msg.Y < 0 || msg.Y >= m.height {
		return m, nil
	}
	if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
		if msg.Y < 2 || msg.Y >= m.height-3 {
			return m, nil
		}
		if msg.X > m.listWidth()+2 && m.group == "crew" && m.currentEntry().id == "" && m.panel != actionsPanel {
			m.panel, m.tableFocus = detailPanel, true
		}
		if msg.X > m.listWidth()+2 && m.panel != actionsPanel && !m.tableFocus {
			m.panel = detailPanel
			m.detail, _ = m.detail.Update(msg)
			return m, nil
		}
		if msg.X <= m.listWidth()+2 {
			m.panel, m.tableFocus = listPanel, false
		}
		key := tea.KeyDown
		if msg.Button == tea.MouseButtonWheelUp {
			key = tea.KeyUp
		}
		return m.Update(tea.KeyMsg{Type: key})
	}
	if msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionPress {
		return m, nil
	}
	if msg.Y == m.height-1 {
		for _, button := range []string{"[List]", "[Details]", "[Actions]", "[Enter]", "[q]"} {
			start := strings.Index(navigationBar, button)
			if msg.X < start || msg.X >= start+len(button) {
				continue
			}
			switch button {
			case "[List]":
				m.panel, m.tableFocus = listPanel, false
			case "[Details]":
				m.panel = detailPanel
				m.tableFocus = m.group == "crew" && m.currentEntry().id == "" && len(m.members(m.currentEntry().crew)) > 0
			case "[Actions]":
				m.panel, m.actionCursor = actionsPanel, 0
			case "[Enter]":
				return m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			case "[q]":
				return m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
			}
			return m, nil
		}
	}
	height := max(1, m.height-5)
	y := msg.Y - 2 // header and top border
	if y < 0 || y >= height {
		return m, nil
	}
	if msg.X >= 1 && msg.X <= m.listWidth() {
		old := m.selectedID()
		m.panel, m.tableFocus = listPanel, false
		if m.group == "crew" {
			entries := m.crewEntries()
			index := max(0, m.crewCursor-height+1) + y
			if index >= len(entries) {
				return m, nil
			}
			again := index == m.crewCursor
			m.crewCursor = index
			m.tableCursor = 0
			if again && entries[index].id == "" {
				return m.Update(tea.KeyMsg{Type: tea.KeySpace})
			}
		} else {
			lines, selectedLine, last := []int{}, 0, ""
			for i, row := range m.rows {
				group := section(row)
				if m.group == "repo" {
					group = clean(row.Repo)
				}
				if group != last {
					lines = append(lines, -1)
					last = group
				}
				if i == m.selected {
					selectedLine = len(lines)
				}
				lines = append(lines, i)
			}
			index := max(0, selectedLine-height+1) + y
			if index >= len(lines) || lines[index] < 0 {
				return m, nil
			}
			m.selectRow(lines[index])
		}
		m.updateDetail()
		return m, m.requestDetail(old != m.selectedID())
	}
	if msg.X > m.listWidth()+2 && msg.X < m.width-1 {
		if m.panel == actionsPanel {
			actions := m.actions()
			index := max(0, m.actionCursor-max(1, height-1)+1) + y - 1
			if y == 0 || index < 0 || index >= len(actions) {
				return m, nil
			}
			m.actionCursor = index
			return m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		}
		m.panel = detailPanel
		if m.group == "crew" && m.currentEntry().id == "" {
			e := m.currentEntry()
			headers := 2
			if m.crewFor(e.crew).Gig != "" {
				headers++
			}
			members := m.members(e.crew)
			start := max(0, m.tableCursor-max(1, height-headers)+1)
			index := start + y - headers
			if y >= headers && index >= 0 && index < len(members) {
				old := m.selectedID()
				m.tableFocus, m.tableCursor = true, index
				m.updateDetail()
				return m, m.requestDetail(old != m.selectedID())
			}
		}
	}
	return m, nil
}
