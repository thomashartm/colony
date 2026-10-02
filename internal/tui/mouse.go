package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func (m Model) navigationBar() string {
	bar := "[o Open agent] · [1 List] · [2 Details] · [3 Actions] · [q Close]"
	if ansi.StringWidth(bar) > m.width-2 {
		return strings.ReplaceAll(bar, " · ", "·")
	}
	return bar
}

func (m Model) navigationAvailable() bool {
	return !m.busy && !m.searching && m.spawn == nil && m.editor == nil && !m.manager && m.retiring == nil && m.terminating == nil && m.importing == nil && m.opening == nil && !m.picking
}

func (m Model) mouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if !m.busy && (m.navigationAvailable() || m.opening != nil) &&
		m.width >= 60 && m.height >= 10 && msg.X >= 0 && msg.X < m.width && msg.Y >= 2 && msg.Y < 2+m.panelHeight() &&
		msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
		if target := hyperlinkAt(m.View(), msg.X, msg.Y); target != "" {
			return m.openLink(target)
		}
	}

	if m.opening != nil {
		return m.agentPickerMouse(msg)
	}
	if m.editor != nil && !m.busy && m.width >= 60 && m.height >= 10 {
		return m.editorMouse(msg)
	}

	if m.terminating != nil && !m.busy && m.width >= 60 && m.height >= 10 && msg.Y == m.height-1 && msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
		for _, b := range []struct{ label, key string }{{"[Cancel: esc]", "esc"}, {"[Terminate: y]", "y"}} {
			start := strings.Index(m.terminateButtons(), b.label)
			if msg.X >= start && msg.X < start+len(b.label) {
				return m.updateTerminate(b.key)
			}
		}
		return m, nil
	}
	if m.retiring != nil && !m.busy && m.width >= 60 && m.height >= 10 && msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress && msg.X > m.listWidth()+2 && msg.X < m.width-1 {
		lines := strings.Split(m.retireView(m.contentHeight()), "\n")
		// Choices follow the wrapped explanation in the right panel.
		count := len(m.retireChoices())
		first := len(lines) - count
		index := m.panelContentY(msg.Y) - first
		if first >= 1 && index >= 0 && index < count {
			dialog := *m.retiring
			dialog.focus = index
			m.retiring = &dialog
			return m.updateRetire("enter")
		}
		return m, nil
	}
	if m.importing != nil && !m.busy && m.width >= 60 && m.height >= 10 && msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress && msg.X > m.listWidth()+2 && msg.X < m.width-1 {
		y := m.panelContentY(msg.Y) - 1
		index := m.importStart(m.contentHeight()) + y/2
		if y >= 0 && msg.Y < 2+m.panelHeight() && index < len(m.importing.sessions) {
			d := *m.importing
			d.cursor = index
			m.importing = &d
			return m.updateImport("enter")
		}
		return m, nil
	}
	if !m.navigationAvailable() || m.width < 60 || m.height < 10 || msg.X < 0 || msg.X >= m.width || msg.Y < 0 || msg.Y >= m.height {
		return m, nil
	}
	if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
		if msg.Y < 2 || msg.Y >= 2+m.panelHeight() {
			return m, nil
		}
		if msg.X > m.listWidth()+2 && !m.overview && m.group == "crew" && m.currentEntry().id == "" && m.panel != actionsPanel {
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
	// Hover only changes selection and help; a left click activates the action.
	if m.panel == actionsPanel && msg.X > m.listWidth()+2 && msg.X < m.width-1 &&
		msg.Y >= 2 && msg.Y < 2+m.panelHeight() &&
		(msg.Action == tea.MouseActionMotion || (msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress)) {
		layout := m.layoutActions(m.contentHeight())
		y := m.panelContentY(msg.Y) - 1 // exclude the panel title
		if y >= 0 && y < layout.menuHeight && layout.start+y < len(layout.rows) {
			if index := layout.rows[layout.start+y].index; index >= 0 {
				m.actionCursor, m.actionScroll = index, layout.start
				if msg.Action == tea.MouseActionPress {
					return m.Update(tea.KeyMsg{Type: tea.KeyEnter})
				}
			}
		}
		return m, nil
	}
	if msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionPress {
		return m, nil
	}
	// The message line sits between the panels and the footer.
	if msg.Y == m.height-1-m.footerRows() && m.copyableMessage() != "" {
		return m.copyMessage()
	}
	if msg.Y == m.height-1 {
		bar := m.navigationBar()
		for _, button := range []string{"[o Open agent]", "[1 List]", "[2 Details]", "[3 Actions]", "[q Close]"} {
			start := ansi.StringWidth(bar[:strings.Index(bar, button)])
			if msg.X < start || msg.X >= start+len(button) {
				continue
			}
			switch button {
			case "[1 List]":
				m.panel, m.tableFocus = listPanel, false
			case "[2 Details]":
				m.panel = detailPanel
				m.tableFocus = !m.overview && m.group == "crew" && m.currentEntry().id == "" && len(m.members(m.currentEntry().crew)) > 0
			case "[3 Actions]":
				m.panel, m.actionCursor, m.actionScroll = actionsPanel, 0, 0
			case "[o Open agent]":
				return m.jump()
			case "[q Close]":
				return m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
			}
			return m, nil
		}
	}
	height := m.contentHeight()
	y := m.panelContentY(msg.Y)
	if msg.X >= 1 && msg.X <= m.listWidth() {
		height = m.listContentHeight()
		y = contentY(msg.Y, m.listHeadingGap())
	}
	if y < 0 || y >= height {
		return m, nil
	}
	if msg.X >= 1 && msg.X <= m.listWidth() {
		old := m.selectedID()
		m.panel, m.tableFocus = listPanel, false
		layout := m.listLayout(height, m.listWidth())
		target := listHeading
		if y < len(layout.fixed) {
			target = layout.fixed[y].index
		} else if index := layout.start + y - len(layout.fixed); index < len(layout.body) {
			target = layout.body[index].index
		}
		if target == listHeading {
			return m, nil
		}
		if target == overviewEntry {
			m.selectOverview()
			m.panel, m.actionCursor, m.actionScroll = actionsPanel, 0, 0
		} else if m.group == "crew" {
			entries := m.crewEntries()
			again := !m.overview && target == m.crewCursor
			m.overview, m.crewCursor, m.tableCursor = false, target, 0
			if again && entries[target].id == "" {
				return m.Update(tea.KeyMsg{Type: tea.KeySpace})
			}
		} else {
			m.selectRow(target)
		}

		m.updateDetail()
		return m, m.requestDetail(old != m.selectedID())
	}
	if msg.X > m.listWidth()+2 && msg.X < m.width-1 {
		m.panel = detailPanel
		if !m.overview && m.group == "crew" && m.currentEntry().id == "" {
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
