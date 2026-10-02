package member

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/thomashartm/motley/internal/crew"
	"github.com/thomashartm/motley/internal/gh"
)

func issueClient(t *testing.T, out string, err error) (*gh.Client, *int) {
	t.Helper()
	calls := 0
	return gh.New(gh.RunnerFunc(func(context.Context, string, ...string) ([]byte, error) {
		calls++
		return []byte(out), err
	})), &calls
}

const parentJSON = `{"data":{"repository":{"issue":{"title":"Cache FX","body":"Do it","url":"https://github.com/o/r/issues/412","parent":{"title":"Epic: FX","url":"https://github.com/o/r/issues/400"},"milestone":{"title":"Q4","url":"https://github.com/o/r/milestone/3"}}}}}`
const milestoneJSON = `{"data":{"repository":{"issue":{"title":"T","body":"","url":"https://github.com/o/r/issues/5","parent":null,"milestone":{"title":"Q4","url":"https://github.com/o/r/milestone/3"}}}}}`

func TestIssueNumber(t *testing.T) {
	for ticket, want := range map[string]int{"412": 412, "#412": 412, " 412 ": 412, "PROJ-412": 0, "0": 0, "": 0, "12345678901": 0, "https://github.com/o/r/issues/4": 0} {
		n, ok := IssueNumber(ticket)
		if n != want || ok != (want > 0) {
			t.Errorf("%q: %d %v", ticket, n, ok)
		}
	}
}

func TestLookupIssueSuggestion(t *testing.T) {
	client, _ := issueClient(t, parentJSON, nil)
	got, err := lookupIssue(context.Background(), client, "git@github.com:o/r.git", "#412", []crew.Crew{{ID: "fx", URL: "https://github.com/o/r/issues/400/"}})
	if err != nil || got.Number != 412 || got.Title != "Cache FX" || got.Body != "Do it" || got.Suggestion == nil || got.Suggestion.Source != "parent issue" || got.Suggestion.CrewID != "fx" {
		t.Fatalf("%+v %v", got, err)
	}
	client, _ = issueClient(t, milestoneJSON, nil)
	got, _ = lookupIssue(context.Background(), client, "git@github.com:o/r.git", "5", nil)
	if got.Suggestion == nil || got.Suggestion.Source != "milestone" || got.Suggestion.Title != "Q4" || got.Suggestion.CrewID != "" {
		t.Fatalf("milestone fallback: %+v", got.Suggestion)
	}
}

func TestLookupIssueSkipsQuietly(t *testing.T) {
	client, calls := issueClient(t, parentJSON, nil)
	for _, tc := range []struct{ remote, ticket string }{{"/tmp/origin.git", "412"}, {"git@github.com:o/r.git", "PROJ-1"}, {"https://gitlab.com/o/r.git", "412"}} {
		got, err := lookupIssue(context.Background(), client, tc.remote, tc.ticket, nil)
		if got != nil || err != nil {
			t.Fatalf("%v: %+v %v", tc, got, err)
		}
	}
	if *calls != 0 {
		t.Fatalf("gh called %d times for non-GitHub cases", *calls)
	}
}

func TestCapBody(t *testing.T) {
	exact := strings.Repeat("a", issueBodyLimit)
	if capBody(exact) != exact {
		t.Fatal("body at the limit must be unchanged")
	}
	long := strings.Repeat("a", issueBodyLimit-1) + "€tail"
	got := capBody(long)
	if !utf8.ValidString(got) || !strings.HasSuffix(got, "\n\n(issue body truncated)") || len(got) > issueBodyLimit+len("\n\n(issue body truncated)") || !strings.HasPrefix(got, strings.Repeat("a", issueBodyLimit-1)+"\n") {
		t.Fatalf("len=%d valid=%v", len(got), utf8.ValidString(got))
	}
}

