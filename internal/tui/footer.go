package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Reserve up to two hint rows and a navigation row. Keep panel sizes stable
// when a modal hides the navigation buttons.
func (m Model) footerRows() int {
	if m.height < 12 {
		return 1
	}
	if m.height == 12 {
		return 2
	}
	return 3
}

func (m Model) contentHeight() int { return max(1, m.height-4-m.footerRows()) }

// Keep whole groups together and leave the last terminal column unused. A compact
// variant retains navigation and confirm/back controls instead of cutting off keys.
func (m Model) footer() string {
	rows := m.footerRows()
	if m.terminating != nil {
		lines := make([]string, rows)
		if rows > 1 {
			lines[0] = "[Terminate] ↑↓ choose · enter select · esc cancel"
		}
		lines[rows-1] = m.terminateButtons()
		return strings.Join(lines, "\n")
	}
	buttons := m.navigationAvailable()
	if buttons {
		rows--
		if rows == 0 {
			return navigationBar
		}
	}
	full, compact := m.footerGroups()
	width := max(1, m.width-2)
	lines := wrapFooter(full, width)
	if len(lines) > rows {
		lines = wrapFooter(compact, width)
	}
	for len(lines) < rows {
		lines = append(lines, "")
	}
	if buttons {
		lines = append(lines, navigationBar)
	}
	return strings.Join(lines, "\n")
}

func wrapFooter(groups []string, width int) []string {
	var lines []string
	for _, group := range groups {
		if len(lines) == 0 || ansi.StringWidth(lines[len(lines)-1])+3+ansi.StringWidth(group) > width {
			lines = append(lines, group)
		} else {
			lines[len(lines)-1] += " │ " + group
		}
	}
	return lines
}

func (m Model) footerGroups() (full, compact []string) {
	// Match the input handlers: the active dialog owns its footer.
	if m.spawn != nil {
		switch m.spawn.step {
		case repoStep:
			return []string{"[Spawn] Nav: ↑↓ select · type to filter", "Act: enter next · esc cancel"}, []string{"[Spawn] ↑↓ select", "enter next · esc cancel"}
		case identityStep, varsStep:
			return []string{"[Spawn] Nav: ↑↓/tab field · ←→ cursor", "Act: enter next · esc cancel"}, []string{"[Spawn] ↑↓ field", "enter next · esc cancel"}
		case agentStep, blueprintStep, modeStep:
			return []string{"[Spawn] Nav: ↑↓/jk select", "Act: enter next · esc cancel"}, []string{"[Spawn] ↑↓ select", "enter next · esc cancel"}
		case previewStep:
			return []string{"[Preview] Nav: ←→ action · ↑↓ scroll", "Act: enter choose · e edit · esc cancel"}, []string{"[Preview] ←→ action", "enter choose · esc cancel"}
		default:
			return []string{"[Spawn] Launching…"}, []string{"[Spawn] Launching…"}
		}
	}
	if m.searching {
		return []string{"[Filter] Type to search", "Act: enter keep · esc clear"}, []string{"[Filter] type", "enter keep · esc clear"}
	}
	if m.editor != nil {
		if m.editor.kind == "reply" {
			return []string{"[Reply] Edit: ←→ cursor", "Act: enter send · esc cancel"}, []string{"[Reply] ←→ cursor", "enter send · esc cancel"}
		}
		if m.editor.kind == "delete" {
			return []string{"[Delete] Nav: ↑↓ choice", "Act: enter toggle/confirm · esc cancel", "Shortcuts: f force · y delete"}, []string{"[Delete] ↑↓ choice", "enter choose · esc cancel"}
		}
		return []string{"[Edit] Nav: ↑↓/tab field/action · ←→ cursor", "Act: enter next/choose · ctrl+s save · esc cancel"}, []string{"[Edit] ↑↓ field/action", "enter choose · esc cancel"}
	}
	if m.manager {
		if m.managerActions {
			return []string{"[Crews] Nav: ↑↓ action", "Act: enter choose · ←/esc back"}, []string{"[Crews] ↑↓ action", "enter choose · esc back"}
		}
		return []string{"[Crews] Nav: ↑↓ crew · → actions", "Act: enter edit/add · esc back", "Shortcuts: a add · e edit · c colour · x delete"}, []string{"[Crews] ↑↓ crew · → actions", "enter edit · esc back"}
	}
	if m.retiring != nil {
		return []string{"[Retire] Nav: ↑↓ choice", "Act: enter toggle/confirm · esc cancel", "Options: f force · k keep branch"}, []string{"[Retire] ↑↓ choice", "enter choose · esc cancel"}
	}
	if m.picking {
		action := "pin"
		if m.pickMode == "send" {
			action = "send"
		}
		return []string{"[Tabs] Nav: ↑↓/jk select", "Act: enter " + action + " · esc cancel"}, []string{"[Tabs] ↑↓ select", "enter " + action + " · esc cancel"}
	}
	if m.panel == actionsPanel {
		return []string{"[Actions] Nav: ↑↓/jk choose", "Act: enter run · ←/esc details"}, []string{"[Actions] ↑↓ choose", "enter run · esc back"}
	}
	if m.panel == detailPanel || m.tableFocus {
		movement := "scroll"
		if m.tableFocus {
			movement = "member"
		}
		return []string{"[Details] Nav: ↑↓ " + movement + " · ← back · → actions", "Act: enter open · esc list"}, []string{"[Details] ← back · → actions", "↑↓ " + movement + " · enter open"}
	}
	quit := "q quit"
	tabs := "Run: t tab"
	if m.monitor {
		quit = "q detach"
		tabs += " · T pin"
	}
	if m.group == "crew" {
		return []string{"[List] Nav: ↑↓ move · → expand/details · ← collapse", "View: tab members · H hidden · g group · G crews", quit}, []string{"[List] → expand/details · ← collapse", "↑↓ move · " + quit}
	}
	return []string{"[List] Nav: ↑↓/jk · → details", "Act: enter open · s spawn · e edit · i reply", "View: / filter · g group · G crews", tabs + " · x retire · r revive · " + quit}, []string{"[List] ↑↓ move · → details", "enter open · " + quit}
}
