package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/thomashartm/motley/internal/member"
)

func TestImportPicker(t *testing.T) {
	m := update(newModel(true, true, "", nil), tea.WindowSizeMsg{Width: 100, Height: 25})
	next, cmd := m.Update(key("a"))
	m = next.(Model)
	if m.menu == nil || cmd != nil {
		t.Fatal("provider chooser missing")
	}
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil || m.importing == nil || !m.busy {
		t.Fatal("discovery did not start")
	}
	m = update(m, importLoaded{agent: "claude", sessions: []member.ImportCandidate{{SessionID: "one", Name: "First", Cwd: "/repo"}, {SessionID: "two", Name: "Second", Cwd: "/other"}}})
	m = update(m, tea.KeyMsg{Type: tea.KeyDown})
	if m.importing.cursor != 1 || !strings.Contains(m.View(), "> Second") {
		t.Fatal("picker target missing")
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.importing != nil || m.busy {
		t.Fatal("cancel failed")
	}
	m = update(m, importLoaded{agent: "claude", sessions: []member.ImportCandidate{{SessionID: "one", Name: "First", Cwd: "/repo"}}})
	next, cmd = m.Update(tea.MouseMsg{X: m.listWidth() + 4, Y: 3 + m.panelHeadingGap(), Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
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

func TestCodexProviderAndImportMouse(t *testing.T) {
	m := update(newModel(false, false, "", nil), tea.WindowSizeMsg{Width: 100, Height: 25})
	m = update(m, key("a"))
	// Provider menu rows: title, blank, Claude, Codex.
	next, cmd := m.Update(tea.MouseMsg{X: m.listWidth() + 5, Y: 5 + m.panelHeadingGap(), Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = next.(Model)
	if cmd == nil || m.importing == nil || m.importing.agent != "codex" {
		t.Fatal("Codex provider click missed")
	}
	m = update(m, importLoaded{agent: "codex", sessions: []member.ImportCandidate{{Agent: "codex", SessionID: "one", Name: "First", Cwd: "/repo"}, {Agent: "codex", SessionID: "two", Name: "Second", Cwd: "/other"}}})
	next, cmd = m.Update(tea.MouseMsg{X: m.listWidth() + 4, Y: 5 + m.panelHeadingGap(), Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = next.(Model)
	if cmd == nil || !m.busy || m.importing.cursor != 1 {
		t.Fatal("Codex session click missed")
	}
	m = update(m, importDone{member: member.Manifest{ID: "codex-two", Agent: "codex", CodexSession: "two", Name: "Second"}})
	if m.importing != nil || m.focusID != "codex-two" || !strings.Contains(m.message, "existing Codex conversation") {
		t.Fatal("Codex success not presented")
	}
}

func TestImportedCodexActionsPreserveSharedWork(t *testing.T) {
	r := row("codex-existing", true)
	r.Agent = "codex"
	r.CodexSession = "one"
	r.CodexSocket = "/server.sock"
	r.External = true
	m := update(newModel(false, false, "", nil), tea.WindowSizeMsg{Width: 100, Height: 25})
	m = update(m, snapshot{rows: []member.Row{r}})
	for _, a := range m.actions() {
		if a.key == "d" {
			t.Fatal("shared-server termination offered")
		}
		if a.key == "x" && (!strings.Contains(a.description, "running work") || strings.Contains(a.description, "stops")) {
			t.Fatal("retirement impact is misleading")
		}
	}
	next, cmd := m.Update(key("d"))
	if next.(Model).terminating != nil || cmd != nil || !strings.Contains(next.(Model).message, "shared server") {
		t.Fatal("terminate shortcut offered to kill shared work")
	}
	m.retiring = &retireDialog{id: r.ID, loaded: true, check: member.RetireCheck{Manifest: r.Manifest}}
	if len(m.retireChoices()) != 2 || !strings.Contains(m.retireView(m.contentHeight()), "running work") {
		t.Fatal("Codex retirement explanation missing")
	}
	m.retiring = nil
	next, _ = m.Update(key("o"))
	if next.(Model).attachID != r.ID {
		t.Fatal("Codex cannot be opened from an external session")
	}
}
