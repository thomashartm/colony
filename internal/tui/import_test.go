package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/thomashartm/motley/internal/agents/claude"
	"github.com/thomashartm/motley/internal/member"
)

func TestImportPicker(t *testing.T) {
	m := update(newModel(true, true, "", nil), tea.WindowSizeMsg{Width: 100, Height: 25})
	next, cmd := m.Update(key("a"))
	m = next.(Model)
	if cmd == nil || m.importing == nil || !m.busy {
		t.Fatal("discovery did not start")
	}
	m = update(m, importLoaded{sessions: []claude.Session{{SessionID: "one", Name: "First", Cwd: "/repo"}, {SessionID: "two", Name: "Second", Cwd: "/other"}}})
	m = update(m, tea.KeyMsg{Type: tea.KeyDown})
	if m.importing.cursor != 1 || !strings.Contains(m.View(), "> Second") {
		t.Fatal("picker target missing")
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.importing != nil || m.busy {
		t.Fatal("cancel failed")
	}
	m = update(m, importLoaded{sessions: []claude.Session{{SessionID: "one", Name: "First", Cwd: "/repo"}}})
	next, cmd = m.Update(tea.MouseMsg{X: m.listWidth() + 4, Y: 3, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if cmd == nil || !next.(Model).busy {
		t.Fatal("click did not import")
	}
	m = update(next.(Model), importDone{member: member.Manifest{ID: "claude-one", Name: "First"}})
	if m.importing != nil || m.busy || m.focusID != "claude-one" {
		t.Fatal("import did not focus result")
	}
	m = update(m, importLoaded{err: errors.New("Claude discovery unavailable")})
	if m.importing != nil || m.message != "Claude discovery unavailable" {
		t.Fatal("discovery failure hidden")
	}
}

func TestImportedMemberActions(t *testing.T) {
	r := row("imported", true)
	r.ClaudeSession = "session"
	r.External = true
	m := update(newModel(true, true, "", nil), tea.WindowSizeMsg{Width: 100, Height: 25})
	m = update(m, snapshot{rows: []member.Row{r}})
	for _, a := range m.actions() {
		if a.key == "i" || a.key == "t" {
			t.Fatal("unavailable terminal action offered")
		}
	}
	m = update(m, key("i"))
	if m.editor != nil || !strings.Contains(m.message, "original terminal") {
		t.Fatal("external reply not explained")
	}
	m = update(m, key("t"))
	if m.picking {
		t.Fatal("external send offered")
	}
	m.retiring = &retireDialog{id: r.ID, loaded: true, check: member.RetireCheck{Manifest: r.Manifest}}
	if len(m.retireChoices()) != 2 || !strings.Contains(m.View(), "Keeps the checkout") {
		t.Fatal("import retirement could imply deleting checkout")
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyDown})
	m = update(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.retiring != nil || m.busy {
		t.Fatal("import retirement cancel")
	}
}
