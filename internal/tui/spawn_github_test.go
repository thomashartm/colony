package tui

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/thomashartm/motley/internal/crew"
	"github.com/thomashartm/motley/internal/gh"
	"github.com/thomashartm/motley/internal/member"
)

// fakeGitHub answers every gh call with out, or fails with err.
func fakeGitHub(out string, err error) *gh.Client {
	return gh.New(gh.RunnerFunc(func(context.Context, string, ...string) ([]byte, error) { return []byte(out), err }))
}

func identityForm(t *testing.T) Model {
	t.Helper()
	m := update(newModel(false, false, "", nil), tea.WindowSizeMsg{Width: 140, Height: 40})
	m.github = fakeGitHub("", nil)
	m.spawn = &spawnForm{step: identityStep, fields: inputs("412", "FX", "feat/412-fx")}
	m.spawn.opts.Repo = "api"
	return m
}

func submitIdentity(t *testing.T, m Model) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	return next.(Model), cmd
}

func TestSpawnLooksUpIssueThenOffersCrew(t *testing.T) {
	m, cmd := submitIdentity(t, identityForm(t))
	if cmd == nil || !m.busy || m.busyText != "Looking up issue #412…" {
		t.Fatalf("lookup not started: busy=%v %q", m.busy, m.busyText)
	}
	issue := &member.IssueContext{Number: 412, Title: "Cache FX", Suggestion: &member.CrewSuggestion{Title: "Epic: FX", URL: "https://github.com/o/r/issues/400", Source: "parent issue"}}
	m = update(m, issueLooked{issue: issue})
	view := ansi.Strip(m.spawnView(30))
	if m.busy || m.spawn.step != crewStep || !strings.Contains(view, "Spawn · Crew") || !strings.Contains(view, `> Create crew "Epic: FX" and assign`) || !strings.Contains(view, "Suggested by the parent issue of #412") {
		t.Fatalf("crew step:\n%s", view)
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.spawn.step != agentStep || !m.spawn.opts.CreateCrew || m.spawn.opts.Issue != issue || !m.spawn.opts.NoGH {
		t.Fatalf("create choice: %+v", m.spawn.opts)
	}
}

func TestSpawnCrewStepMatchAndSkip(t *testing.T) {
	for _, tt := range []struct {
		name  string
		moves int
		check func(member.SpawnOptions) bool
	}{
		{"assign", 0, func(o member.SpawnOptions) bool { return o.Crew == "fx" && !o.CreateCrew && !o.SkipSuggestion }},
		{"skip", 1, func(o member.SpawnOptions) bool { return o.Crew == "" && !o.CreateCrew && o.SkipSuggestion }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := identityForm(t)
			m.crews = []crew.Crew{{ID: "fx", Title: "FX crew", Color: "blue"}}
			m, _ = submitIdentity(t, m)
			m = update(m, issueLooked{issue: &member.IssueContext{Number: 412, Suggestion: &member.CrewSuggestion{Title: "Epic", URL: "u", Source: "milestone", CrewID: "fx"}}})
			if view := ansi.Strip(m.spawnView(30)); !strings.Contains(view, "> Assign crew FX crew") || !strings.Contains(view, "No crew") {
				t.Fatal(view)
			}
			for range tt.moves {
				m = update(m, tea.KeyMsg{Type: tea.KeyDown})
			}
			m = update(m, tea.KeyMsg{Type: tea.KeyEnter})
			if m.spawn.step != agentStep || !tt.check(m.spawn.opts) {
				t.Fatalf("%+v", m.spawn.opts)
			}
		})
	}
}

func TestSpawnLookupFailureWarnsAndContinues(t *testing.T) {
	m, _ := submitIdentity(t, identityForm(t))
	m = update(m, issueLooked{err: gh.ErrAuth})
	view := ansi.Strip(m.spawnView(30))
	if m.busy || m.spawn.step != agentStep || !m.spawn.opts.NoGH || !strings.Contains(strings.Join(strings.Fields(view), " "), "Issue lookup skipped: GitHub CLI is not authenticated; run gh auth login") {
		t.Fatalf("%d\n%s", m.spawn.step, view)
	}
}

func TestSpawnIssueWithoutSuggestionGoesToAgent(t *testing.T) {
	m, _ := submitIdentity(t, identityForm(t))
	m = update(m, issueLooked{issue: &member.IssueContext{Number: 412, Title: "Cache FX"}})
	if m.spawn.step != agentStep || m.spawn.opts.Issue == nil || !m.spawn.opts.NoGH {
		t.Fatalf("%d %+v", m.spawn.step, m.spawn.opts)
	}
}

func TestSpawnExplicitlyClosedFormIgnoresLateLookup(t *testing.T) {
	m, _ := submitIdentity(t, identityForm(t))
	m.spawn = nil
	m = update(m, issueLooked{issue: &member.IssueContext{Number: 412}})
	if m.spawn != nil {
		t.Fatal("late lookup reopened the form")
	}
}

