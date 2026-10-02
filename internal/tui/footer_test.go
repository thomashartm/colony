package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestFooterFitsEveryContextAfterResize(t *testing.T) {
	contexts := map[string]func(*Model){
		"list":         func(m *Model) {},
		"overview":     func(m *Model) { m.selectOverview() },
		"open picker":  func(m *Model) { m.opening = &agentPicker{} },
		"import":       func(m *Model) { m.importing = &importDialog{} },
		"monitor":      func(m *Model) { m.monitor = true },
		"crew":         func(m *Model) { m.group = "crew" },
		"crew monitor": func(m *Model) { m.group = "crew"; m.monitor = true },
		"details":      func(m *Model) { m.panel = detailPanel },
		"table":        func(m *Model) { m.group = "crew"; m.tableFocus = true },
		"actions":      func(m *Model) { m.panel = actionsPanel },
		"edit":         func(m *Model) { m.editor = newEditor("member", "waiting", []string{"Name"}, []string{"Example"}) },
		"delete":       func(m *Model) { m.editor = newEditor("delete", "fx", nil, nil) },
		"reply":        func(m *Model) { m.editor = newEditor("reply", "waiting", []string{"Reply"}, []string{"Hello"}) },
		"crews":        func(m *Model) { m.manager = true },
		"crew actions": func(m *Model) { m.manager = true; m.managerActions = true },
		"terminate":    func(m *Model) { m.terminating = &terminateDialog{id: "waiting"} },
		"retire":       func(m *Model) { m.retiring = &retireDialog{id: "waiting", loaded: true} },
		"pin":          func(m *Model) { m.picking = true },
		"send":         func(m *Model) { m.picking = true; m.pickMode = "send" },
		"filter":       func(m *Model) { m.searching = true },
	}
	for name, setup := range contexts {
		t.Run(name, func(t *testing.T) {
			m := update(newModel(false, false, "", nil), crewSnapshot())
			setup(&m)
			// Shrink and grow one model, as a popup or resized terminal does.
			for _, size := range [][2]int{{160, 30}, {80, 24}, {60, 10}, {60, 12}, {120, 30}} {
				m = update(m, tea.WindowSizeMsg{Width: size[0], Height: size[1]})
				assertFooterFits(t, m)
			}
		})
	}
	for step := repoStep; step <= launchStep; step++ {
		m := newModel(false, false, "", nil)
		m.spawn = &spawnForm{step: step, fields: inputs("", "Example", "feat/example"), vars: []string{"first", "second", "third"}}
		for _, size := range [][2]int{{120, 30}, {60, 10}, {80, 24}} {
			m = update(m, tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			assertFooterFits(t, m)
		}
	}
}

func assertFooterFits(t *testing.T, m Model) {
	t.Helper()
	footer := m.footer()
	if strings.Contains(footer, "…") { // launch progress is text, not a truncated key
		if m.spawn == nil || m.spawn.step != launchStep {
			t.Fatalf("footer cut a shortcut: %q", footer)
		}
	}
	if lipgloss.Height(footer) != m.footerRows() {
		t.Fatalf("footer row budget: %q", footer)
	}
	for _, line := range strings.Split(footer, "\n") {
		if ansi.StringWidth(line) > m.width-2 {
			t.Fatalf("footer overflows %d columns: %q", m.width, line)
		}
	}
	if m.panelHeadingGap() > 0 {
		lines := strings.Split(ansi.Strip(m.View()), "\n")
		if strings.Count(lines[3], "┄") != m.listWidth()+m.detailWidth() {
			t.Fatal("panel headings are missing dotted dividers")
		}
		if strings.TrimSpace(strings.ReplaceAll(lines[4], "│", "")) != "" {
			t.Fatal("panel dividers need a blank row below")
		}
	}
	if lipgloss.Width(m.View()) > m.width || lipgloss.Height(m.View()) > m.height {
		t.Fatalf("view overflows %dx%d: %dx%d", m.width, m.height, lipgloss.Width(m.View()), lipgloss.Height(m.View()))
	}
}

func TestFooterGroupsAndEssentialControls(t *testing.T) {
	m := update(newModel(false, false, "", nil), tea.WindowSizeMsg{Width: 80, Height: 24})
	for _, want := range []string{"Nav:", "Act:", "View:", "q quit", "s spawn", "g group", "x retire"} {
		if !strings.Contains(m.footer(), want) {
			t.Fatalf("missing group/shortcut %q: %s", want, m.footer())
		}
	}
	m = update(m, tea.WindowSizeMsg{Width: 60, Height: 10})
	for _, want := range []string{"[1 List]", "[2 Details]", "[3 Actions]", "[o Open agent]", "[q Close]"} {
		if !strings.Contains(m.footer(), want) {
			t.Fatalf("missing essential %q", want)
		}
	}
	m = update(m, tea.WindowSizeMsg{Width: 60, Height: 12})
	m.panel = actionsPanel
	if !strings.Contains(m.footer(), "enter run") || !strings.Contains(m.footer(), "esc") {
		t.Fatal("action controls hidden")
	}
}

func TestOpenAgentAlwaysVisible(t *testing.T) {
	for _, height := range []int{10, 12, 24} {
		m := update(newModel(true, true, "", nil), tea.WindowSizeMsg{Width: 60, Height: height})
		for _, panel := range []int{listPanel, detailPanel, actionsPanel} {
			m.panel = panel
			if !strings.Contains(m.footer(), "[o Open agent]") {
				t.Fatal("Open agent control hidden")
			}
		}
	}
}