func TestCrewTitle(t *testing.T) {
	client, _ := issueClient(t, parentJSON, nil)
	if title, err := CrewTitle(context.Background(), client, "https://github.com/o/r/issues/412"); err != nil || title != "Cache FX" {
		t.Fatal(title, err)
	}
	project, _ := issueClient(t, `{"title":"Q4 platform"}`, nil)
	if title, err := CrewTitle(context.Background(), project, "https://github.com/orgs/acme/projects/7"); err != nil || title != "Q4 platform" {
		t.Fatal(title, err)
	}
	if _, err := CrewTitle(context.Background(), client, "https://example.com/x"); err == nil || !strings.Contains(err.Error(), "--title is required") {
		t.Fatal(err)
	}
	failing, _ := issueClient(t, "", gh.ErrAuth)
	if _, err := CrewTitle(context.Background(), failing, "https://github.com/o/r/issues/1"); !errors.Is(err, gh.ErrAuth) {
		t.Fatal(err)
	}
}

func TestAddCrewWithLookup(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	client, calls := issueClient(t, parentJSON, nil)
	c, err := AddCrewWithLookup(context.Background(), client, "", "https://github.com/o/r/issues/412", "", "")
	if err != nil || c.Title != "Cache FX" || c.ID != "cache-fx" || *calls != 1 {
		t.Fatalf("%+v %v", c, err)
	}
	c, err = AddCrewWithLookup(context.Background(), client, "Manual", "https://github.com/o/r/issues/412", "", "")
	if err != nil || c.Title != "Manual" || *calls != 1 {
		t.Fatalf("explicit title must not call gh: %+v %v", c, err)
	}
	if _, err := AddCrewWithLookup(context.Background(), client, " ", "", "", ""); err == nil {
		t.Fatal("a crew needs a title or a URL to fetch one from")
	}
}

func TestPrepareIssueLookupAndCrew(t *testing.T) {
	cfg, _, repo, _ := prepareRepo(t, "git@github.com:o/r.git")
	writeBlueprint(t, repo, "issue", "+++\n+++\n{{.Issue.Title}}|{{.Issue.URL}}|{{.Crew.Title}}|{{.Issue.Body}}")
	client, calls := issueClient(t, parentJSON, nil)

	p, err := Prepare(cfg, SpawnOptions{Repo: "api", Branch: "feat/412-fx", Ticket: "412", Blueprint: "issue", GitHub: client})
	if err != nil || p.Prompt != "Cache FX|https://github.com/o/r/issues/412||Do it" || p.Manifest.Issue == nil || p.Manifest.Issue.URL != "https://github.com/o/r/issues/412" || p.NewCrew != nil || p.AutoCrew || *calls != 1 || p.Issue.Suggestion == nil {
		t.Fatalf("no crew yet: %+v %q %v", p, p.Prompt, err)
	}
	p, _ = Prepare(cfg, SpawnOptions{Repo: "api", Branch: "feat/412-fx", Ticket: "412", Blueprint: "issue", GitHub: client, CreateCrew: true})
	if p.NewCrew == nil || p.NewCrew.Title != "Epic: FX" || !strings.Contains(p.Prompt, "|Epic: FX|") {
		t.Fatalf("create crew: %+v %q", p.NewCrew, p.Prompt)
	}
	if _, err := AddCrew("FX epic", "https://github.com/o/r/issues/400", "", ""); err != nil {
		t.Fatal(err)
	}
	p, _ = Prepare(cfg, SpawnOptions{Repo: "api", Branch: "feat/412-fx", Ticket: "412", GitHub: client})
	if !p.AutoCrew || p.Manifest.Crew != "fx-epic" {
		t.Fatalf("auto crew: %+v", p.Manifest)
	}
	p, _ = Prepare(cfg, SpawnOptions{Repo: "api", Branch: "feat/412-fx", Ticket: "412", GitHub: client, Crew: "fx-epic"})
	if p.AutoCrew {
		t.Fatal("an explicit crew is not an automatic choice")
	}
	p, _ = Prepare(cfg, SpawnOptions{Repo: "api", Branch: "feat/412-fx", Ticket: "412", GitHub: client, SkipSuggestion: true})
	if p.AutoCrew || p.Manifest.Crew != "" {
		t.Fatal("skip suggestion")
	}
	before := *calls
	p, _ = Prepare(cfg, SpawnOptions{Repo: "api", Branch: "feat/412-fx", Ticket: "412", GitHub: client, NoGH: true})
	if *calls != before || p.Issue != nil || p.Manifest.Issue != nil {
		t.Fatal("--no-gh must not call gh")
	}
	looked := &IssueContext{Number: 412, Title: "From the TUI", URL: "https://github.com/o/r/issues/412"}
	p, _ = Prepare(cfg, SpawnOptions{Repo: "api", Branch: "feat/412-fx", Ticket: "412", GitHub: client, Issue: looked, NoGH: true})
	if *calls != before || p.Manifest.Issue == nil || p.Manifest.Issue.Title != "From the TUI" {
		t.Fatalf("a caller's lookup must be used as is: %+v", p.Manifest.Issue)
	}
}

