package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/thomashartm/motley/internal/crew"
	"github.com/thomashartm/motley/internal/member"
)

func TestOverviewSelectionAndActionScope(t *testing.T) {
	for _, grouping := range []string{"attention", "repo", "crew"} {
		t.Run(grouping, func(t *testing.T) {
			m := update(newModel(false, false, "", nil), tea.WindowSizeMsg{Width: 120, Height: 30})
			m.group = grouping
			m = update(m, crewSnapshot())
			m = update(m, key("home"))
			if !m.overview || m.selectedID() != "" || m.panel != actionsPanel {
				t.Fatal("Home did not select main actions")
			}
			keys := map[string]bool{}
			for _, a := range m.actions() {
				keys[a.key] = true
			}
			for _, k := range []string{"s", "a", "o", "m", "g", "/"} {
				if !keys[k] {
					t.Fatal("missing main action", k)
				}
			}
			for _, k := range []string{"e", "r", "d", "x", "i", "t"} {
				if keys[k] {
					t.Fatal("member action in main menu", k)
				}
				candidate, cmd := m.Update(key(k))
				if cmd != nil || candidate.(Model).busy || candidate.(Model).editor != nil || candidate.(Model).retiring != nil || candidate.(Model).terminating != nil {
					t.Fatal("main selection acted on member", k)
				}
			}
			m = update(m, crewSnapshot())
			if !m.overview || m.selectedID() != "" {
				t.Fatal("poll replaced overview selection")
			}
			m = update(m, key("1"))
			m = arrow(m, tea.KeyDown)
			if m.overview {
				t.Fatal("cannot leave overview")
			}
			m = arrow(m, tea.KeyUp)
			if !m.overview {
				t.Fatal("cannot reach overview with Up")
			}
			m = arrow(m, tea.KeyEnter)
			if m.panel != actionsPanel {
				t.Fatal("Overview Enter did not open menu")
			}
			m = update(m, key("1"))
			m = arrow(m, tea.KeyDown)
			m = click(m, 3, 2)
			if !m.overview || m.panel != actionsPanel {
				t.Fatal("top entry click did not open main actions")
			}
		})
	}
	m := actionModel(120, 30)
	for _, a := range m.actions() {
		if a.key == "s" || a.key == "a" || a.key == "m" || a.key == "g" {
			t.Fatal("global action in member menu")
		}
	}
}

func TestOverviewCreateActionsWithAndWithoutMembers(t *testing.T) {
	for _, populated := range []bool{false, true} {
		for _, key := range []string{"s", "a", "m"} {
			m := newModel(false, false, "", nil)
			if populated {
				m = update(m, crewSnapshot())
			}
			m.selectOverview()
			m.panel = actionsPanel
			for i, a := range m.actions() {
				if a.key == key {
					m.actionCursor = i
				}
			}
			next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			got := next.(Model)
			switch key {
			case "s":
				if got.spawn == nil || cmd == nil {
					t.Fatal("spawn unreachable")
				}
			case "a":
				if got.importing == nil || cmd == nil {
					t.Fatal("import unreachable")
				}
			case "m":
				if !got.manager {
					t.Fatal("crew management unreachable")
				}
			}
		}
	}
}

func TestAgentPickerSelectionRefreshAndCancel(t *testing.T) {
	for _, grouping := range []string{"attention", "crew"} {
		m := update(newModel(false, false, "", nil), tea.WindowSizeMsg{Width: 120, Height: 30})
		m.group = grouping
		m = update(m, crewSnapshot())
		m.query.SetValue("waiting")
		m.applyFilter()
		m = update(m, key("home"))
		m = update(m, key("o"))
		if len(m.opening.ids) != 3 || m.opening.ids[0] != "busy" {
			t.Fatal("picker used filtered rows or included dead agents")
		}
		before := m.selectedID()
		m = arrow(m, tea.KeyEsc)
		if m.opening != nil || m.selectedID() != before || m.query.Value() != "waiting" {
			t.Fatal("cancel changed selection or filter")
		}
		m = update(m, key("o"))
		snap := crewSnapshot()
		snap.rows = append([]member.Row{row("inserted", true)}, snap.rows...)
		m = update(m, snap)
		next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		got := next.(Model)
		if got.attachID != "busy" || got.selectedID() != "busy" || got.query.Value() != "" || cmd == nil {
			t.Fatalf("wrong picker target after refresh: %+v", got.opening)
		}
	}
	m := update(newModel(false, false, "", nil), snapshot{rows: []member.Row{row("gone", true)}})
	m = update(m, key("home"))
	m = update(m, key("o"))
	m = update(m, snapshot{})
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || next.(Model).opening == nil || !strings.Contains(next.(Model).message, "no longer running") {
		t.Fatal("stale target opened")
	}
	m = arrow(m, tea.KeyEsc)
	m = update(m, key("o"))
	if !strings.Contains(m.agentPickerView(10), "No running agents") {
		t.Fatal("empty picker missing explanation")
	}
	if next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil || next.(Model).attachID != "" {
		t.Fatal("empty picker opened member")
	}
}

