package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/thomashartm/motley/internal/gitx"
	"github.com/thomashartm/motley/internal/member"
)

// menuDialog is a short list of actions for the selected member.
type menuDialog struct {
	title string
	items []menuItem
	focus int
}

type menuItem struct {
	label string
	run   func(Model) (tea.Model, tea.Cmd)
}

func (m Model) updateMenu(key string) (tea.Model, tea.Cmd) {
	d := *m.menu
	m.menu = &d
	switch key {
	case "up", "k", "shift+tab":
		d.focus = (d.focus + len(d.items) - 1) % len(d.items)
	case "down", "j", "tab":
		d.focus = (d.focus + 1) % len(d.items)
	case "esc", "q", "left":
		m.menu = nil
	case "enter":
		item := d.items[d.focus]
		m.menu = nil
		return item.run(m)
	}
	return m, nil
}

// menuView lists items after the title and a blank line; mouse.go relies on that offset.
func (m Model) menuView(height int) string {
	lines := []string{fit(m.menu.title, m.detailWidth()), ""}
	for i, item := range m.menu.items {
		lines = append(lines, fit(control(clean(item.label), i == m.menu.focus), m.detailWidth()))
	}
	return strings.Join(lines[:min(len(lines), height)], "\n")
}

type memberLink struct{ label, url string }

// memberLinks lists the browser targets of r in menu order.
func (m Model) memberLinks(r member.Row) []memberLink {
	var links []memberLink
	add := func(label, u string) {
		if safeWebURL(u) != nil {
			links = append(links, memberLink{label, u})
		}
	}
	if gh, ok := gitx.WebURL(r.RemoteURL); ok && r.Branch != "" {
		add("Branch "+r.Branch, gh.BranchURL(r.Branch))
		if r.Base != "" {
			add("Compare "+r.Base+"..."+r.Branch, gh.CompareURL(r.Base, r.Branch))
		}
	}
	if label, u := issueLink(r); u != "" {
		add(label, u)
	}
	if r.Crew != "" {
		c := m.crewFor(r.Crew)
		add("Crew "+c.Title, c.URL)
	}
	return links
}

// issueLink names the issue recorded at spawn, else the ticket's own link.
func issueLink(r member.Row) (string, string) {
	if r.Issue != nil && safeWebURL(r.Issue.URL) != nil {
		return strings.TrimSpace("Issue #" + strings.TrimPrefix(strings.TrimSpace(r.Ticket), "#") + " " + r.Issue.Title), r.Issue.URL
	}
	label, u := ticketLink(r)
	return "Ticket " + strings.TrimSuffix(label, " ↗"), u
}

func (m Model) beginLinks() (tea.Model, tea.Cmd) {
	if m.selectedID() == "" {
		return m, nil
	}
	links := m.memberLinks(m.selectedRow())
	if len(links) == 0 {
		m.message = "No links for this member"
		return m, nil
	}
	d := &menuDialog{title: "Open in browser"}
	for _, l := range links {
		target := l.url
		d.items = append(d.items, menuItem{label: l.label, run: func(m Model) (tea.Model, tea.Cmd) { return m.openLink(target) }})
	}
	m.menu, m.message = d, ""
	return m, nil
}
