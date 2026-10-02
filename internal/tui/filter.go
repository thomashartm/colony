package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/thomashartm/motley/internal/member"
)

// match is a case-insensitive subsequence match: "fxc" matches "FX cache".
// Preserve attention ordering while narrowing the visible rows.
func match(query, text string) bool {
	q := []rune(strings.ToLower(query))
	if len(q) == 0 {
		return true
	}
	i := 0
	for _, r := range strings.ToLower(text) {
		if r == q[i] {
			i++
			if i == len(q) {
				return true
			}
		}
	}
	return false
}
func (m *Model) applyFilter() {
	id, key := m.selectedID(), m.currentEntry().key()
	m.rows = nil
	for _, r := range m.allRows {
		if match(m.query.Value(), strings.Join([]string{r.ID, r.Name, r.Ticket, r.Repo, r.Branch}, " ")) {
			m.rows = append(m.rows, r)
		}
	}
	m.sortRows()
	m.selected = max(0, min(m.selected, len(m.rows)-1))
	for i, r := range m.rows {
		if r.ID == id {
			m.selected = i
		}
	}
	m.restoreCrewSelection(key, id)
	m.updateDetail()
}
func (m Model) updateFilter(msg tea.Msg) (tea.Model, tea.Cmd) {
	old := m.selectedID()
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc":
			m.query.SetValue("")
			m.searching = false
			m.query.Blur()
		case "enter":
			m.searching = false
			m.query.Blur()
		default:
			m.query, _ = m.query.Update(msg)
		}
	} else {
		m.query, _ = m.query.Update(msg)
	}
	m.applyFilter()
	return m, m.requestDetail(old != m.selectedID())
}
func (m Model) beginReply() (tea.Model, tea.Cmd) {
	if m.selectedID() == "" {
		return m, nil
	}
	r := m.selectedRow()
	if r.External {
		m.message = "Reply in its original terminal; Motley cannot type into it."
		return m, nil
	}
	if !r.Alive || r.CurrentStatus() == "ended" {
		m.message = "Agent is unavailable; jump to the member instead."
		return m, nil
	}
	if r.CurrentStatus() == "permission" {
		m.message = "Permission needs a decision in the agent; press Enter to jump."
		return m, nil
	}
	m.message = ""
	m.editor = newEditor("reply", r.ID, []string{"Reply (sent to the active pane)"}, []string{""})
	return m, nil
}
func (m Model) beginSend() (tea.Model, tea.Cmd) {
	if m.selectedID() == "" || !m.selectedRow().Alive {
		m.message = "Select a live member first."
		return m, nil
	}
	if m.selectedRow().External {
		m.message = "This session is in its original terminal; Motley cannot type into it."
		return m, nil
	}
	m.pickMode = "send"
	m.sendID = m.selectedID()
	m.choices = nil
	for _, c := range workClients(m.clients) {
		if c.Name != m.client && c.TTY != m.client {
			m.choices = append(m.choices, c)
		}
	}
	m.choice = 0
	if len(m.choices) == 0 {
		m.message = "No work tabs attached; open a tab with motley attach."
		return m, nil
	}
	m.picking = true
	return m, nil
}
func (m Model) updateSendPicker(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "q":
		m.picking = false
	case "j", "down":
		m.choice = min(m.choice+1, len(m.choices)-1)
	case "k", "up":
		m.choice = max(0, m.choice-1)
	case "enter":
		if len(m.choices) == 0 {
			return m, nil
		}
		id, tty := m.sendID, m.choices[m.choice].TTY
		m.picking = false
		m.busy = true
		return m, func() tea.Msg { return actionDone{err: member.Send(id, tty)} }
	}
	return m, nil
}