func TestGroupedTableLinksAndMouseTargets(t *testing.T) {
	snap := crewSnapshot()
	a, b := statusRow("alpha", "working", 2), statusRow("beta", "working", 1)
	a.Crew, a.Ticket, a.RemoteURL = "fx", "#42", "git@github.com:owner/repo.git"
	b.Crew, b.Ticket = "other", "https://tracker.example/tickets/71"
	snap.crews = append(snap.crews, crew.Crew{ID: "other", Title: "Operations"})
	snap.rows = append(snap.rows, b, a)
	m := update(newModel(false, false, "", nil), tea.WindowSizeMsg{Width: 180, Height: 45})
	m = update(m, snap)
	view := m.listView(m.listContentHeight(), m.listWidth())
	for _, text := range []string{"TITLE", "TICKET", "CREW", "NEEDS YOU", "WORKING", "FX Banking", "Operations", "No crew", "CC", "●", "⚠", "https://github.com/owner/repo/issues/42", "https://tracker.example/tickets/71"} {
		if !strings.Contains(view, text) {
			t.Fatal("missing table content", text)
		}
	}
	if strings.Index(view, "alpha") > strings.Index(view, "beta") {
		t.Fatal("crew grouping ignored")
	}
	for _, id := range []string{"alpha", "beta", "ungrouped"} {
		m = click(m, 3, listScreenY(t, m, id))
		if m.selectedID() != id {
			t.Fatal("mouse selected wrong row", id, m.selectedID())
		}
		for _, line := range m.listLayout(m.listContentHeight(), m.listWidth()).body {
			if line.index < 0 {
				continue
			}
			marker := "▌ "
			if line.index == m.selected {
				marker = "▌▌"
			}
			if !strings.HasPrefix(ansi.Strip(line.text), marker) {
				t.Fatal("selection did not retain the fixed colour-marker columns")
			}
		}
	}
	for _, heading := range []string{"WORKING", "Operations", "TITLE"} {
		previous := m.selectedID()
		m = click(m, 3, listScreenY(t, m, heading))
		if m.selectedID() != previous {
			t.Fatal("heading selected a member")
		}
	}
}

