package gh

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type call struct {
	dir  string
	args []string
}

func fake(t *testing.T, out string, err error) (*Client, *[]call) {
	t.Helper()
	var calls []call
	return New(RunnerFunc(func(_ context.Context, dir string, args ...string) ([]byte, error) {
		calls = append(calls, call{dir, args})
		return []byte(out), err
	})), &calls
}

func fixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestIssueRecordedWithoutParent(t *testing.T) {
	c, calls := fake(t, fixture(t, "issue-12.json"), nil)
	issue, err := c.Issue(context.Background(), "thomashartm", "motley", 12)
	if err != nil || issue.Title != "W9 — GitHub on demand and links" || issue.URL != "https://github.com/thomashartm/motley/issues/12" || issue.Parent != nil || issue.Milestone != nil || !strings.Contains(issue.Body, "## Scope") {
		t.Fatalf("%+v %v", issue, err)
	}
	got := strings.Join((*calls)[0].args, " ")
	for _, want := range []string{"api graphql", "-f owner=thomashartm", "-f repo=motley", "-F number=12", "parent{title url}", "milestone{title url}"} {
		if !strings.Contains(got, want) {
			t.Fatalf("argv %q lacks %q", got, want)
		}
	}
}

func TestIssueWithParentAndMilestone(t *testing.T) {
	c, _ := fake(t, fixture(t, "issue-parent.json"), nil)
	issue, err := c.Issue(context.Background(), "thomashartm", "motley", 12)
	if err != nil || issue.Parent == nil || issue.Parent.URL != "https://github.com/thomashartm/motley/issues/400" || issue.Milestone == nil || issue.Milestone.Title != "Q4" {
		t.Fatalf("%+v %v", issue, err)
	}
}

func TestIssueNotFoundAndBadJSON(t *testing.T) {
	c, _ := fake(t, `{"data":{"repository":{"issue":null}}}`, nil)
	if _, err := c.Issue(context.Background(), "o", "r", 9); err == nil || !strings.Contains(err.Error(), "issue #9 not found in o/r") {
		t.Fatal(err)
	}
	c, _ = fake(t, `not json`, nil)
	if _, err := c.Issue(context.Background(), "o", "r", 9); err == nil || !strings.Contains(err.Error(), "read gh issue response") {
		t.Fatal(err)
	}
	c, _ = fake(t, "", ErrAuth)
	if _, err := c.Issue(context.Background(), "o", "r", 9); !errors.Is(err, ErrAuth) {
		t.Fatal(err)
	}
}

func TestProjectTitle(t *testing.T) {
	c, calls := fake(t, `{"number":7,"title":"Q4 platform hardening","url":"https://github.com/orgs/acme/projects/7"}`, nil)
	title, err := c.ProjectTitle(context.Background(), "acme", 7)
	if err != nil || title != "Q4 platform hardening" || strings.Join((*calls)[0].args, " ") != "project view 7 --owner acme --format json" {
		t.Fatalf("%q %v %v", title, err, *calls)
	}
	c, _ = fake(t, `{"number":7}`, nil)
	if _, err := c.ProjectTitle(context.Background(), "acme", 7); err == nil {
		t.Fatal("a project without a title must fail")
	}
}

// writeGH installs a fake gh as the only thing on PATH; scripts use absolute paths.
func writeGH(t *testing.T, script string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
}

func TestExecClassifiesFailures(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, err := (Exec{}).Output(context.Background(), "", "version"); !errors.Is(err, ErrMissing) {
		t.Fatalf("missing: %v", err)
	}
	writeGH(t, "echo 'To get started with GitHub CLI, please run:  gh auth login' >&2\nexit 4\n")
	if _, err := (Exec{}).Output(context.Background(), "", "issue", "view"); !errors.Is(err, ErrAuth) {
		t.Fatalf("auth: %v", err)
	}
	writeGH(t, "echo 'error: your authentication token is missing required scopes [read:project]' >&2\necho 'To request it, run:  gh auth refresh -s read:project' >&2\nexit 1\n")
	if _, err := (Exec{}).Output(context.Background(), "", "project", "view"); err == nil || Hint(err) != "gh needs the read:project scope; run gh auth refresh -s read:project" {
		t.Fatalf("scope: %v", err)
	}
	writeGH(t, "echo 'GraphQL: Could not resolve to an Issue with the number of 9.' >&2\necho second >&2\nexit 1\n")
	if _, err := (Exec{}).Output(context.Background(), "", "api"); err == nil || Hint(err) != "gh: GraphQL: Could not resolve to an Issue with the number of 9." {
		t.Fatalf("other: %v", err)
	}
	writeGH(t, "exit 1\n")
	if _, err := (Exec{}).Output(context.Background(), "", "api"); err == nil || !strings.HasPrefix(Hint(err), "gh: exit status 1") {
		t.Fatalf("silent failure: %v", err)
	}
}

func TestExecTimesOut(t *testing.T) {
	writeGH(t, "exec /bin/sleep 5\n")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := (Exec{}).Output(ctx, "", "slow")
	if err == nil || !strings.Contains(Hint(err), "timed out") || time.Since(start) > 3*time.Second {
		t.Fatalf("timeout: %v after %s", err, time.Since(start))
	}
}

func TestExecRunsInDirWithoutPrompts(t *testing.T) {
	writeGH(t, "printf '%s|%s|%s' \"$PWD\" \"$GH_PROMPT_DISABLED\" \"$*\"\n")
	dir := t.TempDir()
	out, err := (Exec{}).Output(context.Background(), dir, "pr", "create")
	real, _ := filepath.EvalSymlinks(dir)
	if err != nil || (string(out) != dir+"|1|pr create" && string(out) != real+"|1|pr create") {
		t.Fatalf("%q %v", out, err)
	}
}

func TestHint(t *testing.T) {
	if Hint(nil) != "" || Hint(ErrMissing) != "GitHub CLI (gh) not found; install gh for issue and PR data" {
		t.Fatal(Hint(ErrMissing))
	}
}
