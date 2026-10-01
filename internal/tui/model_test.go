package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/thomashartm/motley/internal/member"
	"github.com/thomashartm/motley/internal/tmux"
)

func update(m Model, msg tea.Msg) Model { next, _ := m.Update(msg); return next.(Model) }
func key(s string) tea.KeyMsg           { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }
func row(id string, alive bool) member.Row {
	return member.Row{Manifest: member.Manifest{ID: id, Name: id, Repo: "api", Branch: "feat/" + id, Worktree: "/tmp/trees/" + id, Agent: "claude"}, Alive: alive}
}

func TestSelectionSurvivesRefresh(t *testing.T) {
	m := update(newModel(false, true, "client", nil), tea.WindowSizeMsg{Width: 100, Height: 24})
	m = update(m, snapshot{rows: []member.Row{row("b", true), row("dead", false), row("a", true)}})
	if m.selectedID() != "a" {
		t.Fatal("live rows must sort first by id")
	}
	m = update(m, key("j"))
	if m.selectedID() != "b" || !strings.Contains(m.detail.View(), "feat/b") {
		t.Fatal("selection/detail did not move")
	}
	m = update(m, snapshot{rows: []member.Row{row("aa", true), row("b", false), row("a", true)}})
	if m.selectedID() != "b" {
		t.Fatal("selection identity was lost on insertion or state change")
	}
	m = update(m, snapshot{rows: []member.Row{row("a", true)}})
	if m.selectedID() != "a" {
		t.Fatal("selection not clamped after removal")
	}
	m = update(m, snapshot{err: errors.New("server unavailable")})
	if m.selectedID() != "a" || m.pollError != "server unavailable" {
		t.Fatal("failed refresh must preserve last snapshot and show error")
	}
	m = update(m, snapshot{})
	if m.selectedID() != "" || m.pollError != "" || !strings.Contains(m.View(), "No members yet") {
		t.Fatal("empty state")
	}
}
func TestJumpRoutesAndRefusals(t *testing.T) {
	m := newModel(false, false, "", nil)
	m = update(m, snapshot{rows: []member.Row{row("a", false)}})
	m = update(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.attachID != "" || !strings.Contains(m.message, "dead") {
		t.Fatal("dead member should not attach")
	}
	m = update(m, snapshot{rows: []member.Row{row("a", true)}})
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.attachID != "a" {
		t.Fatal("outside jump must attach after restoring terminal")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("must exit alternate screen before attach")
	}
	m.monitor = true
	m.attachID = ""
	m = update(m, tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(m.message, "no longer attached") || m.attachID != "" {
		t.Fatal("disconnected monitor must stay put")
	}
}
func TestMonitorTargetAndPin(t *testing.T) {
	clients := []tmux.Client{
		{Name: "monitor", Session: tmux.MonitorSession, Activity: 1000},
		{Name: "older", Session: "a", Activity: 10}, {Name: "recent", Session: "b", Activity: 20},
	}
	target, err := WorkClient(clients, "")
	if err != nil || target.Name != "recent" {
		t.Fatalf("automatic target: %+v %v", target, err)
	}
	target, err = WorkClient(clients, "older")
	if err != nil || target.Name != "older" {
		t.Fatalf("pinned target: %+v %v", target, err)
	}
	for _, pin := range []string{"monitor", "disconnected"} {
		if _, err := WorkClient(clients, pin); err == nil {
			t.Fatalf("unsafe pin %s accepted", pin)
		}
	}
	m := newModel(true, true, "", nil)
	m = update(m, snapshot{clients: clients})
	m = update(m, key("T"))
	m = update(m, key("j"))
	m = update(m, key("j"))
	m = update(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.pinned != "older" || m.picking {
		t.Fatal("pin picker did not choose client")
	}
	m = update(m, key("T"))
	m = update(m, key("k"))
	m = update(m, key("k"))
	m = update(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.pinned != "" {
		t.Fatal("automatic choice did not unpin")
	}
}
func TestViewFitsTerminal(t *testing.T) {
	for _, size := range [][2]int{{60, 10}, {80, 24}, {140, 40}} {
		m := update(newModel(false, true, "client", nil), tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		rows := make([]member.Row, 50)
		for i := range rows {
			rows[i] = row(strings.Repeat("long界", 20), true)
		}
		m = update(m, snapshot{rows: rows})
		m.selected = 49
		view := m.View()
		if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
			t.Fatalf("view overflow at %v: %dx%d", size, lipgloss.Width(view), lipgloss.Height(view))
		}
	}
}

func TestOpenClientUsesMonitorWithoutWorkTab(t *testing.T) {
	monitor := tmux.Client{Name: "monitor", Session: tmux.MonitorSession}
	target, err := openClient([]tmux.Client{monitor}, "")
	if err != nil || target.Name != "monitor" {
		t.Fatalf("one-tab target: %+v %v", target, err)
	}
	work := tmux.Client{Name: "work", Session: "agent"}
	target, err = openClient([]tmux.Client{monitor, work}, "")
	if err != nil || target.Name != "work" {
		t.Fatalf("separate work target: %+v %v", target, err)
	}
	if _, err = openClient([]tmux.Client{monitor}, "missing"); err == nil {
		t.Fatal("lost pin silently switched monitor")
	}
	if _, err = openClient([]tmux.Client{monitor, {Name: "second", Session: tmux.MonitorSession}}, ""); err == nil {
		t.Fatal("ambiguous monitor client")
	}
	m := update(newModel(true, true, "", nil), snapshot{rows: []member.Row{row("a", true)}, clients: []tmux.Client{monitor}})
	for _, input := range []tea.KeyMsg{{Type: tea.KeyEnter}, key("o")} {
		next, cmd := m.Update(input)
		if !next.(Model).busy || cmd == nil {
			t.Fatal("open action did not switch single monitor tab")
		}
	}
}
