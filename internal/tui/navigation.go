package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	listPanel = iota
	detailPanel
	actionsPanel
)

type navigationAction struct {
	label, key, group, description string
	warning                        bool
}

func (m Model) actions() []navigationAction {
	actions := []navigationAction{}
	add := func(group, label, key, description string, warning bool) {
		actions = append(actions, navigationAction{label, key, group, description, warning})
	}
	if m.selectedID() != "" {
		openHelp := "Switches to this member's running agent so you can interact with it directly."
		if m.selectedRow().External {
			openHelp = "Shows where the agent is running in its original terminal. Switch there yourself, or terminate and revive it to run it in Motley."
		}
		add("Member", "Open agent (o)", "o", openHelp, false)
		add("Member", "Edit member (e)", "e", "Opens an editor for the member's name, ticket, crew and colour. Changes apply when you save.", false)
		add("Member", "Open in browser (b)", "b", "Choose the branch, compare view, issue or crew link to open in your browser. Only links that exist are offered.", false)
		if !m.selectedRow().External {
			add("Member", "Reply (i)", "i", "Opens a reply field. Submitting sends your text and Enter to the running agent. Permission decisions must be made in the agent.", false)
			add("Member", "Send to work tab (t)", "t", "Choose an attached work tab to display this member's running session there.", false)
		}
	}
	if m.selectedID() == "" {
		add("Main actions", "Manage crews (m)", "m", "Opens crew management to add, edit, recolour or delete crews and organise their members.", false)
		add("Main actions", "Spawn member (s)", "s", "Opens setup for a new member. Launch creates its worktree and starts the chosen agent after you review the preview.", false)
		add("Main actions", "Add existing Claude (a)", "a", "Lists running Claude sessions to add to Motley. Import keeps the agent in its original terminal and leaves its files in place.", false)
		add("Main actions", "Open agent (o)", "o", "Choose a running agent to open. The picker includes all members, even when the main list is filtered.", false)
	}
	if m.selectedID() != "" {
		add("Session & cleanup", "Revive member (r)", "r", "Immediately restarts a stopped member in its existing worktree, resuming its saved agent session when available. Does not restore retired members or deleted worktrees.", false)
		add("Session & cleanup", "Terminate agent (d)", "d", "Asks for confirmation, then stops all processes in this member's session. Keeps its worktree, branch and history so you can revive it.", true)
		if m.selectedRow().ClaudeSession != "" {
			add("Session & cleanup", "Retire member; keep files (x)", "x", "Asks for confirmation, then stops the agent and archives the member and history. Keeps the imported checkout, files and branches.", true)
		} else {
			add("Session & cleanup", "Retire member + worktree (x)", "x", "Removes the worktree and stops the session after cleanup checks and confirmation. Archives the member and history. Deletes the local branch unless kept or protected; remote branches stay. Force can discard uncommitted work and unpushed commits.", true)
		}
	}
	if m.selectedID() == "" {
		add("View", "Change grouping (g)", "g", "Cycles the member list between attention, crew and repository grouping.", false)
		add("View", "Filter members (/)", "/", "Opens search to narrow the member list. Enter keeps the filter; Esc clears it.", false)
		if m.group == "crew" {
			add("View", "Show/hide inactive crews (h)", "h", "Toggles whether inactive crews appear in the list.", false)
		}
		if m.monitor {
			add("View", "Pin work tab (p)", "p", "Choose the work tab used when opening agents from the monitor. Automatic selection can be restored in the picker.", false)
		}
	} else {
		add("Overview", "Main actions (Home)", "home", "Selects Overview for spawning and adding agents, managing crews and changing the view.", false)
	}
	// Keep Copy last so a message appearing does not change existing action indices.
	if m.copyableMessage() != "" {
		add("View", "Copy message (c)", "c", "Copies the full status or error message to the clipboard, including text truncated on screen.", false)
	}
	return actions
}

// navigationKey runs after modal editors, so arrows in text remain caret keys.
func (m Model) navigationKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	key := msg.String()
	// Direct panel keys are handled after text inputs and confirmation dialogs.
	switch key {
	case "home":
		m.selectOverview()
		m.panel, m.actionCursor, m.actionScroll = actionsPanel, 0, 0
		return m, nil, true
	case "1":
		m.panel, m.tableFocus = listPanel, false
		return m, nil, true
	case "2":
		m.panel = detailPanel
		m.tableFocus = !m.overview && m.group == "crew" && m.currentEntry().id == "" && len(m.members(m.currentEntry().crew)) > 0
		return m, nil, true
	case "3":
		m.panel, m.actionCursor, m.actionScroll = actionsPanel, 0, 0
		return m, nil, true
	}
	if m.panel == actionsPanel {
		actions := m.actions()
		m.actionCursor = max(0, min(m.actionCursor, len(actions)-1))
		switch key {
		case "up", "k":
			m.actionCursor = max(0, m.actionCursor-1)
		case "down", "j":
			m.actionCursor = min(len(actions)-1, m.actionCursor+1)
		case "left", "esc", "shift+tab":
			m.panel = detailPanel
		case "tab":
			m.panel, m.tableFocus = listPanel, false
		case "enter":
			next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(actions[m.actionCursor].key)})
			return next, cmd, true
		case "right":
		default:
			return m, nil, false
		}
		m.actionScroll = m.layoutActions(m.contentHeight()).start
		return m, nil, true
	}
	if m.tableFocus {
		m.panel = detailPanel
	}
	switch key {
	case "right":
		if m.panel == detailPanel {
			m.panel, m.actionCursor, m.actionScroll = actionsPanel, 0, 0
		} else if m.overview {
			m.panel, m.actionCursor, m.actionScroll = actionsPanel, 0, 0
		} else {
			e := m.currentEntry()
			if !m.overview && m.group == "crew" && e.id == "" && e.crew != "" && !m.expanded[e.crew] {
				return m, nil, false // retain the first Right's expand behavior
			}
			m.panel = detailPanel
			m.tableFocus = !m.overview && m.group == "crew" && e.id == "" && len(m.members(e.crew)) > 0
		}
		return m, nil, true
	case "left", "esc":
		if m.panel == detailPanel {
			m.panel, m.tableFocus = listPanel, false
			return m, nil, true
		}
	case "tab", "shift+tab":
		if !m.overview && m.group == "crew" {
			return m, nil, false
		} // preserve crew table's Tab toggle
		if key == "tab" {
			m.panel = (m.panel + 1) % 3
		} else {
			m.panel = (m.panel + 2) % 3
		}
		m.actionCursor, m.actionScroll = 0, 0
		return m, nil, true
	case "up", "down", "j", "k":
		if m.panel == detailPanel && !m.tableFocus {
			m.detail, _ = m.detail.Update(msg)
			return m, nil, true
		}
	}
	return m, nil, false
}

