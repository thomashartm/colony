package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/thomashartm/motley/internal/member"
)

// Find actual rendered controls, including group headings and scroll offsets.
func actionScreenY(t *testing.T, m Model, label string) int {
	t.Helper()
	for y, line := range strings.Split(ansi.Strip(m.actionsView(m.contentHeight())), "\n") {
		if strings.TrimSpace(line) == strings.TrimSpace(fit(control(label, true), m.detailWidth())) || strings.TrimSpace(line) == strings.TrimSpace(fit(control(label, false), m.detailWidth())) {
			return y + 2 + m.panelHeadingGap()
		}
	}
	t.Fatalf("action %q is not visible:\n%s", label, m.actionsView(m.contentHeight()))
	return -1
}

func actionModel(width, height int) Model {
	m := update(newModel(false, false, "", nil), tea.WindowSizeMsg{Width: width, Height: height})
	m = update(m, snapshot{rows: []member.Row{row("alpha", true)}})
	return update(m, key("3"))
}

func TestActionsGroupsAndConsequences(t *testing.T) {
	m := actionModel(150, 40)
	view := ansi.Strip(m.actionsView(m.contentHeight()))
	for _, group := range []string{"Member", "Session & cleanup", "Overview"} {
		if !strings.Contains(view, "\n"+group+"\n") {
			t.Fatalf("missing group %s:\n%s", group, view)
		}
		if group != "Member" && !strings.Contains(view, "\n\n"+group+"\n") {
			t.Fatalf("no gap before %s", group)
		}
	}
	for i, action := range m.actions() {
		if action.description == "" {
			t.Fatalf("no explanation for %s", action.label)
		}
		m.actionCursor = i
		view := ansi.Strip(m.actionsView(m.contentHeight()))
		if !strings.Contains(view, "> "+action.label) {
			t.Fatalf("selection missing: %s", action.label)
		}
		text := strings.Join(strings.Fields(view), " ")
		switch action.key {
		case "x":
			for _, want := range []string{"Removes the worktree", "local branch", "Force", "unpushed commits"} {
				if !strings.Contains(text, want) {
					t.Fatalf("retirement consequence missing: %s\n%s", want, view)
				}
			}
		case "d":
			if !strings.Contains(text, "confirmation") || !strings.Contains(text, "Keeps its worktree") {
				t.Fatal(view)
			}
		case "r":
			if !strings.Contains(text, "Immediately restarts") || !strings.Contains(text, "Does not restore retired") {
				t.Fatal(view)
			}
		}
		if strings.Contains(strings.ToLower(view), "what happens") {
			t.Fatal("help box still has a heading")
		}
		if strings.ContainsAny(view, "╭╮╰╯│") {
			t.Fatal("action help still has box borders")
		}
		lines := strings.Split(view, "\n")
		boxStart := -1
		for y, line := range lines {
			if strings.HasPrefix(line, "┄") {
				boxStart = y
				break
			}
		}
		bodyLines := strings.Count(ansi.Wrap(action.description, m.detailWidth(), ""), "\n") + 1
		if boxStart < 0 || len(lines)-boxStart != bodyLines+1 {
			t.Fatal("help message contains extra vertical padding")
		}
		if len(lines) != m.contentHeight() || strings.TrimSpace(lines[len(lines)-1]) == "" {
			t.Fatal("help message is not anchored at the bottom")
		}
		panel := strings.Split(ansi.Strip(m.panelHeading(view, m.detailWidth())), "\n")
		if len(panel) != m.panelHeight() || strings.TrimSpace(panel[len(panel)-1]) == "" {
			t.Fatal("heading spacing moved help away from the panel bottom")
		}
	}
}

