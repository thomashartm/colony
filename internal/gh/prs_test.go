package gh

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRollup(t *testing.T) {
	run := func(status, conclusion string) Check {
		return Check{Typename: "CheckRun", Status: status, Conclusion: conclusion}
	}
	ctx := func(state string) Check { return Check{Typename: "StatusContext", State: state} }
	for _, tt := range []struct {
		checks []Check
		want   string
	}{
		{nil, "NONE"},
		{[]Check{run("COMPLETED", "SUCCESS"), run("COMPLETED", "SKIPPED"), run("COMPLETED", "NEUTRAL"), ctx("SUCCESS")}, "SUCCESS"},
		{[]Check{run("COMPLETED", "SUCCESS"), run("IN_PROGRESS", "")}, "PENDING"},
		{[]Check{run("QUEUED", "")}, "PENDING"},
		{[]Check{ctx("PENDING")}, "PENDING"},
		{[]Check{ctx("EXPECTED")}, "PENDING"},
		{[]Check{run("IN_PROGRESS", ""), run("COMPLETED", "TIMED_OUT")}, "FAILURE"},
		{[]Check{run("COMPLETED", "CANCELLED")}, "FAILURE"},
		{[]Check{run("COMPLETED", "STARTUP_FAILURE")}, "FAILURE"},
		{[]Check{ctx("ERROR")}, "FAILURE"},
		{[]Check{ctx("FAILURE"), ctx("PENDING")}, "FAILURE"},
		{[]Check{run("COMPLETED", "ACTION_REQUIRED")}, "FAILURE"},
	} {
		if got := Rollup(tt.checks); got != tt.want {
			t.Errorf("%+v: %s want %s", tt.checks, got, tt.want)
		}
	}
}

func TestPRListCalls(t *testing.T) {
	c, calls := fake(t, fixture(t, "prs.json"), nil)
	prs, err := c.PRsForRepo(context.Background(), "thomashartm", "motley")
	if err != nil || len(prs) != 3 {
		t.Fatalf("%+v %v", prs, err)
	}
	pr := prs[0]
	if pr.Number != 44 || pr.State != "MERGED" || pr.IsDraft || pr.HeadRefName != "feat/wordmark-horns" || !strings.HasPrefix(pr.URL, "https://github.com/thomashartm/motley/pull/44") || len(pr.StatusCheckRollup) != 18 || Rollup(pr.StatusCheckRollup) != "SUCCESS" {
		t.Fatalf("%+v", pr)
	}
	if got := strings.Join((*calls)[0].args, " "); got != "pr list --repo thomashartm/motley --state all --limit 200 --json number,url,state,isDraft,reviewDecision,statusCheckRollup,headRefName" {
		t.Fatal(got)
	}
	c, calls = fake(t, "[]", nil)
	if _, found, err := c.PRForBranch(context.Background(), "o", "r", "feat/x", "open"); found || err != nil {
		t.Fatal(found, err)
	}
	if got := strings.Join((*calls)[0].args, " "); got != "pr list --repo o/r --head feat/x --state open --limit 1 --json number,url,state,isDraft,reviewDecision,statusCheckRollup,headRefName" {
		t.Fatal(got)
	}
	c, _ = fake(t, "{not json", nil)
	if _, err := c.PRsForRepo(context.Background(), "o", "r"); err == nil || !strings.Contains(err.Error(), "read gh pr list") {
		t.Fatal(err)
	}
	c, _ = fake(t, "", ErrAuth)
	if _, _, err := c.PRForBranch(context.Background(), "o", "r", "b", "all"); !errors.Is(err, ErrAuth) {
		t.Fatal(err)
	}
}

func TestCreateAndReady(t *testing.T) {
	c, calls := fake(t, "Creating pull request…\nhttps://github.com/o/r/pull/7\n", nil)
	url, err := c.CreatePR(context.Background(), "/w", "main", "feat/x")
	if err != nil || url != "https://github.com/o/r/pull/7" || (*calls)[0].dir != "/w" || strings.Join((*calls)[0].args, " ") != "pr create --fill --base main --head feat/x" {
		t.Fatalf("%q %v %+v", url, err, *calls)
	}
	c, _ = fake(t, "Warning: 1 uncommitted change\n", nil)
	if _, err := c.CreatePR(context.Background(), "/w", "main", "feat/x"); err == nil || !strings.Contains(err.Error(), "no PR URL") {
		t.Fatal(err)
	}
	c, calls = fake(t, "", nil)
	if err := c.MarkReady(context.Background(), "o", "r", 7); err != nil || strings.Join((*calls)[0].args, " ") != "pr ready 7 --repo o/r" {
		t.Fatal(err, *calls)
	}
}
