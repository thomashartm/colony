package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/thomashartm/motley/internal/gitx"
	"github.com/thomashartm/motley/internal/member"
)

// githubDone ends a gh action; err wins over text.
type githubDone struct {
	text string
	err  error
}

var (
	checkMarks = map[string]string{"SUCCESS": "✔", "FAILURE": "✗", "PENDING": "…"}
	checkWords = map[string]string{"SUCCESS": "passing", "FAILURE": "failing", "PENDING": "pending"}
)

// prShort is the crew table cell: #7 draft ✗, #231 ✔, #231 merged or #9 closed.
func prShort(g *member.PRInfo) string {
	if g == nil || g.PR == 0 {
		return "—"
	}
	s := fmt.Sprintf("#%d", g.PR)
	switch g.State {
	case "MERGED":
		return s + " merged"
	case "CLOSED":
		return s + " closed"
	}
	if g.Draft {
		s += " draft"
	}
	if mark := checkMarks[g.Checks]; mark != "" {
		s += " " + mark
	}
	return s
}

// prLong is the details line, including how old the data is.
func prLong(g *member.PRInfo, now time.Time) string {
	if g == nil {
		return ""
	}
	age := " (refreshed " + elapsed(now.Sub(g.FetchedAt)) + " ago)"
	if g.PR == 0 {
		return "No PR" + age
	}
	parts := []string{fmt.Sprintf("PR #%d %s", g.PR, strings.ToLower(g.State))}
	if g.State == "OPEN" {
		if g.Draft {
			parts = append(parts, "draft")
		}
		if mark := checkMarks[g.Checks]; mark != "" {
			parts = append(parts, "checks "+mark+" "+checkWords[g.Checks])
		}
		if g.Review != "" {
			parts = append(parts, "review "+g.Review)
		}
	}
	return strings.Join(parts, " · ") + age
}

func onGitHub(r member.Row) bool {
	_, ok := gitx.WebURL(r.RemoteURL)
	return ok && r.Branch != ""
}

// githubCommand runs do in the background with a timeout and reports its result.
func githubCommand(timeout time.Duration, do func(ctx context.Context) (string, error)) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		text, err := do(ctx)
		return githubDone{text: text, err: err}
	}
}

func (m Model) refreshSelected() (tea.Model, tea.Cmd) {
	id := m.selectedID()
	if id == "" || m.github == nil {
		return m, nil
	}
	if !onGitHub(m.selectedRow()) {
		m.message = "GitHub data needs a GitHub remote"
		return m, nil
	}
	client := m.github
	m.busy, m.busyText = true, "Refreshing GitHub data for "+id+"…"
	return m, githubCommand(member.LookupTimeout, func(ctx context.Context) (string, error) {
		got, err := member.RefreshGitHub(ctx, client, id)
		if err != nil {
			return "", err
		}
		if got.GH.PR == 0 {
			return "No PR for " + got.Branch, nil
		}
		return fmt.Sprintf("PR #%d %s · %s", got.GH.PR, strings.ToLower(got.GH.State), id), nil
	})
}

func (m Model) refreshAll() (tea.Model, tea.Cmd) {
	if m.github == nil {
		return m, nil
	}
	client := m.github
	m.busy, m.busyText = true, "Refreshing GitHub data for all members…"
	// One gh call per repository; allow a few repositories their full timeout.
	return m, githubCommand(4*member.LookupTimeout, func(ctx context.Context) (string, error) {
		sum, err := member.RefreshAllGitHub(ctx, client)
		if err != nil {
			return "", err
		}
		text := fmt.Sprintf("Refreshed %d %s in %d %s", sum.Members, plural(sum.Members, "member"), sum.Repos, plural(sum.Repos, "repo"))
		if len(sum.Failures) > 0 {
			text += " · failed: " + strings.Join(sum.Failures, "; ")
		}
		return text, nil
	})
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

// beginPRMenu offers the PR actions the member's last refreshed state allows.
func (m Model) beginPRMenu() (tea.Model, tea.Cmd) {
	id := m.selectedID()
	if id == "" || m.github == nil {
		return m, nil
	}
	r := m.selectedRow()
	if !onGitHub(r) {
		m.message = "PR actions need a GitHub remote"
		return m, nil
	}
	client, g := m.github, r.GH
	action := func(busy string, timeout time.Duration, do func(ctx context.Context) (string, error)) func(Model) (tea.Model, tea.Cmd) {
		return func(m Model) (tea.Model, tea.Cmd) {
			m.busy, m.busyText = true, busy
			return m, githubCommand(timeout, do)
		}
	}
	d := &menuDialog{title: "Pull request · " + clean(r.Branch)}
	if g == nil || g.PR == 0 || g.State != "OPEN" {
		d.items = append(d.items, menuItem{label: "Create PR (gh pr create --fill)", run: action("Creating PR…", member.CreateTimeout, func(ctx context.Context) (string, error) {
			url, warning, err := member.CreatePR(ctx, client, id)
			if err != nil {
				return "", err
			}
			if warning != "" {
				return "Created " + url + " · " + warning, nil
			}
			return "Created " + url, nil
		})})
	}
	if g != nil && g.State == "OPEN" && g.Draft {
		number := g.PR
		d.items = append(d.items, menuItem{label: "Mark ready for review", run: action("Marking ready…", member.LookupTimeout, func(ctx context.Context) (string, error) {
			if err := member.MarkPRReady(ctx, client, id); err != nil {
				return "", err
			}
			return fmt.Sprintf("PR #%d is ready for review", number), nil
		})})
	}
	if g != nil && g.PR != 0 && safeWebURL(g.URL) != nil {
		target := g.URL
		d.items = append(d.items, menuItem{label: fmt.Sprintf("Open PR #%d in browser", g.PR), run: func(m Model) (tea.Model, tea.Cmd) { return m.openLink(target) }})
	}
	m.menu, m.message = d, ""
	return m, nil
}
