package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/thomashartm/motley/internal/member"
)

func arrow(m Model, k tea.KeyType) Model { return update(m, tea.KeyMsg{Type: k}) }

func TestArrowPanelsAndEditor(t *testing.T) {
	m := update(newModel(false, false, "", nil), tea.WindowSizeMsg{Width: 60, Height: 10})
	snap := snapshot{rows: []member.Row{row("alpha", true), row("beta", true)}}
	m = update(m, snap)
	m = arrow(m, tea.KeyRight)
	m = arrow(m, tea.KeyDown)
	if m.panel != detailPanel || m.selectedID() != "alpha" {
		t.Fatal("detail scroll moved list selection")
	}
	m = arrow(m, tea.KeyRight)
	if !strings.Contains(m.View(), "> Open agent (o)") {
		t.Fatal("actions selection invisible")
	}
	m = update(m, snap)
	if m.panel != actionsPanel {
		t.Fatal("refresh lost focus")
	}
	m = arrow(m, tea.KeyDown)
	m = arrow(m, tea.KeyEnter)
	if m.editor == nil || m.editor.id != "alpha" {
		t.Fatal("arrow route did not open editor")
	}
	pos := m.editor.fields[0].Position()
	m = arrow(m, tea.KeyLeft)
	if m.editor.fields[0].Position() != pos-1 || m.panel != actionsPanel {
		t.Fatal("left did not move caret")
	}
	m = update(m, key("y"))
	if m.busy || m.editor.fields[0].Value() != "alphya" {
		t.Fatal("shortcut intercepted text")
	}
	for i := 0; i < 4; i++ {
		m = arrow(m, tea.KeyDown)
	}
	if !strings.Contains(m.View(), "> Save") {
		t.Fatal("save control invisible at minimum size")
	}
	view := m.View()
	if lipgloss.Height(view) > 10 || lipgloss.Width(view) > 60 {
		t.Fatal("view exceeds terminal")
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !next.(Model).busy || cmd == nil {
		t.Fatal("save did not dispatch")
	}
	// Cancel is reachable without saving or using a letter shortcut.
	m = arrow(m, tea.KeyDown)
	if !strings.Contains(m.View(), "> Cancel") {
		t.Fatal("cancel control invisible")
	}
	m = arrow(m, tea.KeyEnter)
	if m.editor != nil || m.busy {
		t.Fatal("cancel saved")
	}
	m = arrow(m, tea.KeyLeft)
	m = arrow(m, tea.KeyEsc)
	if m.panel != listPanel {
		t.Fatal("back did not restore list")
	}
}

func TestArrowCrewAndEmptyNavigation(t *testing.T) {
	m := update(newModel(true, true, "monitor", nil), crewSnapshot())
	m = update(m, key("g"))
	m = arrow(m, tea.KeyRight)
	if !m.expanded["fx"] || m.panel != listPanel {
		t.Fatal("first right must expand crew")
	}
	m = arrow(m, tea.KeyRight)
	m = arrow(m, tea.KeyDown)
	if m.panel != detailPanel || m.selectedID() != "busy" {
		t.Fatal("crew table selection")
	}
	m = arrow(m, tea.KeyEnter)
	if m.attachID != "" || !strings.Contains(m.message, "no longer attached") {
		t.Fatal("disconnected monitor must refuse opening")
	}
	m = arrow(m, tea.KeyRight)
	m = update(m, crewSnapshot())
	if m.selectedID() != "busy" || m.panel != actionsPanel {
		t.Fatal("refresh lost table target")
	}
	m = update(m, snapshot{})
	if m.selectedID() != "" {
		t.Fatal("removed member remains selected")
	}
	m.actionCursor = 0
	m = arrow(m, tea.KeyEnter)
	if !m.manager {
		t.Fatal("empty overview cannot reach crews")
	}
	m = arrow(m, tea.KeyEnter)
	if m.editor == nil || m.editor.kind != "add" {
		t.Fatal("empty manager cannot add crew")
	}
	m = arrow(m, tea.KeyEsc)
	m = arrow(m, tea.KeyRight)
	m = arrow(m, tea.KeyDown)
	m = arrow(m, tea.KeyLeft)
	if m.managerActions || !m.manager {
		t.Fatal("crew menu back")
	}
	m = arrow(m, tea.KeyEsc)
	if m.manager {
		t.Fatal("manager exit")
	}
}

func TestArrowConfirmationsAndSpawn(t *testing.T) {
	m := newModel(false, false, "", nil)
	m.retiring = &retireDialog{id: "a", loaded: true, check: member.RetireCheck{Dirty: true}}
	m = arrow(m, tea.KeyEnter)
	if m.busy {
		t.Fatal("dirty retirement accepted")
	}
	m = arrow(m, tea.KeyDown)
	m = arrow(m, tea.KeyEnter)
	if !m.retiring.force || m.busy {
		t.Fatal("force toggle executed retirement")
	}
	m = arrow(m, tea.KeyDown)
	m = arrow(m, tea.KeyEnter)
	if !m.retiring.keep {
		t.Fatal("keep toggle")
	}
	m = arrow(m, tea.KeyDown)
	m = arrow(m, tea.KeyEnter)
	if m.retiring != nil || m.busy {
		t.Fatal("retire cancel")
	}
	m.editor = newEditor("delete", "fx", nil, nil)
	m = arrow(m, tea.KeyEnter)
	if !m.editor.force || m.busy {
		t.Fatal("delete force toggle")
	}
	m = arrow(m, tea.KeyDown)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !next.(Model).busy || cmd == nil {
		t.Fatal("delete confirm")
	}
	m = arrow(m, tea.KeyDown)
	m = arrow(m, tea.KeyEnter)
	if m.editor != nil {
		t.Fatal("delete cancel")
	}
	m.spawn = &spawnForm{step: identityStep, fields: inputs("", "Example", "feat/example")}
	m = arrow(m, tea.KeyDown)
	if m.spawn.field != 1 {
		t.Fatal("spawn down field")
	}
	m = arrow(m, tea.KeyUp)
	if m.spawn.field != 0 {
		t.Fatal("spawn up field")
	}
	m.spawn.step = previewStep
	m = arrow(m, tea.KeyRight)
	if m.spawn.previewAction != 1 {
		t.Fatal("prompt edit unreachable")
	}
	m = arrow(m, tea.KeyRight)
	m = arrow(m, tea.KeyEnter)
	if m.spawn != nil || m.busy {
		t.Fatal("preview cancel launched")
	}
}
