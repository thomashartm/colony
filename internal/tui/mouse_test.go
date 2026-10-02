package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/thomashartm/motley/internal/member"
)

func click(m Model, x, y int) Model {
	return update(m, tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
}

func listScreenY(t *testing.T, m Model, text string) int {
	t.Helper()
	for y, line := range strings.Split(ansi.Strip(m.listView(m.listContentHeight(), m.listWidth())), "\n") {
		if strings.Contains(line, text) {
			if y > 0 {
				y += m.listHeadingGap()
			}
			return y + 2
		}
	}
	t.Fatalf("list row %q not visible:\n%s", text, m.listView(m.listContentHeight(), m.listWidth()))
	return -1
}

func TestMouseNavigation(t *testing.T) {
	m := update(newModel(false, false, "", nil), tea.WindowSizeMsg{Width: 80, Height: 20})
	m = update(m, snapshot{rows: []member.Row{row("alpha", true), row("beta", true)}})
	m = click(m, ansi.StringWidth(m.navigationBar()[:strings.Index(m.navigationBar(), "[2 Details]")])+1, m.height-1)
	if m.panel != detailPanel {
		t.Fatal("details button")
	}
	m = click(m, ansi.StringWidth(m.navigationBar()[:strings.Index(m.navigationBar(), "[1 List]")])+1, m.height-1)
	if m.panel != listPanel {
		t.Fatal("list button")
	}
	m = click(m, 3, listScreenY(t, m, "beta"))
	if m.selectedID() != "beta" {
		t.Fatalf("clicked member: %s", m.selectedID())
	}
	m = click(m, ansi.StringWidth(m.navigationBar()[:strings.Index(m.navigationBar(), "[3 Actions]")])+1, m.height-1)
	if m.panel != actionsPanel {
		t.Fatal("actions button")
	}
	m = click(m, m.listWidth()+4, actionScreenY(t, m, "Edit member (e)"))
	if m.editor == nil || m.editor.id != "beta" {
		t.Fatal("action click")
	}
	m = click(m, ansi.StringWidth(m.navigationBar()[:strings.Index(m.navigationBar(), "[1 List]")])+1, m.height-1)
	if m.editor == nil || m.panel != actionsPanel {
		t.Fatal("click escaped editor")
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyEsc})
	m = click(m, ansi.StringWidth(m.navigationBar()[:strings.Index(m.navigationBar(), "[2 Details]")])+1, m.height-1)
	m = click(m, ansi.StringWidth(m.navigationBar()[:strings.Index(m.navigationBar(), "[o Open agent]")])+1, m.height-1)
	if m.attachID != "beta" {
		t.Fatal("explicit Enter button did not jump")
	}
}

func TestMouseIgnoresReleaseAndSmallTerminal(t *testing.T) {
	m := update(newModel(false, false, "", nil), tea.WindowSizeMsg{Width: 80, Height: 20})
	m = update(m, tea.MouseMsg{X: 12, Y: 19, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})
	if m.panel != listPanel {
		t.Fatal("release activated control")
	}
	m = update(m, tea.WindowSizeMsg{Width: 40, Height: 8})
	m = click(m, 12, 7)
	if m.panel != listPanel {
		t.Fatal("hidden controls activated")
	}
}

func TestMouseCrewAndScrolling(t *testing.T) {
	m := update(newModel(false, false, "", nil), tea.WindowSizeMsg{Width: 80, Height: 20})
	m = update(m, crewSnapshot())
	m = update(m, key("g"))
	m = click(m, 3, listScreenY(t, m, "FX Banking"))
	if !m.expanded["fx"] {
		t.Fatal("crew click did not expand")
	}
	m = click(m, m.listWidth()+4, 6+m.panelHeadingGap())
	if !m.tableFocus || m.selectedID() != "busy" {
		t.Fatalf("crew member click: %s", m.selectedID())
	}
	m = update(newModel(false, false, "", nil), tea.WindowSizeMsg{Width: 60, Height: 10})
	m = update(m, snapshot{rows: []member.Row{row("a", true), row("b", true)}})
	m.detail.SetContent(strings.Repeat("detail line\n", 20))
	m = update(m, tea.MouseMsg{X: m.listWidth() + 4, Y: 3, Button: tea.MouseButtonWheelDown})
	if m.detail.YOffset == 0 || m.selectedID() != "a" {
		t.Fatal("wheel did not scroll details independently")
	}
	m = update(m, tea.MouseMsg{X: 3, Y: 3, Button: tea.MouseButtonWheelDown})
	if m.selectedID() != "b" {
		t.Fatal("wheel over list did not select member")
	}
}

func TestMouseUsesMergedFooterBounds(t *testing.T) {
	for _, size := range [][2]int{{60, 10}, {80, 10}, {80, 12}, {80, 20}} {
		height := size[1]
		m := update(newModel(false, false, "", nil), tea.WindowSizeMsg{Width: size[0], Height: height})
		m = update(m, snapshot{rows: []member.Row{row("alpha", true), row("beta", true)}})
		lines := strings.Split(m.View(), "\n")
		if lines[height-1] != m.navigationBar() {
			t.Fatal("navigation not on last row")
		}
		if strings.Count(lines[height-1], "·") != 4 {
			t.Fatal("navigation items are missing separators")
		}
		for x, ch := range lines[height-1] {
			if ch == '·' {
				next, cmd := m.Update(tea.MouseMsg{X: ansi.StringWidth(lines[height-1][:x]), Y: height - 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
				if cmd != nil || next.(Model).panel != m.panel {
					t.Fatal("separator activated a navigation button")
				}
			}
		}
		m = click(m, ansi.StringWidth(m.navigationBar()[:strings.Index(m.navigationBar(), "[3 Actions]")])+1, height-1)
		if m.panel != actionsPanel {
			t.Fatal("visible Actions button missed")
		}
		// The border and grouped hint rows must never scroll/select an action.
		before := m.actionCursor
		m = update(m, tea.MouseMsg{X: m.listWidth() + 4, Y: 2 + m.panelHeight(), Button: tea.MouseButtonWheelDown})
		if m.actionCursor != before {
			t.Fatal("footer wheel moved selection")
		}
		m = click(m, m.listWidth()+4, 2+m.panelHeight())
		if m.editor != nil || m.manager || m.actionCursor != before {
			t.Fatal("footer click activated a hidden action")
		}
	}
}