func TestTableAndOverviewCompactScrolling(t *testing.T) {
	for _, size := range [][2]int{{60, 10}, {80, 12}, {120, 30}} {
		m := update(newModel(false, false, "", nil), tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		snap := crewSnapshot()
		for i := 0; i < 20; i++ {
			r := row(strings.Repeat("支払", 12)+string(rune('a'+i)), true)
			r.Crew = "fx"
			snap.rows = append(snap.rows, r)
		}
		m = update(m, snap)
		for range m.rows {
			view := m.View()
			if lipgloss.Height(view) > m.height || lipgloss.Width(view) > m.width {
				t.Fatalf("table overflow %v", size)
			}
			layout := m.listLayout(m.listContentHeight(), m.listWidth())
			visible := layout.body[layout.start:min(len(layout.body), layout.start+m.listContentHeight()-len(layout.fixed))]
			found := false
			for i, line := range visible {
				if line.index == m.selected {
					found = true
					m = click(m, 3, i+len(layout.fixed)+2+m.listHeadingGap())
					if m.selectedID() == "" {
						t.Fatal("scrolled hit target lost")
					}
				}
			}
			if !found {
				t.Fatal("selected row scrolled out of view")
			}
			m = arrow(m, tea.KeyDown)
		}
		m = click(m, 3, 2)
		if !m.overview {
			t.Fatal("pinned overview inaccessible")
		}
		m = update(m, key("o"))
		if lipgloss.Height(m.View()) > m.height || lipgloss.Width(m.View()) > m.width {
			t.Fatal("picker overflow")
		}
	}
}

func TestTicketLinks(t *testing.T) {
	for _, tc := range []struct{ ticket, remote, want string }{
		{"42", "git@github.com:owner/repo.git", "https://github.com/owner/repo/issues/42"},
		{"#42", "ssh://git@github.com/owner/repo.git", "https://github.com/owner/repo/issues/42"},
		{"42", "https://gitlab.com/team/repo.git", "https://gitlab.com/team/repo/-/issues/42"},
		{"https://tracker.example/ABC-4", "", "https://tracker.example/ABC-4"},
		{"ABC-4", "https://github.com/owner/repo", ""},
		{"42", "https://unknown.example/owner/repo", ""},
		{"42", "https://secret@github.com/owner/repo", ""},
		{"https://example.com/a\x1b]52;c;bad", "", ""},
		{"javascript:alert(1)", "", ""},
	} {
		r := row("a", true)
		r.Ticket, r.RemoteURL = tc.ticket, tc.remote
		label, target := ticketLink(r)
		if target != tc.want || strings.Contains(label, "\x1b") {
			t.Fatalf("ticket %q: %q %q", tc.ticket, label, target)
		}
	}
}

func TestDetailFieldsAlignmentWrappingAndSpacing(t *testing.T) {
	fields := []detailField{{"Status", "working"}, {"Worktree", strings.Repeat("支払", 25)}, {"Ticket", ""}}
	for _, width := range []int{25, 39, 40, 70} {
		text := ansi.Strip(detailFields(fields, width))
		if strings.Contains(text, "\n\n") {
			t.Fatal("unexpected blank row between detail fields")
		}
		for _, line := range strings.Split(text, "\n") {
			if ansi.StringWidth(line) > width {
				t.Fatal("wrapped field overflow", width, line)
			}
		}
		if width >= 40 {
			if !strings.Contains(text, "Status:        working") || !strings.Contains(text, "Ticket:        —") {
				t.Fatal("label columns differ", text)
			}
			if !strings.Contains(text, "\n               ") {
				t.Fatal("wrapped value lost indentation", text)
			}
		} else if !strings.Contains(text, "Status:\n  working") {
			t.Fatal("compact detail labels are not stacked", text)
		}
	}
	m := update(newModel(false, false, "", nil), tea.WindowSizeMsg{Width: 120, Height: 30})
	m = update(m, crewSnapshot())
	details := ansi.Strip(m.memberDetails())
	for _, want := range []string{"Status:", "Ticket:", "Crew:", "Workspace\n┄", "Session\n┄", "Worktree:", "Main repo:"} {
		if !strings.Contains(details, want) {
			t.Fatal("missing detail", want)
		}
	}
}

func TestAgentPickerMouseAndStoppedTarget(t *testing.T) {
	for _, width := range []int{60, 120} {
		m := update(newModel(false, false, "", nil), tea.WindowSizeMsg{Width: width, Height: 20})
		m = update(m, snapshot{rows: []member.Row{row("alpha", true), row("beta", true)}})
		m = update(m, key("home"))
		m = update(m, key("o"))
		y := -1
		for i, line := range strings.Split(ansi.Strip(m.agentPickerView(m.contentHeight())), "\n") {
			if strings.Contains(line, "beta") {
				y = i + 2 + m.panelHeadingGap()
			}
		}
		if y < 0 {
			t.Fatal("picker title is unreadable", width)
		}
		m = click(m, m.listWidth()+4, y)
		if m.attachID != "beta" || m.opening != nil {
			t.Fatal("picker click opened wrong agent", width)
		}
	}
	m := update(newModel(false, false, "", nil), snapshot{rows: []member.Row{row("alpha", true), row("beta", true)}})
	m = update(m, key("home"))
	m = update(m, key("o"))
	m = update(m, snapshot{rows: []member.Row{row("beta", true), row("alpha", false)}})
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil || next.(Model).attachID != "" || !strings.Contains(next.(Model).message, "no longer running") {
		t.Fatal("stopped picker target was opened or replaced")
	}
}

func TestOverviewUpAndEmptyDownKeepMainSelection(t *testing.T) {
	m := update(newModel(false, false, "", nil), snapshot{rows: []member.Row{row("alpha", true), row("beta", true)}})
	m = arrow(m, tea.KeyDown)
	m = update(m, key("home"))
	m = update(m, key("1"))
	m = arrow(m, tea.KeyUp)
	if !m.overview || m.selectedID() != "" {
		t.Fatal("Up on Overview selected a stale member")
	}
	m = update(m, snapshot{})
	m = arrow(m, tea.KeyDown)
	if !m.overview || m.selectedID() != "" {
		t.Fatal("Down selected nonexistent member")
	}
	m = arrow(m, tea.KeyEnter)
	if m.panel != actionsPanel || m.opening != nil {
		t.Fatal("empty overview Enter did not open main actions")
	}
}

func TestSpawnFromOverviewSelectsCreatedMember(t *testing.T) {
	m := newModel(false, false, "", nil)
	m.selectOverview()
	m.spawn = &spawnForm{step: launchStep}
	m = update(m, spawnFinished{manifest: member.Manifest{ID: "created", Name: "Created"}})
	if m.overview || m.selectedID() != "created" {
		t.Fatal("spawn left Overview selected")
	}
}
