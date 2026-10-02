package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/pelletier/go-toml/v2"
	"github.com/thomashartm/motley/internal/crew"
	"github.com/thomashartm/motley/internal/gh"
	"github.com/thomashartm/motley/internal/member"
	"github.com/thomashartm/motley/internal/state"
)

func TestPRFormatting(t *testing.T) {
	now := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		g           *member.PRInfo
		short, long string
	}{
		{nil, "—", ""},
		{&member.PRInfo{FetchedAt: now.Add(-3 * time.Minute)}, "—", "No PR (refreshed 3m ago)"},
		{&member.PRInfo{PR: 7, State: "OPEN", Draft: true, Checks: "FAILURE", Review: "REVIEW_REQUIRED", FetchedAt: now.Add(-time.Hour)}, "#7 draft ✗", "PR #7 open · draft · checks ✗ failing · review REVIEW_REQUIRED (refreshed 1h ago)"},
		{&member.PRInfo{PR: 231, State: "OPEN", Checks: "SUCCESS", Review: "APPROVED", FetchedAt: now}, "#231 ✔", "PR #231 open · checks ✔ passing · review APPROVED (refreshed 0s ago)"},
		{&member.PRInfo{PR: 231, State: "MERGED", Checks: "SUCCESS", FetchedAt: now}, "#231 merged", "PR #231 merged (refreshed 0s ago)"},
		{&member.PRInfo{PR: 9, State: "CLOSED", Draft: true, FetchedAt: now}, "#9 closed", "PR #9 closed (refreshed 0s ago)"},
		{&member.PRInfo{PR: 9, State: "OPEN", Checks: "PENDING", FetchedAt: now}, "#9 …", "PR #9 open · checks … pending (refreshed 0s ago)"},
		{&member.PRInfo{PR: 9, State: "OPEN", Checks: "NONE", FetchedAt: now.Add(time.Minute)}, "#9", "PR #9 open (refreshed 0s ago)"},
	} {
		if s, l := prShort(tt.g), prLong(tt.g, now); s != tt.short || l != tt.long {
			t.Errorf("%+v:\n%q %q\nwant\n%q %q", tt.g, s, l, tt.short, tt.long)
		}
	}
}

func writeManifest(t *testing.T, m member.Manifest) {
	t.Helper()
	dir, err := state.MembersDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	m.Schema = 1
	data, err := toml.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.WriteAtomic(filepath.Join(dir, m.ID+".toml"), data); err != nil {
		t.Fatal(err)
	}
}

func prClient(out string, err error) *gh.Client {
	return gh.New(gh.RunnerFunc(func(context.Context, string, ...string) ([]byte, error) { return []byte(out), err }))
}

func TestRefreshSelectedAndAll(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	r := githubRow("alpha")
	writeManifest(t, r.Manifest)
	m := update(newModel(false, false, "", nil), tea.WindowSizeMsg{Width: 140, Height: 40})
	m = update(m, snapshot{rows: []member.Row{r}})
	m.github = prClient(`[{"number":7,"url":"https://github.com/acme/api/pull/7","state":"OPEN","isDraft":false,"headRefName":"feat/alpha","statusCheckRollup":[]}]`, nil)
	next, cmd := m.Update(key("u"))
	m = next.(Model)
	if cmd == nil || !m.busy || m.busyText != "Refreshing GitHub data for alpha…" {
		t.Fatal(m.busyText)
	}
	m = update(m, cmd())
	if m.busy || m.message != "PR #7 open · alpha" {
		t.Fatalf("message %q", m.message)
	}
	next, cmd = m.Update(key("U"))
	if m = next.(Model); m.busyText != "Refreshing GitHub data for all members…" {
		t.Fatal(m.busyText)
	}
	m = update(m, cmd())
	if m.message != "Refreshed 1 member in 1 repo" {
		t.Fatalf("message %q", m.message)
	}
	m.github = prClient("[]", nil)
	next, cmd = m.Update(key("u"))
	if m = update(next.(Model), cmd()); m.message != "No PR for feat/alpha" {
		t.Fatalf("message %q", m.message)
	}
	m.github = prClient("", gh.ErrMissing)
	next, cmd = m.Update(key("u"))
	if m = update(next.(Model), cmd()); m.message != "GitHub CLI (gh) not found; install gh for issue and PR data" {
		t.Fatalf("hint %q", m.message)
	}
	next, cmd = m.Update(key("U"))
	if m = update(next.(Model), cmd()); m.message != "GitHub CLI (gh) not found; install gh for issue and PR data" {
		t.Fatalf("hint %q", m.message)
	}
}

func TestRefreshNeedsSelectionAndGitHubRemote(t *testing.T) {
	m := update(newModel(false, false, "", nil), tea.WindowSizeMsg{Width: 140, Height: 40})
	m.github = prClient("[]", nil)
	if next, cmd := m.Update(key("u")); cmd != nil || next.(Model).busy {
		t.Fatal("u without a member must do nothing")
	}
	m = update(m, snapshot{rows: []member.Row{row("local", true)}})
	if next, cmd := m.Update(key("u")); cmd != nil || next.(Model).message != "GitHub data needs a GitHub remote" {
		t.Fatalf("%q", next.(Model).message)
	}
}

