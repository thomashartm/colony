package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/thomashartm/motley/internal/member"
	"github.com/thomashartm/motley/internal/state"
	"strings"
)

func (m *Model) selectOverview() {
	m.overview, m.tableFocus = true, false
	m.message = ""
	m.detail.GotoTop()
	m.updateDetail()
}

// The picker stores IDs, never list offsets: background refreshes must not
// silently change the target of Open.
type agentPicker struct {
	ids    []string
	cursor int
}

func (m Model) beginAgentPicker() (tea.Model, tea.Cmd) {
	p := &agentPicker{}
	for _, r := range m.allRows {
		if r.Alive {
			p.ids = append(p.ids, r.ID)
		}
	}
	m.opening, m.message = p, ""
	return m, nil
}
func (m Model) agentPickerStart(height int) int {
	return max(0, m.opening.cursor-max(1, height-2)+1)
}
func (m Model) pickerMember(id string) (member.Row, bool) {
	for _, r := range m.allRows {
		if r.ID == id {
			return r, true
		}
	}
	return member.Row{}, false
}
func (m Model) updateAgentPicker(key string) (tea.Model, tea.Cmd) {
	p := *m.opening
	m.opening = &p
	switch key {
	case "esc", "q":
		m.opening = nil
	case "up", "k":
		p.cursor = max(0, p.cursor-1)
	case "down", "j":
		p.cursor = max(0, min(len(p.ids)-1, p.cursor+1))
	case "enter":
		if len(p.ids) == 0 {
			return m, nil
		}
		id := p.ids[p.cursor]
		r, ok := m.pickerMember(id)
		if !ok || !r.Alive {
			m.message = "This agent is no longer running. Close the picker and choose again."
			return m, nil
		}
		m.opening = nil
		m.query.SetValue("")
		m.applyFilter()
		m.overview, m.tableFocus = false, false
		for i, row := range m.rows {
			if row.ID == id {
				m.selected = i
				break
			}
		}
		if m.group == "crew" {
			if m.expanded == nil {
				m.expanded = map[string]bool{}
			}
			m.expanded[m.crewFor(r.Crew).ID] = true
			m.restoreCrewSelection("member:"+id, id)
		}
		m.event, m.gitDetail = state.Event{}, ""
		m.updateDetail()
		return m.jump()
	}
	return m, nil
}
func (m Model) agentPickerView(height int) string {
	width := m.detailWidth()
	header := "  " + m.memberTableHeader(width-2)
	if width < 40 {
		header = "  STATUS / TITLE"
	}
	lines := []string{"Open agent", header}
	if len(m.opening.ids) == 0 {
		return "Open agent\nNo running agents.\nEsc returns to main actions."
	}
	for i := m.agentPickerStart(height); i < len(m.opening.ids) && len(lines) < height; i++ {
		id := m.opening.ids[i]
		r, ok := m.pickerMember(id)
		if !ok {
			r = member.Row{Manifest: member.Manifest{ID: id, Name: "Unavailable"}}
		}
		// A visible marker identifies keyboard focus even on terminals without colour.
		text := m.memberTableRow(r, max(1, width-2), false)
		if width < 40 {
			name := r.Name
			if name == "" {
				name = r.ID
			}
			icon, _ := statusIcon(r.CurrentStatus())
			text = icon + " " + clean(name)
		}
		line := control(text, i == m.opening.cursor)
		lines = append(lines, fit(line, width))
	}
	return strings.Join(lines, "\n")
}
func (m Model) agentPickerMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.busy || m.width < 60 || m.height < 10 || msg.X <= m.listWidth()+2 || msg.X >= m.width-1 || msg.Y >= 2+m.panelHeight() {
		return m, nil
	}
	y := m.panelContentY(msg.Y) - 2
	if y < 0 {
		return m, nil
	}
	if msg.Button == tea.MouseButtonWheelUp {
		return m.updateAgentPicker("up")
	}
	if msg.Button == tea.MouseButtonWheelDown {
		return m.updateAgentPicker("down")
	}
	if msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress {
		index := m.agentPickerStart(m.contentHeight()) + y
		if index < len(m.opening.ids) {
			p := *m.opening
			p.cursor = index
			m.opening = &p
			return m.updateAgentPicker("enter")
		}
	}
	return m, nil
}
