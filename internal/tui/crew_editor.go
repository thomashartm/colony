package tui

import (
	"context"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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

// The add form's empty title is fetched for GitHub issue and project URLs.
var (
	addCrewLabels = []string{"Title (blank: fetched from a GitHub issue or project URL)", "URL (optional)", "Colour", "Gig (optional)"}
	crewLabels    = []string{"Title", "URL (optional)", "Colour", "Gig (optional)"}
)

func newEditor(kind, id string, labels, values []string) *identityEditor {
	e := &identityEditor{kind: kind, id: id, labels: labels}
	for _, v := range values {
		input := textinput.New()
		input.CharLimit = 2048
		input.SetValue(v)
		input.Prompt = ""
		input.Placeholder = "Not set"
		e.fields = append(e.fields, input)
	}
	if len(e.fields) > 0 {
		e.fields[0].Focus()
	}
	return e
}
func (e *identityEditor) colourField(index int) bool {
	if index < 0 || index >= len(e.fields) {
		return false
	}
	return (e.kind == "member" && index == 3) || ((e.kind == "add" || e.kind == "crew") && index == 2)
}

func (e *identityEditor) selectorName(index int) string {
	if e.colourField(index) {
		return "Colour"
	}
	if e.kind == "member" && index == 2 && index < len(e.fields) {
		return "Crew"
	}
	return ""
}

func (e *identityEditor) focusField() tea.Cmd {
	if e.focus >= len(e.fields) || e.selectorName(e.focus) != "" {
		return nil
	}
	return e.fields[e.focus].Focus()
}

func (e *identityEditor) changeSelection(crews []crew.Crew, step int) {
	options := []string{""}
	if e.colourField(e.focus) {
		for _, colour := range palette.Colors {
			options = append(options, colour.Name)
		}
	} else {
		for _, c := range crews {
			options = append(options, c.ID)
		}
	}
	selected := 0
	for i, name := range options {
		if e.fields[e.focus].Value() == name {
			selected = i
			break
		}
	}
	e.fields[e.focus].SetValue(options[(selected+step+len(options))%len(options)])
}

// Reserve space for the longest option, capped by the panel, so arrow
// targets stay put while cycling. Colour selectors retain their compact width.
func (e *identityEditor) selectorWidth(index, width int, crews []crew.Crew) int {
	size := 11
	if e.selectorName(index) == "Crew" {
		for _, c := range crews {
			size = max(size, ansi.StringWidth(clean(c.Title))+2)
		}
		if id := e.fields[index].Value(); id != "" {
			if _, ok := crew.Find(crews, id); !ok {
				size = max(size, len("Unavailable crew"))
			}
		}
	}
	return min(size, max(1, width-6))
}

func (e *identityEditor) selectorView(index, width int, crews []crew.Crew) string {
	preview := "No crew"
	if e.colourField(index) {
		name := "Automatic"
		if e.kind == "member" {
			name = "Inherit"
		}
		preview = "◇ " + name
		if colour, ok := palette.Lookup(e.fields[index].Value()); ok {
			preview = colored("■ "+colour.Name, colour)
		}
	} else if id := e.fields[index].Value(); id != "" {
		preview = "Unavailable crew"
		if c, ok := crew.Find(crews, id); ok {
			preview = colored("■ "+clean(c.Title), palette.Resolve(c.ID, c.Color, ""))
		}
	}
	size := e.selectorWidth(index, width, crews)
	preview = fit(preview, size)
	preview += strings.Repeat(" ", max(0, size-ansi.StringWidth(preview)))
	arrow := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	return "  " + arrow.Render("‹") + " " + preview + " " + arrow.Render("›")
}

func (m Model) editMember() (tea.Model, tea.Cmd) {
	if m.selectedID() == "" {
		m.message = "Select a member, or Tab into the crew's members."
		return m, nil
	}
	r := m.selectedRow()
	m.message = ""
	m.editor = newEditor("member", r.ID, []string{"Name", "Ticket", "Crew", "Colour"}, []string{r.Name, r.Ticket, r.Crew, r.Color})
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
		m.editor = newEditor("add", "", addCrewLabels, []string{"", "", "", ""})
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
		m.editor = newEditor("crew", c.ID, crewLabels, []string{c.Title, c.URL, c.Color, c.Gig})
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
		if e.selectorName(e.focus) != "" && (key.String() == "left" || key.String() == "right") {
			step := 1
			if key.String() == "left" {
				step = -1
			}
			e.changeSelection(m.crews, step)
			return m, nil
		}
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
				return m, e.focusField()
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
						return m, e.focusField()
					}
					return m, nil
				}
			}
			if key.String() == "y" && e.kind != "delete" {
				break
			}
			m.busy = true
			m.busyText = "Saving…"
			if e.kind == "add" && strings.TrimSpace(e.fields[0].Value()) == "" && strings.TrimSpace(e.fields[1].Value()) != "" {
				m.busyText = "Fetching crew title…"
			}
			e.err = ""
			client := m.github
			return m, func() tea.Msg {
				var err error
				switch e.kind {
				case "reply":
					err = member.Reply(e.id, e.fields[0].Value())
				case "add":
					if client == nil { // lookups disabled: the title is required
						_, err = member.AddCrew(e.fields[0].Value(), e.fields[1].Value(), e.fields[2].Value(), e.fields[3].Value())
						break
					}
					ctx, cancel := context.WithTimeout(context.Background(), member.LookupTimeout)
					_, err = member.AddCrewWithLookup(ctx, client, e.fields[0].Value(), e.fields[1].Value(), e.fields[2].Value(), e.fields[3].Value())
					cancel()
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
	if e.focus < len(e.fields) && e.selectorName(e.focus) == "" {
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

type editorTarget struct {
	x, y, width, focus int
	action             bool
	selectorStep       int
}

func editorButton(label string, focused, warning bool) string {
	colour := lipgloss.Color("6")
	if warning {
		colour = lipgloss.Color("3")
	}
	style := lipgloss.NewStyle().Bold(true).Foreground(colour)
	if focused {
		style = style.Background(colour).Foreground(lipgloss.Color("0"))
	}
	return style.Render(control("[ "+label+" ]", focused))
}

func (m Model) editorLayout(height int) ([]string, []editorTarget) {
	e := m.editor
	width := m.detailWidth()
	title := "Edit " + clean(e.id)
	switch e.kind {
	case "add":
		title = "Add crew"
	case "reply":
		title = "Reply to " + clean(e.id)
	case "delete":
		title = "Delete crew " + clean(e.id) + "?"
	}
	lines := make([]string, height)
	lines[0] = fit(title, width)
	targets := []editorTarget{}
	// Keep actions at the bottom; scroll only the fields. The compact layout
	// retains a full label/value pair, a hint/error row and the buttons.
	buttonY := height - 1
	hintY := buttonY - 1
	if height >= 8 {
		hintY--
	}
	fieldEnd := hintY
	if height >= 10 {
		fieldEnd--
	}
	if e.kind == "delete" {
		check := "off"
		if e.force {
			check = "on"
		}
		lines[1] = fit(editorButton("Force unassign: "+check, e.focus == 0, true), width)
		targets = append(targets, editorTarget{x: 0, y: 1, width: min(width, lipgloss.Width(lines[1])), focus: 0, action: true})
	} else if len(e.fields) > 0 {
		var fieldLines []string
		var fieldFocus []int
		selectedLine := 0
		selected := min(e.focus, len(e.fields)-1)
		for i, input := range e.fields {
			if i > 0 {
				fieldLines = append(fieldLines, "")
				fieldFocus = append(fieldFocus, -1)
			}
			if i == selected {
				selectedLine = len(fieldLines)
			}
			labelStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("8"))
			if i == e.focus {
				labelStyle = labelStyle.Foreground(lipgloss.Color("6"))
			}
			label := labelStyle.Render(fit(e.labels[i]+":", width))
			input.Width = max(1, width-4)
			value := lipgloss.NewStyle().PaddingLeft(2).Render(input.View())
			if e.selectorName(i) != "" {
				value = e.selectorView(i, width, m.crews)
			}
			fieldLines = append(fieldLines, label, fit(value, width))
			fieldFocus = append(fieldFocus, i, i)
		}
		available := max(2, fieldEnd-1)
		start := max(0, selectedLine+2-available)
		// Never begin with a dangling value or gap from the previous field.
		for start > 0 && (fieldFocus[start] < 0 || fieldFocus[start] == fieldFocus[start-1]) {
			start++
		}
		for y, i := 1, start; y < fieldEnd && i < len(fieldLines); y, i = y+1, i+1 {
			lines[y] = fieldLines[i]
			if fieldFocus[i] >= 0 {
				focus := fieldFocus[i]
				if e.selectorName(focus) != "" && i > 0 && fieldFocus[i-1] == focus {
					targets = append(targets,
						editorTarget{x: 2, y: y, width: 2, focus: focus, selectorStep: -1},
						editorTarget{x: e.selectorWidth(focus, width, m.crews) + 4, y: y, width: 2, focus: focus, selectorStep: 1})
				}
				targets = append(targets, editorTarget{x: 0, y: y, width: width, focus: focus})
			}
		}
	}
	if e.err != "" {
		lines[hintY] = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render(fit(e.err, width))
	} else if name := e.selectorName(e.focus); name != "" {
		hint := "←/→ choose " + strings.ToLower(name) + " · Enter next"
		if name == "Crew" && len(m.crews) == 0 {
			hint = "No crews yet · Enter next"
		}
		lines[hintY] = lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(fit(hint, width))
	}

	primary := "Save"
	firstAction := len(e.fields)
	if e.kind == "reply" {
		primary = "Send"
	}
	if e.kind == "delete" {
		primary, firstAction = "Delete", 1
	}
	for i, label := range []string{primary, "Cancel"} {
		button := editorButton(label, e.focus == firstAction+i, e.kind == "delete" && i == 0)
		x := lipgloss.Width(lines[buttonY])
		if i > 0 {
			gap := max(1, min(3, width-x-lipgloss.Width(button)))
			lines[buttonY] += strings.Repeat(" ", gap)
			x += gap
		}
		lines[buttonY] += button
		targets = append(targets, editorTarget{x: x, y: buttonY, width: lipgloss.Width(button), focus: firstAction + i, action: true})
	}
	return lines, targets
}

func (m Model) editorView(height int) string {
	lines, _ := m.editorLayout(height)
	return strings.Join(lines, "\n")
}

func (m Model) editorMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionPress {
		return m, nil
	}
	x, y := msg.X-m.listWidth()-3, m.panelContentY(msg.Y)
	if y < 0 || y >= m.contentHeight() {
		return m, nil
	}
	_, targets := m.editorLayout(m.contentHeight())
	for _, target := range targets {
		if y != target.y || x < target.x || x >= target.x+target.width {
			continue
		}
		e := *m.editor
		e.fields = append([]textinput.Model(nil), e.fields...)
		for i := range e.fields {
			e.fields[i].Blur()
		}
		e.focus = target.focus
		m.editor = &e
		if target.selectorStep != 0 {
			e.changeSelection(m.crews, target.selectorStep)
			return m, nil
		}
		if target.action {
			return m.updateEditor(tea.KeyMsg{Type: tea.KeyEnter})
		}
		return m, e.focusField()
	}
	return m, nil
}
