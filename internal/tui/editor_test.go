package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/thomashartm/motley/internal/crew"
	"github.com/thomashartm/motley/internal/palette"
)

func editorModel(width, height int) Model {
	m := actionModel(width, height)
	return update(m, key("e"))
}

func editorClickText(t *testing.T, m Model, text string) (tea.Model, tea.Cmd) {
	t.Helper()
	// Use the rendered panel rather than the layout's hit targets.
	view := ansi.Strip(panelHeading(m.editorView(m.contentHeight()), m.detailWidth(), m.panelHeadingGap()))
	for y, line := range strings.Split(view, "\n") {
		if x := strings.Index(line, text); x >= 0 {
			return m.Update(tea.MouseMsg{X: m.listWidth() + 3 + ansi.StringWidth(line[:x]), Y: y + 2, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		}
	}
	t.Fatalf("control %q not visible:\n%s", text, view)
	return m, nil
}

func TestEditorLabelsSpacingAndButtons(t *testing.T) {
	m := editorModel(120, 30)
	lines := strings.Split(ansi.Strip(m.editorView(m.contentHeight())), "\n")
	for _, label := range m.editor.labels {
		found := false
		for y, line := range lines {
			if line != label+":" {
				continue
			}
			found = true
			if !strings.HasPrefix(lines[y+1], "  ") || strings.TrimSpace(lines[y+2]) != "" {
				t.Fatalf("label/value/spacing unclear for %s", label)
			}
		}
		if !found {
			t.Fatalf("label missing: %s", label)
		}
	}
	buttons := lines[len(lines)-1]
	if !strings.Contains(buttons, "[ Save ]") || !strings.Contains(buttons, "   ") || !strings.Contains(buttons, "[ Cancel ]") {
		t.Fatal("buttons missing or crowded", buttons)
	}
	if lines[len(lines)-2] != "" {
		t.Fatal("no space above buttons")
	}
}

func TestEditorMouseFieldsAndButtons(t *testing.T) {
	m := editorModel(120, 30)
	next, _ := editorClickText(t, m, "Ticket:")
	m = next.(Model)
	if m.editor.focus != 1 || !m.editor.fields[1].Focused() || m.editor.fields[0].Focused() {
		t.Fatal("click did not focus the field")
	}
	m = update(m, key("42"))
	if m.editor.fields[1].Value() != "42" {
		t.Fatal("typing missed selected field")
	}
	next, cmd := editorClickText(t, m, "[ Save ]")
	if cmd == nil || !next.(Model).busy {
		t.Fatal("Save click did not dispatch")
	}
	// Busy editors ignore clicks until the pending write finishes.
	busy := next.(Model)
	next, cmd = editorClickText(t, busy, "[ Cancel ]")
	if cmd != nil || next.(Model).editor == nil {
		t.Fatal("click escaped a pending save")
	}
	next, cmd = editorClickText(t, m, "[ Cancel ]")
	if cmd != nil || next.(Model).editor != nil {
		t.Fatal("Cancel click did not close without saving")
	}
}

func TestEditorMouseDeleteAndReply(t *testing.T) {
	m := editorModel(100, 25)
	m.editor = newEditor("delete", "crew", nil, nil)
	next, cmd := editorClickText(t, m, "[ Force unassign: off ]")
	m = next.(Model)
	if cmd != nil || !m.editor.force || m.busy {
		t.Fatal("force click did not toggle safely")
	}
	next, cmd = editorClickText(t, m, "[ Delete ]")
	if cmd == nil || !next.(Model).busy {
		t.Fatal("Delete did not dispatch")
	}
	m.editor = newEditor("reply", "alpha", []string{"Reply"}, []string{"Hello"})
	next, cmd = editorClickText(t, m, "[ Send ]")
	if cmd == nil || !next.(Model).busy {
		t.Fatal("Send did not dispatch")
	}
}

func TestEditorResizeKeepsFieldAndButtonsReachable(t *testing.T) {
	for _, size := range [][2]int{{60, 10}, {80, 20}, {120, 30}} {
		m := editorModel(size[0], size[1])
		for i := 0; i < len(m.editor.fields)+2; i++ {
			view := m.View()
			if lipgloss.Width(view) > m.width || lipgloss.Height(view) > m.height {
				t.Fatalf("editor exceeds %dx%d", m.width, m.height)
			}
			if i < len(m.editor.fields) {
				if !strings.Contains(ansi.Strip(view), m.editor.labels[i]+":") {
					t.Fatalf("focused label not visible: %s", m.editor.labels[i])
				}
			}
			if !strings.Contains(ansi.Strip(view), "[ Save ]") || !strings.Contains(ansi.Strip(view), "[ Cancel ]") {
				t.Fatal("editor buttons are not visible")
			}
			m = arrow(m, tea.KeyDown)
		}
		next, cmd := editorClickText(t, m, "[ Cancel ]")
		if cmd != nil || next.(Model).editor != nil {
			t.Fatal("resized Cancel target missed")
		}
	}
}

func TestColourSelectorCyclesAndRejectsTyping(t *testing.T) {
	m := editorModel(100, 25)
	for i := 0; i < 3; i++ {
		m = arrow(m, tea.KeyDown)
	}
	if !strings.Contains(ansi.Strip(m.View()), "◇ Inherit") {
		t.Fatal("inherit choice missing")
	}
	for _, field := range m.editor.fields {
		if field.Focused() {
			t.Fatal("colour selector still has a text cursor")
		}
	}
	for _, colour := range palette.Colors {
		m = arrow(m, tea.KeyRight)
		if m.editor.fields[3].Value() != colour.Name || !strings.Contains(ansi.Strip(m.View()), "■ "+colour.Name) {
			t.Fatalf("colour choice or swatch missing: %s", colour.Name)
		}
	}
	m = arrow(m, tea.KeyRight)
	if m.editor.fields[3].Value() != "" {
		t.Fatal("right did not wrap to inherit")
	}
	m = arrow(m, tea.KeyLeft)
	if m.editor.fields[3].Value() != "grey" {
		t.Fatal("left did not wrap to grey")
	}
	for _, msg := range []tea.Msg{key("invalid"), tea.KeyMsg{Type: tea.KeyBackspace}, tea.KeyMsg{Type: tea.KeyDelete}, key("y")} {
		m = update(m, msg)
	}
	if m.editor.fields[3].Value() != "grey" || m.busy {
		t.Fatal("text input changed or submitted the colour")
	}
	m = arrow(m, tea.KeyEnter)
	if m.editor.focus != 4 || m.busy {
		t.Fatal("Enter did not advance to Save")
	}
}

func TestColourSelectorCrewDefaultsAndMouse(t *testing.T) {
	for _, kind := range []string{"add", "crew"} {
		m := editorModel(100, 25)
		m.editor = newEditor(kind, "team", []string{"Title", "URL", "Colour", "Gig"}, []string{"Team", "", "", ""})
		for i := 0; i < 2; i++ {
			m = arrow(m, tea.KeyDown)
		}
		if !strings.Contains(ansi.Strip(m.View()), "◇ Automatic") {
			t.Fatal("automatic crew colour missing")
		}
		next, cmd := editorClickText(t, m, "›")
		m = next.(Model)
		if cmd != nil || m.editor.fields[2].Value() != "red" || m.busy {
			t.Fatal("right arrow click did not select red")
		}
		next, cmd = editorClickText(t, m, "‹")
		m = next.(Model)
		if cmd != nil || m.editor.fields[2].Value() != "" {
			t.Fatal("left arrow click did not restore automatic")
		}
		m.editor.fields[2].SetValue("blue")
		m = arrow(m, tea.KeyLeft)
		if m.editor.fields[2].Value() != "green" {
			t.Fatal("selector did not start from saved colour")
		}
		m = update(m, tea.WindowSizeMsg{Width: 60, Height: 10})
		assertFooterFits(t, m)
		if !strings.Contains(ansi.Strip(m.View()), "■ green") {
			t.Fatal("compact selector hidden")
		}
	}
}

func TestCrewSelectorUsesNamesAndStoredIDs(t *testing.T) {
	m := editorModel(100, 25)
	m.crews = []crew.Crew{{ID: "crew-42", Title: "Payments", Color: "blue"}, {ID: "crew-77", Title: "Support", Color: "green"}}
	m = arrow(arrow(m, tea.KeyDown), tea.KeyDown)
	if !strings.Contains(ansi.Strip(m.View()), "No crew") {
		t.Fatal("No crew option missing")
	}
	for _, want := range []string{"crew-42", "crew-77", "", "crew-42"} {
		m = arrow(m, tea.KeyRight)
		if m.editor.fields[2].Value() != want {
			t.Fatalf("wrong stored crew: %q", m.editor.fields[2].Value())
		}
		if want != "" {
			c, _ := crew.Find(m.crews, want)
			if !strings.Contains(ansi.Strip(m.View()), "■ "+c.Title) {
				t.Fatal("crew title missing")
			}
		}
	}
	if m.editor.fields[2].Focused() {
		t.Fatal("crew selector has a text cursor")
	}
	for _, msg := range []tea.Msg{key("other-id"), tea.KeyMsg{Type: tea.KeyBackspace}, tea.KeyMsg{Type: tea.KeyDelete}} {
		m = update(m, msg)
	}
	if m.editor.fields[2].Value() != "crew-42" {
		t.Fatal("typing changed selected crew")
	}
	// Refresh and reorder while editing: preserve the ID, not the old list position.
	m = update(m, snapshot{rows: m.allRows, crews: []crew.Crew{m.crews[1], m.crews[0]}})
	if m.editor.fields[2].Value() != "crew-42" {
		t.Fatal("refresh changed selection")
	}
	m = arrow(m, tea.KeyRight)
	if m.editor.fields[2].Value() != "" {
		t.Fatal("existing selection was not located by ID")
	}
	m = arrow(m, tea.KeyLeft)
	if m.editor.fields[2].Value() != "crew-42" {
		t.Fatal("left wrap missed last crew")
	}
	m = arrow(m, tea.KeyEnter)
	if m.editor.focus != 3 || m.busy {
		t.Fatal("Enter did not advance to Colour")
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.editor != nil || m.selectedRow().Crew != "" {
		t.Fatal("cancel saved an assignment")
	}
}

func TestCrewSelectorEmptyAndRemovedCrews(t *testing.T) {
	m := editorModel(100, 25)
	m = arrow(arrow(m, tea.KeyDown), tea.KeyDown)
	for _, direction := range []tea.KeyType{tea.KeyLeft, tea.KeyRight} {
		m = arrow(m, direction)
	}
	if m.editor.fields[2].Value() != "" || !strings.Contains(ansi.Strip(m.View()), "No crews yet") {
		t.Fatal("empty crew list is not explained")
	}
	m.editor.fields[2].SetValue("removed")
	if !strings.Contains(ansi.Strip(m.View()), "Unavailable crew") {
		t.Fatal("missing assignment hidden")
	}
	if m.editor.fields[2].Value() != "removed" {
		t.Fatal("rendering silently cleared assignment")
	}
	m = arrow(m, tea.KeyRight)
	if m.editor.fields[2].Value() != "" {
		t.Fatal("cannot clear unavailable crew")
	}
}

func TestCrewSelectorMouseAndLongNames(t *testing.T) {
	for _, width := range []int{60, 80, 120} {
		m := editorModel(width, 25)
		m.crews = []crew.Crew{{ID: "long-id", Title: strings.Repeat("支払", 20), Color: "blue"}}
		m = arrow(arrow(m, tea.KeyDown), tea.KeyDown)
		next, cmd := editorClickText(t, m, "›")
		m = next.(Model)
		if cmd != nil || m.editor.fields[2].Value() != "long-id" {
			t.Fatal("right arrow did not select crew")
		}
		assertFooterFits(t, m)
		next, cmd = editorClickText(t, m, "‹")
		m = next.(Model)
		if cmd != nil || m.editor.fields[2].Value() != "" {
			t.Fatal("left arrow did not clear assignment")
		}
		m = update(m, tea.WindowSizeMsg{Width: 60, Height: 10})
		assertFooterFits(t, m)
		next, _ = editorClickText(t, m, "›")
		if next.(Model).editor.fields[2].Value() != "long-id" {
			t.Fatal("compact arrow target missed")
		}
	}
}
