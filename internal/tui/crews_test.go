package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/thomashartm/colony/internal/crew"
	"github.com/thomashartm/colony/internal/minion"
)

func crewSnapshot() snapshot {
	a, b, c, d := row("waiting", true), row("busy", true), row("ungrouped", true), row("dead", false)
	a.Crew, a.Status, a.Since, a.Ticket = "fx", "permission", 10, "433"
	b.Crew, b.Status, b.Repo, b.Since = "fx", "working", "worker", 20
	d.Crew = "inactive"
	return snapshot{rows: []minion.Row{b, c, d, a}, crews: []crew.Crew{
		{ID: "inactive", Title: "Archived work", Color: "grey"},
		{ID: "fx", Title: "FX Banking", Color: "blue", URL: "https://example.com/work"},
	}}
}
func TestCrewNavigationAndRefresh(t *testing.T) {
	m := update(newModel(false, false, "", nil), tea.WindowSizeMsg{Width: 120, Height: 30})
	m = update(m, crewSnapshot())
	m = update(m, key("g"))
	if m.group != "crew" || len(m.crewEntries()) != 2 || m.currentEntry().crew != "fx" || m.selectedID() != "" {
		t.Fatalf("collapsed crews: %+v", m.crewEntries())
	}
	if !strings.Contains(m.crewTable(20, 73), "MINION") || !strings.Contains(m.crewTable(20, 73), "waiting") {
		t.Fatal("missing member table")
	}
	m = update(m, key("H"))
	if len(m.crewEntries()) != 3 || m.currentEntry().crew != "fx" {
		t.Fatal("hidden crew insertion lost selection")
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyTab})
	if m.selectedID() != "waiting" {
		t.Fatal("attention member must be first")
	}
	m = update(m, key("j"))
	if m.selectedID() != "busy" {
		t.Fatal("member navigation")
	}
	// A status transition reorders members, preserving the selected identity.
	snap := crewSnapshot()
	snap.rows[0].Status = "question"
	snap.rows[0].Since = 5
	m = update(m, snap)
	if m.selectedID() != "busy" || m.tableCursor != 0 {
		t.Fatal("refresh lost member identity")
	}
	m = update(m, key("e"))
	if m.editor == nil || m.editor.id != "busy" {
		t.Fatal("member edit target")
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyEsc})
	m = update(m, key("x"))
	if m.retiring == nil || m.retiring.id != "busy" || !strings.Contains(m.View(), "confirm") {
		t.Fatal("member retire target/dialog")
	}
	m.busy = false
	m = update(m, tea.KeyMsg{Type: tea.KeyEsc})
	m = update(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.attachID != "busy" {
		t.Fatal("member jump target")
	}
	m.attachID = ""
	m = update(m, tea.KeyMsg{Type: tea.KeyEsc})
	m = update(m, tea.KeyMsg{Type: tea.KeyRight})
	if len(m.crewEntries()) != 5 {
		t.Fatal("expand crew")
	}
	m = update(m, key("j"))
	if m.selectedID() != "busy" || !strings.Contains(m.detail.View(), "feat/busy") {
		t.Fatal("expanded member detail")
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyLeft})
	if m.currentEntry().crew != "fx" || m.selectedID() != "" || len(m.crewEntries()) != 3 {
		t.Fatal("collapse to parent")
	}
	m = update(m, key("g"))
	if m.group != "repo" || m.rows[len(m.rows)-1].Repo != "worker" {
		t.Fatal("repo grouping")
	}
	m = update(m, key("g"))
	if m.group != "attention" || m.rows[0].ID != "busy" {
		t.Fatal("attention grouping")
	}
}
func TestCrewViewsFitAndColumnsShrink(t *testing.T) {
	for _, size := range [][2]int{{60, 10}, {80, 24}, {140, 40}} {
		m := update(newModel(false, true, "client", nil), tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		snap := crewSnapshot()
		for i := 0; i < 50; i++ {
			r := row(strings.Repeat("long界", 20), true)
			r.Crew = "fx"
			snap.rows = append(snap.rows, r)
		}
		m = update(m, snap)
		m = update(m, key("g"))
		m.tableFocus = true
		m.tableCursor = 50
		views := []string{m.View()}
		m = update(m, key("e"))
		views = append(views, m.View())
		m = update(m, tea.KeyMsg{Type: tea.KeyEsc})
		m = update(m, key("G"))
		views = append(views, m.View())
		m = update(m, key("a"))
		views = append(views, m.View())
		for _, view := range views {
			if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
				t.Fatalf("crew view overflow at %v: %dx%d", size, lipgloss.Width(view), lipgloss.Height(view))
			}
		}
	}
	m := update(newModel(false, false, "", nil), crewSnapshot())
	m = update(m, key("g"))
	for _, c := range []struct {
		width         int
		branch, since bool
	}{{35, false, false}, {60, true, false}, {80, true, true}} {
		table := m.crewTable(10, c.width)
		if strings.Contains(table, "BRANCH") != c.branch || strings.Contains(table, "SINCE") != c.since {
			t.Fatalf("columns at %d: %s", c.width, table)
		}
	}
}
