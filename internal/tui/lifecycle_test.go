package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/thomashartm/motley/internal/member"
)

func TestRetireDialogRequiresConfirmationAndExplicitForce(t *testing.T) {
	m := update(newModel(false, true, "client", nil), tea.WindowSizeMsg{Width: 100, Height: 25})
	m = update(m, snapshot{rows: []member.Row{row("a", true), row("b", false)}})
	next, cmd := m.Update(key("x"))
	m = next.(Model)
	if cmd == nil || !m.busy || m.retiring == nil {
		t.Fatal("x did not start prechecks")
	}
	m = update(m, retireChecked{id: "a", check: member.RetireCheck{Manifest: m.rows[0].Manifest, Dirty: true, Ahead: 2}})
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd != nil || m.busy || !strings.Contains(m.message, "discarded") {
		t.Fatal("dirty retirement proceeded without force")
	}
	m = update(m, key("f"))
	m = update(m, key("k"))
	if !m.retiring.force || !m.retiring.keep {
		t.Fatal("retirement choices missing")
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.retiring != nil || m.busy {
		t.Fatal("cancel did not return to overview")
	}
	m = update(m, key("x"))
	m = update(m, retireChecked{id: "a", err: errors.New("main worktree")})
	m = update(m, key("f"))
	_, cmd = m.Update(key("y"))
	if cmd != nil {
		t.Fatal("force bypassed structural refusal")
	}
}
func TestReviveOnlyDeadAndLifecycleFailure(t *testing.T) {
	m := update(newModel(false, true, "client", nil), snapshot{rows: []member.Row{row("a", true), row("b", false)}})
	next, cmd := m.Update(key("r"))
	m = next.(Model)
	if cmd != nil || !strings.Contains(m.message, "dead member") {
		t.Fatal("live revive not refused")
	}
	m = update(m, key("j"))
	next, cmd = m.Update(key("r"))
	m = next.(Model)
	if cmd == nil || !m.busy {
		t.Fatal("dead revive not started")
	}
	m = update(m, lifecycleDone{id: "b", action: "Revived", err: errors.New("worktree missing")})
	if m.busy || m.message != "worktree missing" {
		t.Fatal("lifecycle error not presented")
	}
}