func TestActionsHoverAndNonActionRows(t *testing.T) {
	m := actionModel(150, 40)
	x := m.listWidth() + 4
	y := actionScreenY(t, m, "Retire member + worktree (x)")
	next, cmd := m.Update(tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionMotion})
	m = next.(Model)
	if cmd != nil || m.busy || m.retiring != nil || m.actions()[m.actionCursor].key != "x" {
		t.Fatal("hover executed an action or did not select retirement")
	}
	if !strings.Contains(ansi.Strip(m.actionsView(m.contentHeight())), "Removes the worktree") {
		t.Fatal("hover did not update help")
	}
	selected := m.actionCursor
	for i, line := range strings.Split(ansi.Strip(m.actionsView(m.contentHeight())), "\n") {
		if strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "> ") {
			continue
		}
		next, cmd = m.Update(tea.MouseMsg{X: x, Y: i + 2 + m.panelHeadingGap(), Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		candidate := next.(Model)
		if cmd != nil || candidate.busy || candidate.actionCursor != selected || candidate.retiring != nil || candidate.manager {
			t.Fatalf("non-action row %d activated something: %q", i, line)
		}
	}
	next, cmd = m.Update(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if cmd == nil || next.(Model).retiring == nil {
		t.Fatal("click did not open retirement checks")
	}
}

func TestActionsScrollResizeAndHoverStability(t *testing.T) {
	for _, size := range [][2]int{{60, 10}, {80, 12}, {80, 20}, {100, 25}, {150, 40}} {
		m := actionModel(size[0], size[1])
		for i := range m.actions() {
			view := m.View()
			if lipgloss.Width(view) > m.width || lipgloss.Height(view) > m.height {
				t.Fatalf("actions overflow at %dx%d:\n%s", m.width, m.height, view)
			}
			actionScreenY(t, m, m.actions()[i].label)
			m = arrow(m, tea.KeyDown)
		}
		// Hover an earlier visible row while scrolled; its position must stay put.
		for _, action := range m.actions() {
			view := ansi.Strip(m.actionsView(m.contentHeight()))
			if !strings.Contains(view, "  "+action.label+"\n") {
				continue
			}
			y := actionScreenY(t, m, action.label)
			m = update(m, tea.MouseMsg{X: m.listWidth() + 4, Y: y, Action: tea.MouseActionMotion})
			if got := actionScreenY(t, m, action.label); got != y {
				t.Fatalf("hover moved row from %d to %d", y, got)
			}
		}
		m = update(m, tea.WindowSizeMsg{Width: 60, Height: 10})
		actionScreenY(t, m, m.actions()[m.actionCursor].label)
	}
}

func TestActionsWithoutMember(t *testing.T) {
	m := actionModel(100, 25)
	m = update(m, snapshot{})
	for _, action := range m.actions() {
		if action.group == "Member" || action.group == "Session & cleanup" {
			t.Fatal("member action shown without target")
		}
	}
	m = update(m, key("3"))
	m = arrow(m, tea.KeyEnter)
	if !m.manager {
		t.Fatal("crew management unreachable without members")
	}
}

func TestActionsDoNotCaptureFooterClicks(t *testing.T) {
	m := actionModel(60, 10)
	next, cmd := m.Update(tea.MouseMsg{
		X: ansi.StringWidth(m.navigationBar()[:strings.Index(m.navigationBar(), "[q Close]")]) + 1, Y: m.height - 1,
		Button: tea.MouseButtonLeft, Action: tea.MouseActionPress,
	})
	if cmd == nil {
		t.Fatal("Actions panel swallowed Close button")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("Close did not quit: %T", next)
	}
}

func TestReviveRoutesNeverOpenRetirement(t *testing.T) {
	for _, route := range []string{"shortcut", "enter", "mouse"} {
		t.Run(route, func(t *testing.T) {
			m := actionModel(100, 25)
			m = update(m, snapshot{rows: []member.Row{row("stopped", false)}})
			for i, action := range m.actions() {
				if action.key == "r" {
					m.actionCursor = i
					break
				}
			}
			var msg tea.Msg = key("r")
			if route == "enter" {
				msg = tea.KeyMsg{Type: tea.KeyEnter}
			}
			if route == "mouse" {
				msg = tea.MouseMsg{X: m.listWidth() + 4, Y: actionScreenY(t, m, "Revive member (r)"), Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
			}
			next, cmd := m.Update(msg)
			got := next.(Model)
			if cmd == nil || !got.busy || got.busyText != "Reviving…" || got.retiring != nil || got.terminating != nil {
				t.Fatalf("%s did not dispatch Revive", route)
			}
		})
	}
}

func TestActionShortcutsUseDistinctLowercaseLetters(t *testing.T) {
	m := actionModel(100, 25)
	m.monitor, m.group = true, "crew"
	seen := map[string]bool{}
	for _, action := range m.actions() {
		if action.key != strings.ToLower(action.key) || seen[action.key] {
			t.Fatalf("ambiguous action shortcut: %s", action.key)
		}
		seen[action.key] = true
	}
}