// The renderer and mouse handler share rows so headings, gaps and help never
// become clickable actions. Keep the scroll position stable while hovering.
type actionRow struct {
	text  string
	index int // -1 for a heading or gap
}
type actionLayout struct {
	rows                          []actionRow
	start, menuHeight, helpHeight int
}

func (m Model) layoutActions(height int) actionLayout {
	actions := m.actions()
	cursor := max(0, min(m.actionCursor, len(actions)-1))
	layout := actionLayout{}
	selectedLine := 0
	for i, action := range actions {
		if i == 0 || action.group != actions[i-1].group {
			if i > 0 {
				layout.rows = append(layout.rows, actionRow{"", -1})
			}
			layout.rows = append(layout.rows, actionRow{action.group, -1})
		}
		if i == cursor {
			selectedLine = len(layout.rows)
		}
		layout.rows = append(layout.rows, actionRow{action.label, i})
	}
	// Reserve a fixed help height across selections to prevent pointer targets
	// moving when descriptions wrap differently.
	helpWidth := m.detailWidth()
	for _, action := range actions {
		lines := strings.Count(ansi.Wrap(action.description, helpWidth, ""), "\n") + 1
		layout.helpHeight = max(layout.helpHeight, lines+1)
	}
	layout.helpHeight = min(layout.helpHeight, max(2, height/2))
	layout.menuHeight = max(1, height-1-layout.helpHeight)
	layout.start = max(0, min(m.actionScroll, len(layout.rows)-layout.menuHeight))
	if selectedLine < layout.start {
		layout.start = selectedLine
	}
	if selectedLine >= layout.start+layout.menuHeight {
		layout.start = selectedLine - layout.menuHeight + 1
	}
	return layout
}

func (m Model) actionsView(height int) string {
	actions := m.actions()
	cursor := max(0, min(m.actionCursor, len(actions)-1))
	layout := m.layoutActions(height)
	width := m.detailWidth()
	title := "Overview actions"
	if id := m.selectedID(); id != "" {
		title = "Actions: " + clean(m.selectedRow().Name)
		if m.selectedRow().Name == "" {
			title = "Actions: " + clean(id)
		}
	}
	if layout.start > 0 {
		title += " ↑"
	}
	if layout.start+layout.menuHeight < len(layout.rows) {
		title += " ↓"
	}
	lines := []string{fit(title, width)}
	for y := 0; y < layout.menuHeight; y++ {
		line := ""
		if i := layout.start + y; i < len(layout.rows) {
			row := layout.rows[i]
			if row.index >= 0 {
				line = fit(control(row.text, row.index == cursor), width)
				if row.index == cursor {
					line = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6")).Render(line)
				}
			} else {
				line = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("8")).Render(fit(row.text, width))
			}
		}
		lines = append(lines, line)
	}
	lines = append(lines, actionHelp(actions[cursor], width, layout.helpHeight))
	return strings.Join(lines, "\n")
}

func actionHelp(action navigationAction, width, height int) string {
	colour := lipgloss.Color("6")
	if action.warning {
		colour = lipgloss.Color("3")
	}
	body := strings.Split(ansi.Wrap(action.description, width, ""), "\n")
	bodyHeight := height - 1
	if len(body) > bodyHeight {
		body = body[:bodyHeight]
		body[bodyHeight-1] = ansi.Truncate(body[bodyHeight-1]+" …", width, "…")
	}
	message := panelDivider(width) + "\n" + lipgloss.NewStyle().Foreground(colour).Render(strings.Join(body, "\n"))
	// Keep unused reserved space above the message, never inside or below it.
	return strings.Repeat("\n", max(0, height-lipgloss.Height(message))) + message
}

func control(label string, focused bool) string {
	if focused {
		return "> " + label
	}
	return "  " + label
}

// panelHeading adds a divider and the requested space below the title.
func panelHeading(content string, width, gap int) string {
	if gap == 0 {
		return content
	}
	title, body, _ := strings.Cut(content, "\n")
	divider := panelDivider(width)
	return title + "\n" + divider + strings.Repeat("\n", gap) + body
}

// panelContentY maps screen coordinates back to the unadorned panel content.
// The divider and blank row are not interactive.
func (m Model) panelContentY(screenY int) int {
	return contentY(screenY, m.panelHeadingGap())
}

func contentY(screenY, gap int) int {
	y := screenY - 2 // app header and top border
	if y > 0 {
		if y <= gap {
			return -1
		}
		y -= gap
	}
	return y
}

func panelDivider(width int) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(strings.Repeat("┄", width))
}
