package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/thomashartm/motley/internal/crew"
	"github.com/thomashartm/motley/internal/member"
	"github.com/thomashartm/motley/internal/palette"
)

type identityEditor struct {
	kind, id string
	labels   []string
	fields   []textinput.Model
	focus    int
	err      string
	force    bool
}
type identitySaved struct {
	crews []crew.Crew
	err   error
}

func newEditor(kind, id string, labels, values []string) *identityEditor {
	e := &identityEditor{kind: kind, id: id, labels: labels}
	for _, v := range values {
		input := textinput.New()
		input.CharLimit = 2048
		input.SetValue(v)
		input.Prompt = ""
		e.fields = append(e.fields, input)
	}
	if len(e.fields) > 0 {
		e.fields[0].Focus()
	}
	return e
}
func (m Model) editMember() (tea.Model, tea.Cmd) {
	if m.selectedID() == "" {
		m.message = "Select a member, or Tab into the crew's members."
		return m, nil
	}
	r := m.selectedRow()
	m.message = ""
	m.editor = newEditor("member", r.ID, []string{"Name", "Ticket", "Crew id (empty = none)", "Colour (empty = inherit)"}, []string{r.Name, r.Ticket, r.Crew, r.Color})
	return m, textinput.Blink
}
func (m Model) updateManager(key string) (tea.Model, tea.Cmd) {
	if m.managerActions {
		switch key {
		case "up", "k":
			m.managerAction = max(0, m.managerAction-1)
			return m, nil
		case "down", "j":
			m.managerAction = min(4, m.managerAction+1)
			return m, nil
		case "left", "esc":
			m.managerActions = false
			return m, nil
		case "enter":
			m.managerActions = false
			key = []string{"a", "e", "c", "x", "esc"}[m.managerAction]
		}
	}
	switch key {
	case "esc", "q", "left":
		m.manager = false
		m.managerActions = false
	case "right":
		m.managerActions = true
		m.managerAction = 0
	case "enter":
		if len(m.crews) == 0 {
			return m.updateManager("a")
		}
		return m.updateManager("e")
	case "j", "down":
		m.managerCursor = min(m.managerCursor+1, max(0, len(m.crews)-1))
	case "k", "up":
		m.managerCursor = max(0, m.managerCursor-1)
	case "a":
		m.message = ""
		m.editor = newEditor("add", "", []string{"Title", "URL (optional)", "Colour (empty = automatic)", "Gig (optional)"}, []string{"", "", "", ""})
		return m, textinput.Blink
	case "e", "c", "x":
		m.message = ""
		if len(m.crews) == 0 {
			return m, nil
		}
		c := m.crews[m.managerCursor]
		if key == "x" {
			m.editor = newEditor("delete", c.ID, nil, nil)
			return m, nil
		}
		if key == "c" {
			color := palette.Colors[0].Name
			for i, p := range palette.Colors {
				if p.Name == c.Color {
					color = palette.Colors[(i+1)%len(palette.Colors)].Name
				}
			}
			m.busy = true
			m.busyText = "Recolouring…"
			return m, func() tea.Msg {
				err := member.EditCrew(c.ID, member.CrewEdit{Color: &color})
				crews, loadErr := crew.Load()
				if err == nil {
					err = loadErr
				}
				return identitySaved{crews: crews, err: err}
			}
		}
		m.editor = newEditor("crew", c.ID, []string{"Title", "URL (optional)", "Colour", "Gig (optional)"}, []string{c.Title, c.URL, c.Color, c.Gig})
		return m, textinput.Blink
	}
	return m, nil
}
func (m Model) updateEditor(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Copy the fields before editing; save commands capture an immutable snapshot.
	e := *m.editor
	e.fields = append([]textinput.Model(nil), e.fields...)
	m.editor = &e
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc":
			m.editor = nil
			return m, nil
		case "tab", "shift+tab", "up", "down":
			if e.focus < len(e.fields) {
				e.fields[e.focus].Blur()
			}
			delta := 1
			if key.String() == "shift+tab" || key.String() == "up" {
				delta = -1
			}
			size := len(e.fields) + 2
			if e.kind == "delete" {
				size = 3
			}
			e.focus = (e.focus + delta + size) % size
			if e.focus < len(e.fields) {
				return m, e.fields[e.focus].Focus()
			}
			return m, nil
		case "f":
			if e.kind == "delete" {
				e.force = !e.force
				return m, nil
			}
		case "enter", "ctrl+s", "y":
			if key.String() == "enter" {
				if e.kind == "delete" {
					switch e.focus {
					case 0:
						e.force = !e.force
						return m, nil
					case 2:
						m.editor = nil
						return m, nil
					}
				} else if e.focus == len(e.fields)+1 {
					m.editor = nil
					return m, nil
				} else if e.focus < len(e.fields) && e.kind != "reply" {
					e.fields[e.focus].Blur()
					e.focus++
					if e.focus < len(e.fields) {
						return m, e.fields[e.focus].Focus()
					}
					return m, nil
				}
			}
			if key.String() == "y" && e.kind != "delete" {
				break
			}
			m.busy = true
			m.busyText = "Saving…"
			e.err = ""
			return m, func() tea.Msg {
				var err error
				switch e.kind {
				case "reply":
					err = member.Reply(e.id, e.fields[0].Value())
				case "add":
					_, err = member.AddCrew(e.fields[0].Value(), e.fields[1].Value(), e.fields[2].Value(), e.fields[3].Value())
				case "crew":
					a, b, c, d := e.fields[0].Value(), e.fields[1].Value(), e.fields[2].Value(), e.fields[3].Value()
					err = member.EditCrew(e.id, member.CrewEdit{Title: &a, URL: &b, Color: &c, Gig: &d})
				case "member":
					a, b, c, d := e.fields[0].Value(), e.fields[1].Value(), e.fields[2].Value(), e.fields[3].Value()
					err = member.EditIdentity(e.id, member.IdentityEdit{Name: &a, Ticket: &b, Crew: &c, Color: &d})
				case "delete":
					err = member.RemoveCrew(e.id, e.force)
				}
				crews, loadErr := crew.Load()
				if err == nil {
					err = loadErr
				}
				return identitySaved{crews: crews, err: err}
			}
		}
	}
	if e.focus < len(e.fields) {
		var cmd tea.Cmd
		e.fields[e.focus], cmd = e.fields[e.focus].Update(msg)
		return m, cmd
	}
	return m, nil
}
func (m Model) managerView(height int) string {
	if m.managerActions {
		labels := []string{"Add crew", "Edit crew", "Cycle colour", "Delete crew", "Back"}
		lines := []string{"Crew actions"}
		start := max(0, m.managerAction-max(1, height-1)+1)
		for i := start; i < len(labels) && len(lines) < height; i++ {
			lines = append(lines, control(labels[i], i == m.managerAction))
		}
		return strings.Join(lines, "\n")
	}
	lines := []string{"Crews", "→ actions · enter edit · a add"}
	if len(m.crews) == 0 {
		return strings.Join(append(lines, "No crews yet. Enter to add."), "\n")
	}
	start := max(0, m.managerCursor-max(1, height-2)+1)
	for i := start; i < len(m.crews) && len(lines) < height; i++ {
		c := m.crews[i]
		prefix := "  "
		if i == m.managerCursor {
			prefix = "> "
		}
		lines = append(lines, fit(prefix+colored("▌ "+clean(c.Title), palette.Resolve(c.ID, c.Color, ""))+" · "+c.ID, m.detailWidth()))
	}
	return strings.Join(lines, "\n")
}
func (m Model) editorView(height int) string {
	e := m.editor
	title := "Edit " + e.id
	if e.kind == "add" {
		title = "Add crew"
	}
	if e.kind == "delete" {
		lines := []string{"Delete crew " + e.id + "?", control(fmt.Sprintf("Force unassign: %t", e.force), e.focus == 0), control("Delete", e.focus == 1), control("Cancel", e.focus == 2), e.err}
		for i := range lines {
			lines[i] = fit(lines[i], m.detailWidth())
		}
		return strings.Join(lines[:min(height, len(lines))], "\n")
	}
	lines := []string{fit(title, m.detailWidth())}
	// Scroll fields with focus so the form works at the minimum terminal height.
	first := max(0, e.focus-(max(1, (height-2)/2)-1))
	for i := first; i < len(e.fields)+2; i++ {
		if i >= len(e.fields) {
			if len(lines) >= height-1 {
				break
			}
			label := "Save"
			if e.kind == "reply" {
				label = "Send"
			}
			if i == len(e.fields)+1 {
				label = "Cancel"
			}
			lines = append(lines, control(label, i == e.focus))
			continue
		}
		if len(lines)+2 > height-1 {
			break
		}
		field := e.fields[i]
		field.Width = max(1, m.detailWidth()-2)
		label := e.labels[i]
		if i == e.focus {
			label = "> " + label
		}
		lines = append(lines, fit(label, m.detailWidth()), fit(field.View(), m.detailWidth()))
	}
	if e.err != "" {
		lines = append(lines, fit(e.err, m.detailWidth()))
	} else {
		lines = append(lines, fit("Colours: red orange yellow green blue purple brown grey", m.detailWidth()))
	}
	return strings.Join(lines[:min(len(lines), height)], "\n")
}
