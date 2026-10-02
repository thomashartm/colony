package tui

import (
	"net/url"
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/thomashartm/motley/internal/member"
	"github.com/thomashartm/motley/internal/palette"
)

// Use one layout for painting and hit testing; headings never select members.
const (
	listHeading   = -1
	overviewEntry = -2
)

type memberListLine struct {
	text  string
	index int
}
type memberListLayout struct {
	fixed, body []memberListLine
	start       int
}

func cell(text string, width int) string {
	text = fit(text, width)
	return text + strings.Repeat(" ", max(0, width-ansi.StringWidth(text)))
}

// Keep status and agent badges visible, with all three identity columns even
// in a narrow panel. Extra space primarily goes to the title.
func memberColumns(width int) (title, ticket, crew int) {
	ticket = min(12, max(6, width/6))
	crew = min(20, max(6, width/5))
	title = max(1, width-9-ticket-crew)
	return
}
func (m Model) memberTableHeader(width int) string {
	title, ticket, crew := memberColumns(width)
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("8")).Render(
		cell("ST AG", 7) + cell("TITLE", title) + " " + cell("TICKET", ticket) + " " + cell("CREW", crew))
}
func selectionMarker(selected bool) string {
	if selected {
		return "▌▌"
	}
	return "▌ "
}

func (m Model) memberTableRow(r member.Row, width int, selected bool) string {
	title, ticket, crew := memberColumns(width)
	icon, statusColor := statusIcon(r.CurrentStatus())
	badge, badgeColor := palette.Badge(r.Agent)
	prefix := colored(selectionMarker(selected), member.Color(r.Manifest, m.crews)) + lipgloss.NewStyle().Foreground(statusColor).Render(icon) + " " + colored(badge, badgeColor) + " "
	name := r.Name
	if name == "" {
		name = r.ID
	}
	label, target := ticketLink(r)
	c := m.crewFor(r.Crew)
	return prefix + cell(clean(name), title) + " " + link(cell(label, ticket), target) + " " + colored(cell(clean(c.Title), crew), palette.Resolve(c.ID, c.Color, ""))
}

// Explicit web links work for any tracker. Numeric tickets can be resolved
// for known repository hosts; leave other identifiers as text rather than
// inventing tracker URLs. Never emit terminal controls from persisted values.
// safeWebURL admits only absolute http(s) URLs without credentials or control
// characters; nothing else is linked or handed to open/xdg-open.
func safeWebURL(raw string) *url.URL {
	if strings.IndexFunc(raw, unicode.IsControl) >= 0 {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
		return nil
	}
	return u
}

func ticketLink(r member.Row) (string, string) {
	ticket := strings.TrimSpace(r.Ticket)
	if ticket == "" {
		return "—", ""
	}
	if u := safeWebURL(ticket); u != nil {
		label := "Link ↗"
		parts := strings.Split(strings.TrimRight(u.Path, "/"), "/")
		if last := parts[len(parts)-1]; digits(last) {
			label = "#" + last + " ↗"
		}
		return label, u.String()
	}
	number := strings.TrimPrefix(ticket, "#")
	if !digits(number) {
		return clean(ticket), ""
	}
	remote := r.RemoteURL
	if strings.HasPrefix(remote, "git@") {
		host, path, ok := strings.Cut(strings.TrimPrefix(remote, "git@"), ":")
		if ok {
			remote = "https://" + host + "/" + path
		}
	} else if strings.HasPrefix(remote, "ssh://git@") {
		remote = "https://" + strings.TrimPrefix(remote, "ssh://git@")
	}
	u := safeWebURL(remote)
	if u == nil {
		return "#" + number, ""
	}
	u.Path = strings.TrimSuffix(strings.TrimRight(u.Path, "/"), ".git")
	u.RawPath, u.RawQuery, u.Fragment = "", "", ""
	if len(strings.Split(strings.Trim(u.Path, "/"), "/")) < 2 {
		return "#" + number, ""
	}
	switch u.Host {
	case "github.com":
		u.Path += "/issues/" + number
	case "gitlab.com":
		u.Path += "/-/issues/" + number
	default:
		return "#" + number, ""
	}
	return "#" + number + " ↗", u.String()
}
func digits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func (m Model) listLayout(height, width int) memberListLayout {
	top := selectionMarker(m.overview) + "Overview · actions (Home)"
	style := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	layout := memberListLayout{fixed: []memberListLine{{style.Render(cell(top, width)), overviewEntry}}}
	selectedLine := 0
	if m.group == "crew" {
		for i, e := range m.crewEntries() {
			selected := !m.overview && i == m.crewCursor
			text := m.crewEntryLine(e, width, selected)
			if selected {
				selectedLine = i
			}
			layout.body = append(layout.body, memberListLine{text, i})
		}
		if len(layout.body) == 0 {
			layout.body = append(layout.body, memberListLine{"No live crews. h shows inactive.", listHeading})
		}
	} else {
		layout.fixed = append(layout.fixed, memberListLine{m.memberTableHeader(width), listHeading})
		lastSection, lastCrew := "", ""
		for i, r := range m.rows {
			group := section(r)
			if m.group == "repo" {
				group = clean(r.Repo) + " · " + group
			}
			if group != lastSection {
				if len(layout.body) > 0 {
					layout.body = append(layout.body, memberListLine{"", listHeading})
				}
				layout.body = append(layout.body, memberListLine{lipgloss.NewStyle().Bold(true).Render(group), listHeading})
				lastSection, lastCrew = group, ""
			}
			c := m.crewFor(r.Crew)
			if c.ID != lastCrew {
				layout.body = append(layout.body, memberListLine{colored("  "+clean(c.Title), palette.Resolve(c.ID, c.Color, "")), listHeading})
				lastCrew = c.ID
			}
			selected := !m.overview && i == m.selected
			line := m.memberTableRow(r, width, selected)
			if selected {
				selectedLine = len(layout.body)
			}
			layout.body = append(layout.body, memberListLine{line, i})
		}
		if len(m.rows) == 0 {
			text := "No members yet."
			if m.query.Value() != "" {
				text = "No matches. Esc clears filter."
			}
			layout.body = append(layout.body, memberListLine{text, listHeading})
		}
	}
	available := max(1, height-len(layout.fixed))
	if !m.overview {
		layout.start = max(0, selectedLine-available+1)
	}
	return layout
}
func (m Model) listView(height, width int) string {
	layout := m.listLayout(height, width)
	lines := make([]string, 0, height)
	for _, row := range layout.fixed {
		lines = append(lines, fit(row.text, width))
	}
	for i := layout.start; i < len(layout.body) && len(lines) < height; i++ {
		lines = append(lines, fit(layout.body[i].text, width))
	}
	return strings.Join(lines, "\n")
}
