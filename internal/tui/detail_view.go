package tui

import (
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/thomashartm/motley/internal/gitx"
	"github.com/thomashartm/motley/internal/member"
	"github.com/thomashartm/motley/internal/tmux"
)

type detailField struct{ label, value string }

// Labels share one column; wrapped values stay in their own column. Narrow
// panels stack labels above indented values, matching the editor layout.
func detailFields(fields []detailField, width int) string {
	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("8"))
	var blocks []string
	for _, field := range fields {
		value := field.value
		if value == "" {
			value = "—"
		}
		if width < 40 {
			wrapped := wrapLinkedValue(value, max(1, width-2))
			for i := range wrapped {
				wrapped[i] = "  " + wrapped[i]
			}
			blocks = append(blocks, labelStyle.Render(field.label+":")+"\n"+strings.Join(wrapped, "\n"))
		} else {
			wrapped := wrapLinkedValue(value, max(1, width-15))
			wrapped[0] = labelStyle.Render(cell(field.label+":", 13)) + "  " + wrapped[0]
			for i := 1; i < len(wrapped); i++ {
				wrapped[i] = strings.Repeat(" ", 15) + wrapped[i]
			}
			blocks = append(blocks, strings.Join(wrapped, "\n"))
		}
	}
	return strings.Join(blocks, "\n")
}

// branchFields link the branch and its compare view when origin is on GitHub.
func branchFields(r member.Row) []detailField {
	gh, ok := gitx.WebURL(r.RemoteURL)
	if !ok || r.Branch == "" {
		return []detailField{{"Branch", clean(r.Branch)}}
	}
	fields := []detailField{{"Branch", link(clean(r.Branch), gh.BranchURL(r.Branch))}}
	if r.Base != "" {
		fields = append(fields, detailField{"Compare", link(clean(r.Base+"..."+r.Branch), gh.CompareURL(r.Base, r.Branch))})
	}
	return fields
}

func (m Model) memberDetails() string {
	r := m.selectedRow()
	width := m.detailWidth()
	name := r.Name
	if name == "" {
		name = r.ID
	}
	title := colored("▌ "+clean(name), member.Color(r.Manifest, m.crews))
	status := r.CurrentStatus()
	icon, sc := statusIcon(status)
	ticket, ticketURL := ticketLink(r)
	// The recorded issue adds its title and its canonical URL to the ticket.
	if r.Issue != nil && safeWebURL(r.Issue.URL) != nil {
		ticket = strings.TrimSpace(strings.TrimSuffix(ticket, " ↗")+" "+clean(r.Issue.Title)) + " ↗"
		ticketURL = r.Issue.URL
	}
	fields := []detailField{
		{"Status", lipgloss.NewStyle().Foreground(sc).Render(icon+" "+status) + " · " + since(r)},
		{"Ticket", link(ticket, ticketURL)},
		{"Crew", m.crewLabel(r.Crew)},
	}
	if c := m.crewFor(r.Crew); c.Gig != "" {
		fields = append(fields, detailField{"Gig", clean(c.Gig)})
	}
	fields = append(fields, branchFields(r)...)
	if r.GH != nil {
		value := clean(prLong(r.GH, time.Now()))
		if r.GH.PR != 0 && safeWebURL(r.GH.URL) != nil {
			value = link(value, r.GH.URL)
		}
		fields = append(fields, detailField{"PR", value})
	}
	fields = append(fields, detailField{"Agent", coloredBadge(r.Agent) + " " + clean(r.Agent)})
	if r.Blueprint != "" {
		fields = append(fields, detailField{"Blueprint", clean(r.Blueprint)})
	}
	body := fit(title, width) + "\n" + detailFields(fields, width)
	if m.event.Status == status {
		text := m.event.Summary
		switch status {
		case "question":
			if q := m.event.Detail["question"]; q != "" {
				text = q
			}
		case "permission":
			if tool := m.event.Detail["tool"]; tool != "" {
				text = tool + ": " + m.event.Detail["input"] + "\n\n" + text
			}
		}
		if text != "" {
			body = fit(title, width) + "\n" + ansi.Wrap(multiline(text), width, "") + "\n\n" + panelDivider(width) + "\n\n" + detailFields(fields, width)
		}
	}
	section := func(label string, fields []detailField) {
		body += "\n\n" + lipgloss.NewStyle().Bold(true).Render(label) + "\n" + panelDivider(width) + "\n\n" + detailFields(fields, width)
	}
	if status == "ready" && m.gitDetail != "" {
		section("Changes", []detailField{{"Git", multiline(m.gitDetail)}})
	}
	section("Workspace", []detailField{{"Repository", clean(r.Repo)}, {"Base", clean(r.Base)}, {"Worktree", clean(r.Worktree)}, {"Main repo", clean(r.RepoPath)}, {"Remote", clean(r.RemoteURL)}})
	var clients []string
	for _, c := range m.clients {
		if c.Session == tmux.SessionName(r.ID) {
			clients = append(clients, clean(c.Name))
		}
	}
	session := []detailField{{"ID", clean(r.ID)}, {"Created", r.CreatedAt.Local().Format("2006-01-02 15:04 MST")}, {"Tabs", strings.Join(clients, ", ")}}
	if r.ClaudeSession != "" {
		session = append(session, detailField{"Claude ID", clean(r.ClaudeSession)}, detailField{"Checkout", "Imported; files are kept on retirement."})
		if r.External {
			session = append(session, detailField{"Terminal", "Runs in its original terminal; Terminate and Revive to run it in Motley."})
		}
	}
	section("Session", session)

	return body
}