func TestPrepareLookupFailureWarns(t *testing.T) {
	cfg, _, _, _ := prepareRepo(t, "git@github.com:o/r.git")
	client, _ := issueClient(t, "", gh.ErrAuth)
	p, err := Prepare(cfg, SpawnOptions{Repo: "api", Branch: "feat/412-fx", Ticket: "412", GitHub: client, CreateCrew: true})
	if err != nil || p.Issue != nil || len(p.Warnings) != 2 || p.Warnings[0] != "issue 412 lookup skipped: GitHub CLI is not authenticated; run gh auth login" || !strings.Contains(p.Warnings[1], "--create-crew ignored") {
		t.Fatalf("%q %v", p.Warnings, err)
	}
}

func TestPrepareHugeIssueBodyStaysUnderPromptCap(t *testing.T) {
	cfg, _, repo, _ := prepareRepo(t, "git@github.com:o/r.git")
	writeBlueprint(t, repo, "issue", "+++\n+++\n{{.Issue.Body}}")
	huge := `{"data":{"repository":{"issue":{"title":"Big","url":"https://github.com/o/r/issues/1","body":"` + strings.Repeat("ü", 40000) + `"}}}}`
	client, _ := issueClient(t, huge, nil)
	p, err := Prepare(cfg, SpawnOptions{Repo: "api", Branch: "feat/1-big", Ticket: "1", Blueprint: "issue", GitHub: client})
	if err != nil || !utf8.ValidString(p.Prompt) || !strings.HasSuffix(p.Prompt, "(issue body truncated)") {
		t.Fatalf("len=%d %v", len(p.Prompt), err)
	}
}

func TestSpawnPreparedCreatesSuggestedCrewOnce(t *testing.T) {
	cfg, root, repo, bin := prepareRepo(t, "ROOT/remote.git")
	if out, err := exec.Command("git", "init", "--bare", filepath.Join(root, "remote.git")).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	if out, err := exec.Command("git", "-C", repo, "push", "-u", "origin", "main").CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	script := "#!/bin/sh\ncase \"$*\" in *list-sessions*) echo 'no server running on fixture' >&2; exit 1;; *list-keys*) echo 'unknown key'; exit 1;; *) exit 0;; esac\n"
	if err := os.WriteFile(filepath.Join(bin, "tmux"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	suggested := &crew.Crew{Title: "Epic: FX", URL: "https://github.com/o/r/issues/400", Kind: "issue"}
	var ids []string
	for _, branch := range []string{"feat/one", "feat/two"} {
		p, err := Prepare(cfg, SpawnOptions{Repo: "api", Branch: branch})
		if err != nil {
			t.Fatal(err)
		}
		p.NewCrew = suggested
		m, err := SpawnPrepared(p, io.Discard)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, m.Crew)
	}
	crews, err := crew.Load()
	if err != nil || len(crews) != 1 || crews[0].ID != "epic-fx" || crews[0].Kind != "issue" || ids[0] != "epic-fx" || ids[1] != "epic-fx" {
		t.Fatalf("crews=%+v ids=%v err=%v", crews, ids, err)
	}
}
