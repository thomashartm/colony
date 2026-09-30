package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/thomashartm/colony/internal/crew"
	"github.com/thomashartm/colony/internal/minion"
	"github.com/thomashartm/colony/internal/palette"
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
func (m Model) editMinion() (tea.Model, tea.Cmd) {
	if m.selectedID() == "" {
		m.message = "Select a minion, or Tab into the crew's members."
		return m, nil
	}
	r := m.selectedRow()
	m.message = ""
	m.editor = newEditor("minion", r.ID, []string{"Name", "Ticket", "Crew id (empty = none)", "Colour (empty = inherit)"}, []string{r.Name, r.Ticket, r.Crew, r.Color})
	return m, textinput.Blink
}
func (m Model) updateManager(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "q":
		m.manager = false
	case "j", "down":
		m.managerCursor = min(m.managerCursor+1, max(0, len(m.crews)-1))
	case "k", "up":
		m.managerCursor = max(0, m.managerCursor-1)
	case "a":
		m.message = ""
		m.editor = newEditor("add", "", []string{"Title", "URL (optional)", "Colour (empty = automatic)"}, []string{"", "", ""})
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
				err := minion.EditCrew(c.ID, minion.CrewEdit{Color: &color})
				crews, loadErr := crew.Load()
				if err == nil {
					err = loadErr
				}
				return identitySaved{crews: crews, err: err}
			}
		}
		m.editor = newEditor("crew", c.ID, []string{"Title", "URL (optional)", "Colour"}, []string{c.Title, c.URL, c.Color})
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
		case "tab", "shift+tab":
			if len(e.fields) == 0 {
				return m, nil
			}
			e.fields[e.focus].Blur()
			delta := 1
			if key.String() == "shift+tab" {
				delta = -1
			}
			e.focus = (e.focus + delta + len(e.fields)) % len(e.fields)
			return m, e.fields[e.focus].Focus()
		case "f":
			if e.kind == "delete" {
				e.force = !e.force
				return m, nil
			}
		case "ctrl+s", "y":
			if key.String() == "y" && e.kind != "delete" {
				break
			}
			m.busy = true
			m.busyText = "Saving…"
			e.err = ""
			return m, func() tea.Msg {
				var err error
				switch e.kind {
				case "add":
					_, err = minion.AddCrew(e.fields[0].Value(), e.fields[1].Value(), e.fields[2].Value())
				case "crew":
					a, b, c := e.fields[0].Value(), e.fields[1].Value(), e.fields[2].Value()
					err = minion.EditCrew(e.id, minion.CrewEdit{Title: &a, URL: &b, Color: &c})
				case "minion":
					a, b, c, d := e.fields[0].Value(), e.fields[1].Value(), e.fields[2].Value(), e.fields[3].Value()
					err = minion.EditIdentity(e.id, minion.IdentityEdit{Name: &a, Ticket: &b, Crew: &c, Color: &d})
				case "delete":
					err = minion.RemoveCrew(e.id, e.force)
				}
				crews, loadErr := crew.Load()
				if err == nil {
					err = loadErr
				}
				return identitySaved{crews: crews, err: err}
			}
		}
	}
	if len(e.fields) > 0 {
		var cmd tea.Cmd
		e.fields[e.focus], cmd = e.fields[e.focus].Update(msg)
		return m, cmd
	}
	return m, nil
}
func (m Model) managerView(height int) string {
	lines := []string{"Crews", "a add · e edit · c colour · x delete"}
	if len(m.crews) == 0 {
		return strings.Join(append(lines, "No crews yet. Press a to add one."), "\n")
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
		return fit("Delete crew "+e.id+"?", m.detailWidth()) + "\n" + fmt.Sprintf("Force unassign: %t", e.force) + "\n" + fit(e.err, m.detailWidth())
	}
	lines := []string{fit(title, m.detailWidth())}
	// Scroll fields with focus so the form works at the minimum terminal height.
	first := max(0, e.focus-(max(1, (height-2)/2)-1))
	for i := first; i < len(e.fields) && len(lines)+2 <= height-1; i++ {
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
