package member

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thomashartm/motley/internal/gh"
	"github.com/thomashartm/motley/internal/state"
)

const prJSON = `[{"number":7,"url":"https://github.com/o/r/pull/7","state":"OPEN","isDraft":true,"reviewDecision":"REVIEW_REQUIRED","headRefName":"feat/a","statusCheckRollup":[{"__typename":"CheckRun","status":"COMPLETED","conclusion":"FAILURE"}]}]`

func ghState(t *testing.T) string {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	dir, err := state.MembersDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func saveTest(t *testing.T, dir string, m Manifest) {
	t.Helper()
	m.Schema, m.Agent = 1, "claude"
	if err := saveManifest(dir, m); err != nil {
		t.Fatal(err)
	}
}

// scripted answers the first matching argv prefix and records every call.
func scripted(t *testing.T, replies map[string]string) (*gh.Client, *[]string) {
	t.Helper()
	var calls []string
	return gh.New(gh.RunnerFunc(func(_ context.Context, _ string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		calls = append(calls, joined)
		for prefix, out := range replies {
			if strings.HasPrefix(joined, prefix) {
				return []byte(out), nil
			}
		}
		return []byte("[]"), nil
	})), &calls
}

func TestRefreshGitHubWritesPRAndIssue(t *testing.T) {
	dir := ghState(t)
	saveTest(t, dir, Manifest{ID: "a", Name: "A", Branch: "feat/a", Ticket: "412", RemoteURL: "git@github.com:o/r.git"})
	client, calls := scripted(t, map[string]string{"pr list": prJSON, "api graphql": `{"data":{"repository":{"issue":{"title":"New title","url":"https://github.com/o/r/issues/412"}}}}`})
	m, err := RefreshGitHub(context.Background(), client, "a")
	if err != nil || m.GH == nil || m.GH.PR != 7 || m.GH.URL != "https://github.com/o/r/pull/7" || !m.GH.Draft || m.GH.Review != "REVIEW_REQUIRED" || m.GH.Checks != "FAILURE" || m.GH.State != "OPEN" || m.GH.FetchedAt.IsZero() || m.Issue == nil || m.Issue.Title != "New title" {
		t.Fatalf("%+v %+v %v", m.GH, m.Issue, err)
	}
	if !strings.Contains((*calls)[0], "--head feat/a --state all") {
		t.Fatal(*calls)
	}
	saved, err := Load(dir, "a")
	if err != nil || saved.GH == nil || saved.GH.PR != 7 || saved.Issue == nil {
		t.Fatalf("not persisted: %+v %v", saved, err)
	}
}

func TestRefreshWithoutPRRecordsTheCheck(t *testing.T) {
	dir := ghState(t)
	saveTest(t, dir, Manifest{ID: "a", Branch: "feat/a", Ticket: "PROJ-1", RemoteURL: "git@github.com:o/r.git"})
	client, calls := scripted(t, nil)
	m, err := RefreshGitHub(context.Background(), client, "a")
	if err != nil || m.GH == nil || m.GH.PR != 0 || m.GH.FetchedAt.IsZero() || len(*calls) != 1 {
		t.Fatalf("%+v %v %v", m.GH, err, *calls)
	}
}

func TestRefreshKeepsConcurrentEdits(t *testing.T) {
	dir := ghState(t)
	saveTest(t, dir, Manifest{ID: "a", Name: "Old", Branch: "feat/a", RemoteURL: "git@github.com:o/r.git"})
	client := gh.New(gh.RunnerFunc(func(context.Context, string, ...string) ([]byte, error) {
		// A user edit lands while gh is running.
		saveTest(t, dir, Manifest{ID: "a", Name: "Renamed", Crew: "fx", Branch: "feat/a", RemoteURL: "git@github.com:o/r.git"})
		return []byte("[]"), nil
	}))
	if _, err := RefreshGitHub(context.Background(), client, "a"); err != nil {
		t.Fatal(err)
	}
	m, _ := Load(dir, "a")
	if m.Name != "Renamed" || m.Crew != "fx" || m.GH == nil || m.GH.PR != 0 {
		t.Fatalf("lost concurrent edit: %+v", m)
	}
}

func TestRefreshFailureKeepsManifest(t *testing.T) {
	dir := ghState(t)
	saveTest(t, dir, Manifest{ID: "a", Branch: "feat/a", RemoteURL: "git@github.com:o/r.git", GH: &PRInfo{PR: 3}})
	client := gh.New(gh.RunnerFunc(func(context.Context, string, ...string) ([]byte, error) { return nil, gh.ErrAuth }))
	if _, err := RefreshGitHub(context.Background(), client, "a"); !errors.Is(err, gh.ErrAuth) {
		t.Fatal(err)
	}
	if m, _ := Load(dir, "a"); m.GH == nil || m.GH.PR != 3 {
		t.Fatalf("failed refresh changed the manifest: %+v", m.GH)
	}
}

func TestRefreshNonGitHub(t *testing.T) {
	dir := ghState(t)
	saveTest(t, dir, Manifest{ID: "a", Branch: "feat/a", RemoteURL: "/tmp/origin.git"})
	client, calls := scripted(t, nil)
	if _, err := RefreshGitHub(context.Background(), client, "a"); err == nil || !strings.Contains(err.Error(), "not on GitHub") || len(*calls) != 0 {
		t.Fatal(err, *calls)
	}
}

func TestRefreshAllBatchesPerRepo(t *testing.T) {
	dir := ghState(t)
	saveTest(t, dir, Manifest{ID: "a", Branch: "feat/a", RemoteURL: "git@github.com:o/r.git"})
	saveTest(t, dir, Manifest{ID: "b", Branch: "feat/b", RemoteURL: "https://github.com/o/r.git"})
	saveTest(t, dir, Manifest{ID: "c", Branch: "feat/c", RemoteURL: "git@github.com:o/other.git"})
	saveTest(t, dir, Manifest{ID: "local", Branch: "feat/l", RemoteURL: "/tmp/x.git"})
	var calls []string
	client := gh.New(gh.RunnerFunc(func(_ context.Context, _ string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		calls = append(calls, joined)
		if strings.Contains(joined, "o/other") {
			return nil, context.DeadlineExceeded
		}
		// The older closed PR on feat/a must not replace the newer open one.
		return []byte(strings.TrimSuffix(prJSON, "]") + `,{"number":2,"state":"CLOSED","headRefName":"feat/a"}]`), nil
	}))
	sum, err := RefreshAllGitHub(context.Background(), client)
	if err != nil || sum.Members != 2 || sum.Repos != 1 || len(sum.Failures) != 1 || !strings.Contains(sum.Failures[0], "o/other") || len(calls) != 2 {
		t.Fatalf("%+v %v %v", sum, err, calls)
	}
	a, _ := Load(dir, "a")
	b, _ := Load(dir, "b")
	l, _ := Load(dir, "local")
	if a.GH == nil || a.GH.PR != 7 || b.GH == nil || b.GH.PR != 0 || l.GH != nil {
		t.Fatalf("a=%+v b=%+v local=%+v", a.GH, b.GH, l.GH)
	}
}

func TestRefreshAllStopsOnMissingGH(t *testing.T) {
	dir := ghState(t)
	saveTest(t, dir, Manifest{ID: "a", Branch: "feat/a", RemoteURL: "git@github.com:o/r.git"})
	saveTest(t, dir, Manifest{ID: "b", Branch: "feat/b", RemoteURL: "git@github.com:o/other.git"})
	calls := 0
	client := gh.New(gh.RunnerFunc(func(context.Context, string, ...string) ([]byte, error) { calls++; return nil, gh.ErrMissing }))
	if _, err := RefreshAllGitHub(context.Background(), client); !errors.Is(err, gh.ErrMissing) || calls != 1 {
		t.Fatal(err, calls)
	}
}

func TestOpenPRBestEffort(t *testing.T) {
	m := Manifest{Branch: "feat/a", RemoteURL: "git@github.com:o/r.git"}
	client, calls := scripted(t, map[string]string{"pr list": prJSON})
	if pr := OpenPR(context.Background(), client, m); pr == nil || pr.Number != 7 || !strings.Contains((*calls)[0], "--state open") {
		t.Fatal(pr, *calls)
	}
	failing := gh.New(gh.RunnerFunc(func(context.Context, string, ...string) ([]byte, error) { return nil, gh.ErrAuth }))
	if OpenPR(context.Background(), failing, m) != nil {
		t.Fatal("errors must be swallowed")
	}
	m.RemoteURL = "/tmp/x.git"
	if OpenPR(context.Background(), client, m) != nil {
		t.Fatal("non-GitHub")
	}
}

func TestMarkPRReadyNeedsKnownPR(t *testing.T) {
	dir := ghState(t)
	saveTest(t, dir, Manifest{ID: "a", Branch: "feat/a", RemoteURL: "git@github.com:o/r.git"})
	client, calls := scripted(t, map[string]string{"pr list": prJSON})
	if err := MarkPRReady(context.Background(), client, "a"); err == nil || !strings.Contains(err.Error(), "no known PR") || len(*calls) != 0 {
		t.Fatal(err, *calls)
	}
	saveTest(t, dir, Manifest{ID: "a", Branch: "feat/a", RemoteURL: "git@github.com:o/r.git", GH: &PRInfo{PR: 7, Draft: true}})
	if err := MarkPRReady(context.Background(), client, "a"); err != nil || (*calls)[0] != "pr ready 7 --repo o/r" || !strings.HasPrefix((*calls)[1], "pr list") {
		t.Fatal(err, *calls)
	}
}

func TestUnpushedWarning(t *testing.T) {
	root := t.TempDir()
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	remote, work := filepath.Join(root, "remote.git"), filepath.Join(root, "work")
	git := func(dir string, args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("%v %s", err, out)
		}
	}
	git(root, "init", "--bare", remote)
	git(root, "init", "-b", "feat/a", work)
	git(work, "config", "user.name", "Fixture")
	git(work, "config", "user.email", "fixture@example.invalid")
	git(work, "commit", "--allow-empty", "-m", "one")
	git(work, "remote", "add", "origin", remote)
	git(work, "push", "-u", "origin", "feat/a")
	if w := unpushedWarning(work); w != "" {
		t.Fatal(w)
	}
	git(work, "commit", "--allow-empty", "-m", "two")
	if w := unpushedWarning(work); w != "1 local commit is not pushed; the PR shows pushed commits only" {
		t.Fatal(w)
	}
	git(work, "commit", "--allow-empty", "-m", "three")
	if w := unpushedWarning(work); w != "2 local commits are not pushed; the PR shows pushed commits only" {
		t.Fatal(w)
	}
	if w := unpushedWarning(filepath.Join(root, "missing")); w != "" {
		t.Fatal(w)
	}
}
