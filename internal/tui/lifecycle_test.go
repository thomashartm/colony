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

func TestTerminateSelectedMemberAndCancel(t *testing.T) {
	m := update(newModel(true, true, "client", nil), tea.WindowSizeMsg{Width: 100, Height: 25})
	m = update(m, snapshot{rows: []member.Row{row("alpha", true), row("beta", true)}})
	// Select beta by clicking its list row, then Actions in the bottom bar.
	m = update(m, tea.MouseMsg{X: 5, Y: listScreenY(t, m, "beta"), Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = update(m, tea.MouseMsg{X: strings.Index(m.navigationBar(), "[3 Actions]") + 1, Y: 24, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if m.selectedID() != "beta" || !strings.Contains(m.View(), "Actions: beta") {
		t.Fatal("action target is not selected member")
	}
	index := -1
	for i, a := range m.actions() {
		if a.key == "d" {
			index = i
		}
	}
	if index < 0 {
		t.Fatal("terminate action missing")
	}
	m.actionCursor = index
	next, cmd := m.Update(tea.MouseMsg{X: m.listWidth() + 4, Y: actionScreenY(t, m, "Terminate agent (d)"), Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = next.(Model)
	if cmd != nil || m.terminating == nil || m.terminating.id != "beta" || m.busy {
		t.Fatal("terminate should confirm selected target")
	}
	// Polling can reorder rows without changing the confirmation's target.
	m = update(m, snapshot{rows: []member.Row{row("beta", true), row("alpha", true)}})
	if m.terminating.id != "beta" {
		t.Fatal("confirmation target changed")
	}
	next, cmd = m.Update(tea.MouseMsg{X: strings.Index(m.terminateButtons(), "[Terminate: y]") + 1, Y: 24, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if !next.(Model).busy || cmd == nil {
		t.Fatal("mouse confirmation did not dispatch")
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.terminating != nil || m.busy {
		t.Fatal("default Enter must cancel")
	}
	m = update(m, key("d"))
	m = update(m, tea.KeyMsg{Type: tea.KeyRight})
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !next.(Model).busy || cmd == nil {
		t.Fatal("arrow confirmation did not dispatch")
	}
	m = update(next.(Model), lifecycleDone{id: "beta", action: "Terminated"})
	if m.terminating != nil || m.busy || m.message != "Terminated beta" {
		t.Fatal("termination did not complete")
	}
	m = update(m, snapshot{rows: []member.Row{row("beta", false)}})
	next, cmd = m.Update(key("d"))
	if cmd != nil || next.(Model).terminating != nil {
		t.Fatal("dead member can be terminated")
	}
}

func TestRetireMouseChoices(t *testing.T) {
	m := update(newModel(true, true, "", nil), tea.WindowSizeMsg{Width: 100, Height: 25})
	m.retiring = &retireDialog{id: "a", loaded: true, check: member.RetireCheck{Dirty: true}}
	click := func(index int) (tea.Model, tea.Cmd) {
		first := len(strings.Split(m.retireView(m.contentHeight()), "\n")) - 4
		return m.Update(tea.MouseMsg{X: m.listWidth() + 4, Y: 2 + first + index + m.panelHeadingGap(), Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	}
	next, cmd := click(0)
	m = next.(Model)
	if cmd != nil || m.busy {
		t.Fatal("mouse discarded work without force")
	}
	next, _ = click(1)
	m = next.(Model)
	if !m.retiring.force || m.busy {
		t.Fatal("force click did not toggle")
	}
	next, _ = click(2)
	m = next.(Model)
	if !m.retiring.keep {
		t.Fatal("keep branch click did not toggle")
	}
	next, cmd = click(0)
	if !next.(Model).busy || cmd == nil {
		t.Fatal("confirmed retirement did not dispatch")
	}
	next, cmd = click(3)
	if next.(Model).retiring != nil || cmd != nil {
		t.Fatal("mouse cancel did not cancel")
	}
}
