package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/thomashartm/motley/internal/crew"
	"github.com/thomashartm/motley/internal/member"
	"github.com/thomashartm/motley/internal/palette"
	"github.com/thomashartm/motley/internal/state"
)

const noCrew = "~none"

type crewEntry struct{ crew, id string }

func (e crewEntry) key() string {
	if e.id != "" {
		return "member:" + e.id
	}
	return "crew:" + e.crew
}
func colored(text string, c palette.Color) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(c.Hex)).Render(text)
}
func link(text, url string) string {
	if url == "" {
		return text
	}
	return "\x1b]8;;" + url + "\x1b\\" + text + "\x1b]8;;\x1b\\"
}
func (m Model) crewFor(id string) crew.Crew {
	if c, ok := crew.Find(m.crews, id); ok {
		return c
	}
	return crew.Crew{ID: noCrew, Title: "No crew", Color: "grey"}
}
func (m Model) members(id string) []member.Row {
	var rows []member.Row
	for _, r := range m.rows {
		if m.crewFor(r.Crew).ID == id {
			rows = append(rows, r)
		}
	}
	return rows
}
func (m Model) crewEntries() []crewEntry {
	crews := append([]crew.Crew(nil), m.crews...)
	sort.Slice(crews, func(i, j int) bool {
		if crews[i].Title == crews[j].Title {
			return crews[i].ID < crews[j].ID
		}
		return crews[i].Title < crews[j].Title
	})
	crews = append(crews, m.crewFor(noCrew))
	var entries []crewEntry
	for _, c := range crews {
		members := m.members(c.ID)
		if m.query.Value() != "" && len(members) == 0 {
			continue
		}
		live := false
		for _, r := range members {
			live = live || r.Alive
		}
		if !live && !m.showHidden {
			continue
		}
		if c.ID == noCrew && len(members) == 0 {
			continue
		}
		entries = append(entries, crewEntry{crew: c.ID})
		if m.expanded[c.ID] {
			for _, r := range members {
				entries = append(entries, crewEntry{crew: c.ID, id: r.ID})
			}
		}
	}
	return entries
}
func (m Model) currentEntry() crewEntry {
	entries := m.crewEntries()
	if m.crewCursor >= 0 && m.crewCursor < len(entries) {
		return entries[m.crewCursor]
	}
	return crewEntry{}
}
func (m Model) selectedRow() member.Row {
	id := m.selectedID()
	for _, r := range m.rows {
		if r.ID == id {
			return r
		}
	}
	return member.Row{}
}
func (m *Model) restoreCrewSelection(key, member string) {
	entries := m.crewEntries()
	m.crewCursor = max(0, min(m.crewCursor, len(entries)-1))
	for i, e := range entries {
		if e.key() == key {
			m.crewCursor = i
			break
		}
	}
	rows := m.members(m.currentEntry().crew)
	m.tableCursor = max(0, min(m.tableCursor, len(rows)-1))
	for i, r := range rows {
		if r.ID == member {
			m.tableCursor = i
			break
		}
	}
	if len(rows) == 0 {
		m.tableFocus = false
	}
}
func (m *Model) sortRows() {
	sort.SliceStable(m.rows, func(i, j int) bool {
		a, b := m.rows[i], m.rows[j]
		if m.group == "repo" && a.Repo != b.Repo {
			return a.Repo < b.Repo
		}
		if sectionOrder(a) != sectionOrder(b) {
			return sectionOrder(a) < sectionOrder(b)
		}
		if state.Attention(a.CurrentStatus()) && a.Since != b.Since {
			return a.Since < b.Since
		}
		return a.ID < b.ID
	})
}
func (m Model) groupingKey(key string) (Model, tea.Cmd, bool) {
	oldID := m.selectedID()
	handled := true
	switch key {
	case "g":
		switch m.group {
		case "crew":
			m.group = "repo"
		case "repo":
			m.group = "attention"
		default:
			m.group = "crew"
		}
		m.tableFocus = false
		m.sortRows()
		for i, r := range m.rows {
			if r.ID == oldID {
				m.selected = i
				break
			}
		}
		if m.group == "crew" {
			for i, e := range m.crewEntries() {
				if e.crew == m.crewFor(m.rowsSafeSelected().Crew).ID && e.id == "" {
					m.crewCursor = i
					break
				}
			}
			m.restoreCrewSelection("", "")
		}
	case "H":
		key := m.currentEntry().key()
		m.showHidden = !m.showHidden
		m.restoreCrewSelection(key, oldID)
	default:
		if m.group != "crew" {
			return m, nil, false
		}
		e := m.currentEntry()
		switch key {
		case "j", "down", "k", "up":
			delta := 1
			if key == "k" || key == "up" {
				delta = -1
			}
			if m.tableFocus {
				m.tableCursor = max(0, min(m.tableCursor+delta, len(m.members(e.crew))-1))
			} else {
				m.crewCursor = max(0, min(m.crewCursor+delta, len(m.crewEntries())-1))
				m.tableCursor = 0
			}
		case "tab":
			if e.id == "" && len(m.members(e.crew)) > 0 {
				m.tableFocus = !m.tableFocus
			}
		case "esc":
			m.tableFocus = false
		case "enter":
			if !m.tableFocus && e.id == "" {
				m.tableFocus = len(m.members(e.crew)) > 0
			} else {
				handled = false
			}
		case "right", " ":
			if e.id == "" && e.crew != "" {
				if m.expanded == nil {
					m.expanded = map[string]bool{}
				}
				m.expanded[e.crew] = key == "right" || !m.expanded[e.crew]
				m.tableFocus = false
			}
		case "left":
			delete(m.expanded, e.crew)
			m.tableFocus = false
			m.restoreCrewSelection("crew:"+e.crew, "")
		default:
			handled = false
		}
	}
	if !handled {
		return m, nil, false
	}
	m.alert = false
	m.message = ""
	m.updateDetail()
	return m, m.requestDetail(oldID != m.selectedID()), true
}
func (m Model) rowsSafeSelected() member.Row {
	if m.selected >= 0 && m.selected < len(m.rows) {
		return m.rows[m.selected]
	}
	return member.Row{}
}
func (m Model) memberLine(r member.Row, width int) string {
	color := member.Color(r.Manifest, m.crews)
	icon, statusColor := statusIcon(r.CurrentStatus())
	badge, bc := palette.Badge(r.Agent)
	prefix := colored("▌", color) + " " + lipgloss.NewStyle().Foreground(statusColor).Render(icon) + " " + colored(badge, bc) + " "
	name := r.Name
	if name == "" {
		name = r.ID
	}
	if r.Ticket != "" {
		name = r.Ticket + " · " + name
	}
	suffix := " " + since(r)
	if c, ok := crew.Find(m.crews, r.Crew); ok && width >= 32 {
		cc := palette.Resolve(c.ID, c.Color, "")
		suffix = " " + colored(fit(clean(c.Title), 8), cc) + suffix
	}
	return prefix + fit(clean(name), max(1, width-lipgloss.Width(prefix)-lipgloss.Width(suffix))) + suffix
}
func (m Model) crewList(height, width int) string {
	entries := m.crewEntries()
	if len(entries) == 0 {
		return "No live crews.\nH shows inactive crews.\nG manages crews."
	}
	var lines []string
	for i, e := range entries {
		line := ""
		if e.id != "" {
			for _, r := range m.rows {
				if r.ID == e.id {
					line = "  " + m.memberLine(r, width-2)
					break
				}
			}
		} else {
			c := m.crewFor(e.crew)
			color := palette.Resolve(c.ID, c.Color, "")
			arrow := "▸"
			if m.expanded[e.crew] {
				arrow = "▾"
			}
			counts := totals(m.members(e.crew))
			title := fit(clean(c.Title), max(1, width-lipgloss.Width(counts)-5))
			if c.URL != "" {
				title = fit(title, max(1, width-lipgloss.Width(counts)-7)) + " ↗"
			}
			line = colored("▌", color) + " " + arrow + " " + colored(link(title, c.URL), color) + " " + counts
		}
		if i == m.crewCursor {
			line = lipgloss.NewStyle().Reverse(true).Render(line)
		}
		lines = append(lines, fit(line, width))
	}
	start := max(0, m.crewCursor-height+1)
	return strings.Join(lines[start:min(len(lines), start+height)], "\n")
}
func (m Model) crewTable(height, width int) string {
	e := m.currentEntry()
	c := m.crewFor(e.crew)
	rows := m.members(e.crew)
	color := palette.Resolve(c.ID, c.Color, "")
	title := colored("▌ "+clean(c.Title), color)
	if c.URL != "" {
		title = link(title+" ↗", c.URL)
	}
	lines := []string{fit(title+fmt.Sprintf(" · %d members", len(rows)), width)}
	if c.Gig != "" {
		lines = append(lines, fit("Gig  "+clean(c.Gig), width))
	}
	if len(rows) == 0 {
		return strings.Join(append(lines, "No members in this crew."), "\n")
	}
	// The rightmost columns disappear before shrinking the identity columns.
	widths := []int{2, 12, 5, 6, 8}
	headers := []string{"ST", "MEMBER", "AGENT", "TICKET", "REPO"}
	if width >= 56 {
		widths = append(widths, 12)
		headers = append(headers, "BRANCH")
	}
	if width >= 64 {
		widths = append(widths, 5)
		headers = append(headers, "SINCE")
	}
	total := len(widths) - 1
	for _, w := range widths {
		total += w
	}
	widths[1] = max(5, widths[1]+width-total)
	format := func(values []string) string {
		var cells []string
		for i, w := range widths {
			v := fit(values[i], w)
			cells = append(cells, v+strings.Repeat(" ", max(0, w-ansi.StringWidth(v))))
		}
		return fit(strings.Join(cells, " "), width)
	}
	lines = append(lines, format(headers))
	available := max(1, height-len(lines))
	start := max(0, m.tableCursor-available+1)
	for i := start; i < len(rows) && len(lines) < height; i++ {
		r := rows[i]
		icon, sc := statusIcon(r.CurrentStatus())
		badge, bc := palette.Badge(r.Agent)
		ticket := r.Ticket
		if ticket != "" {
			ticket = "#" + ticket
		}
		name := r.Name
		if name == "" {
			name = r.ID
		}
		vals := []string{lipgloss.NewStyle().Foreground(sc).Render(icon), clean(name), colored(badge, bc), clean(ticket), clean(r.Repo), clean(r.Branch), since(r)}
		line := format(vals)
		if m.tableFocus && i == m.tableCursor {
			line = lipgloss.NewStyle().Reverse(true).Render(line)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func coloredBadge(agent string) string { label, c := palette.Badge(agent); return colored(label, c) }
func (m Model) crewLabel(id string) string {
	c := m.crewFor(id)
	return colored(link(clean(c.Title), c.URL), palette.Resolve(c.ID, c.Color, ""))
}
func (m Model) groupName() string {
	if m.group == "" {
		return "attention"
	}
	return m.group
}