func TestPRMenuEntriesFollowState(t *testing.T) {
	m := update(newModel(false, false, "", nil), tea.WindowSizeMsg{Width: 140, Height: 40})
	m.github = prClient("[]", nil)
	labels := func(g *member.PRInfo) string {
		r := githubRow("alpha")
		r.GH = g
		m = update(m, snapshot{rows: []member.Row{r}})
		m = update(m, key("P"))
		if m.menu == nil {
			return m.message
		}
		var out []string
		for _, i := range m.menu.items {
			out = append(out, i.label)
		}
		m.menu = nil
		return strings.Join(out, "|")
	}
	if got := labels(nil); got != "Create PR (gh pr create --fill)" {
		t.Fatal(got)
	}
	if got := labels(&member.PRInfo{PR: 7, State: "OPEN", Draft: true, URL: "https://github.com/acme/api/pull/7"}); got != "Mark ready for review|Open PR #7 in browser" {
		t.Fatal(got)
	}
	if got := labels(&member.PRInfo{PR: 7, State: "OPEN", URL: "https://github.com/acme/api/pull/7"}); got != "Open PR #7 in browser" {
		t.Fatal(got)
	}
	if got := labels(&member.PRInfo{PR: 7, State: "MERGED", URL: "https://github.com/acme/api/pull/7"}); got != "Create PR (gh pr create --fill)|Open PR #7 in browser" {
		t.Fatal(got)
	}
	if got := labels(&member.PRInfo{PR: 7, State: "OPEN", URL: "-flag"}); got != "" && strings.Contains(got, "Open PR") {
		t.Fatal("a non-web PR URL must not be offered: " + got)
	}
	m = update(m, snapshot{rows: []member.Row{row("local", true)}})
	m = update(m, key("P"))
	if m.menu != nil || m.message != "PR actions need a GitHub remote" {
		t.Fatal(m.message)
	}
}

func TestPRMenuRunsActions(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	r := githubRow("alpha")
	r.GH = &member.PRInfo{PR: 7, State: "OPEN", Draft: true, URL: "https://github.com/acme/api/pull/7"}
	writeManifest(t, r.Manifest)
	m := update(newModel(false, false, "", nil), tea.WindowSizeMsg{Width: 140, Height: 40})
	m = update(m, snapshot{rows: []member.Row{r}})
	var calls []string
	m.github = gh.New(gh.RunnerFunc(func(_ context.Context, _ string, args ...string) ([]byte, error) {
		calls = append(calls, strings.Join(args, " "))
		return []byte("[]"), nil
	}))
	m = update(m, key("P"))
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m = next.(Model); m.busyText != "Marking ready…" || cmd == nil {
		t.Fatal(m.busyText)
	}
	if m = update(m, cmd()); m.message != "PR #7 is ready for review" || calls[0] != "pr ready 7 --repo acme/api" {
		t.Fatalf("%q %v", m.message, calls)
	}

	var opened []string
	m.openURL = func(u string) error { opened = append(opened, u); return nil }
	m = update(m, key("P"))
	m = update(m, tea.KeyMsg{Type: tea.KeyDown})
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = update(next.(Model), cmd())
	if len(opened) != 1 || opened[0] != "https://github.com/acme/api/pull/7" {
		t.Fatal(opened)
	}

	m.github = prClient("", errors.New("gh: a pull request for branch \"feat/alpha\" already exists"))
	r.GH = nil
	m = update(m, snapshot{rows: []member.Row{r}})
	m = update(m, key("P"))
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m = next.(Model); m.busyText != "Creating PR…" {
		t.Fatal(m.busyText)
	}
	if m = update(m, cmd()); !strings.Contains(m.message, "already exists") || m.busy {
		t.Fatal(m.message)
	}
}

func TestPRColumnAndDetail(t *testing.T) {
	m := update(newModel(false, false, "", nil), tea.WindowSizeMsg{Width: 180, Height: 40})
	r := githubRow("alpha")
	r.Crew = "fx"
	r.GH = &member.PRInfo{PR: 7, State: "OPEN", Draft: true, Checks: "FAILURE", URL: "https://github.com/acme/api/pull/7", FetchedAt: time.Now()}
	m = update(m, snapshot{rows: []member.Row{r}, crews: []crew.Crew{{ID: "fx", Title: "FX", Color: "blue"}}})
	if links := m.memberLinks(r); len(links) != 3 || links[2].label != "PR #7" || links[2].url != r.GH.URL {
		t.Fatalf("%+v", links)
	}
	detail := m.detail.View()
	if !strings.Contains(detail, "\x1b]8;;https://github.com/acme/api/pull/7\x1b\\") || !strings.Contains(ansi.Strip(detail), "PR #7 open · draft · checks ✗ failing") {
		t.Fatalf("PR field not linked:\n%s", ansi.Strip(detail))
	}
	m = update(m, key("g"))
	if table := ansi.Strip(m.crewTable(20, 100)); !strings.Contains(table, " PR") || !strings.Contains(table, "#7 draft ✗") {
		t.Fatal(table)
	}
	if narrow := ansi.Strip(m.crewTable(20, 70)); strings.Contains(narrow, "#7 draft") || !strings.Contains(narrow, "SINCE") {
		t.Fatal("PR column must drop first when narrow:\n" + narrow)
	}
}

func TestRetireDialogShowsOpenPR(t *testing.T) {
	m := update(newModel(false, false, "", nil), tea.WindowSizeMsg{Width: 140, Height: 40})
	r := githubRow("alpha")
	m = update(m, snapshot{rows: []member.Row{r}})
	m.retiring = &retireDialog{id: "alpha"}
	m = update(m, retireChecked{id: "alpha", check: member.RetireCheck{Manifest: r.Manifest, OpenPR: &gh.PR{Number: 7, URL: "https://github.com/acme/api/pull/7"}}})
	if view := ansi.Strip(m.View()); !strings.Contains(view, "Open PR #7 stays open on GitHub") {
		t.Fatal(view)
	}
}