func TestNonIssueTicketSkipsLookup(t *testing.T) {
	for _, ticket := range []string{"PROJ-1", ""} {
		m := identityForm(t)
		m.spawn.fields[0].SetValue(ticket)
		next, cmd := submitIdentity(t, m)
		if cmd != nil || next.busy || next.spawn.step != agentStep {
			t.Fatalf("ticket %q must go straight to the agent step", ticket)
		}
	}
}

func TestSpawnPreviewSummarisesIssueAndCrew(t *testing.T) {
	m := identityForm(t)
	issue := &member.IssueContext{Number: 412, Title: "Cache FX"}
	plan := member.Prepared{Manifest: member.Manifest{Repo: "api", Branch: "feat/412-fx", Agent: "claude"}, Issue: issue, NewCrew: &crew.Crew{Title: "Epic: FX"}}
	m = update(m, spawnPrepared{plan: plan})
	if view := ansi.Strip(m.spawnView(30)); !strings.Contains(view, "api @ feat/412-fx · claude\n#412 Cache FX · new crew Epic: FX\n> Launch") {
		t.Fatal(view)
	}
	if m.spawn.preview.Height != m.contentHeight()-5 {
		t.Fatalf("preview height %d ignores the context line (content %d)", m.spawn.preview.Height, m.contentHeight())
	}
	m.crews = []crew.Crew{{ID: "fx", Title: "FX crew", Color: "blue"}}
	plan.NewCrew, plan.Manifest.Crew = nil, "fx"
	m = update(m, spawnPrepared{plan: plan})
	if view := ansi.Strip(m.spawnView(30)); !strings.Contains(view, "#412 Cache FX · crew FX crew") {
		t.Fatal(view)
	}
	m = update(m, spawnPrepared{plan: member.Prepared{Manifest: member.Manifest{Repo: "api", Branch: "feat/412-fx", Agent: "claude"}}})
	if view := ansi.Strip(m.spawnView(30)); strings.Contains(view, "crew") || m.spawn.preview.Height != m.contentHeight()-4 {
		t.Fatalf("plain spawn has no context line (height %d):\n%s", m.spawn.preview.Height, view)
	}
}

func TestIssueLinkAndDetail(t *testing.T) {
	m := update(newModel(false, false, "", nil), tea.WindowSizeMsg{Width: 160, Height: 40})
	r := githubRow("alpha")
	r.Ticket = "412"
	r.Issue = &member.IssueRef{Title: "Cache FX", URL: "https://github.com/acme/api/issues/412"}
	m = update(m, snapshot{rows: []member.Row{r}})
	links := m.memberLinks(r)
	if len(links) != 3 || links[2].label != "Issue #412 Cache FX" || links[2].url != r.Issue.URL {
		t.Fatalf("%+v", links)
	}
	detail := m.detail.View()
	if !strings.Contains(detail, "\x1b]8;;https://github.com/acme/api/issues/412\x1b\\") || !strings.Contains(ansi.Strip(detail), "#412 Cache FX ↗") {
		t.Fatalf("issue title not linked in details:\n%s", ansi.Strip(detail))
	}

	// Without a recorded issue the numeric ticket still links to GitHub.
	r.Issue = nil
	links = m.memberLinks(r)
	if len(links) != 3 || links[2].label != "Ticket #412" || links[2].url != "https://github.com/acme/api/issues/412" {
		t.Fatalf("%+v", links)
	}
	// A recorded issue URL that is not a web URL is never offered.
	r.Issue = &member.IssueRef{Title: "Bad", URL: "-flag"}
	if links = m.memberLinks(r); len(links) != 3 || links[2].label != "Ticket #412" {
		t.Fatalf("%+v", links)
	}
}

func TestCrewEditorFetchesMissingTitle(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", filepath.Join(t.TempDir(), "state"))
	m := update(newModel(false, false, "", nil), tea.WindowSizeMsg{Width: 140, Height: 40})
	m.github = fakeGitHub(`{"data":{"repository":{"issue":{"title":"Epic: FX","body":"","url":"https://github.com/o/r/issues/400"}}}}`, nil)
	m.editor = newEditor("add", "", addCrewLabels, []string{"", "https://github.com/o/r/issues/400", "", ""})
	m.editor.focus = len(m.editor.fields)
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m = next.(Model); cmd == nil || m.busyText != "Fetching crew title…" {
		t.Fatalf("save not started: %q", m.busyText)
	}
	saved, ok := cmd().(identitySaved)
	if !ok || saved.err != nil || len(saved.crews) != 1 || saved.crews[0].Title != "Epic: FX" || saved.crews[0].ID != "epic-fx" {
		t.Fatalf("%+v", saved)
	}

	m = update(m, saved)
	m.github = fakeGitHub("", gh.ErrMissing)
	m.editor = newEditor("add", "", addCrewLabels, []string{"", "https://github.com/o/r/issues/401", "", ""})
	m.editor.focus = len(m.editor.fields)
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("second save not started")
	}
	if saved, ok := cmd().(identitySaved); !ok || !errors.Is(saved.err, gh.ErrMissing) || len(saved.crews) != 1 {
		t.Fatalf("%+v", saved)
	}
}
