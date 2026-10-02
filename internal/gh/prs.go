package gh

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Check is one entry of a PR's statusCheckRollup: a CheckRun (Status and
// Conclusion) or a commit StatusContext (State).
type Check struct {
	Typename                  string `json:"__typename"`
	Status, Conclusion, State string
}

// PR is the subset of gh pr list fields motley records.
type PR struct {
	Number                      int
	URL, State                  string
	IsDraft                     bool
	ReviewDecision, HeadRefName string
	StatusCheckRollup           []Check
}

const prFields = "number,url,state,isDraft,reviewDecision,statusCheckRollup,headRefName"

// Rollup reduces statusCheckRollup to SUCCESS, FAILURE, PENDING or NONE. Any
// failure wins over pending checks; skipped and neutral runs count as passed.
func Rollup(checks []Check) string {
	if len(checks) == 0 {
		return "NONE"
	}
	pending := false
	for _, c := range checks {
		if c.Typename == "StatusContext" {
			switch c.State {
			case "FAILURE", "ERROR":
				return "FAILURE"
			case "PENDING", "EXPECTED":
				pending = true
			}
			continue
		}
		switch c.Conclusion {
		case "FAILURE", "CANCELLED", "TIMED_OUT", "ACTION_REQUIRED", "STARTUP_FAILURE":
			return "FAILURE"
		}
		if c.Status != "COMPLETED" {
			pending = true
		}
	}
	if pending {
		return "PENDING"
	}
	return "SUCCESS"
}

func (c *Client) prList(ctx context.Context, args ...string) ([]PR, error) {
	out, err := c.run.Output(ctx, "", append(append([]string{"pr", "list"}, args...), "--json", prFields)...)
	if err != nil {
		return nil, err
	}
	var prs []PR
	if err := json.Unmarshal(out, &prs); err != nil {
		return nil, fmt.Errorf("read gh pr list: %w", err)
	}
	return prs, nil
}

// PRForBranch returns the newest PR whose head is branch in the given state
// (open, closed, merged or all).
func (c *Client) PRForBranch(ctx context.Context, owner, repo, branch, state string) (PR, bool, error) {
	prs, err := c.prList(ctx, "--repo", owner+"/"+repo, "--head", branch, "--state", state, "--limit", "1")
	if err != nil || len(prs) == 0 {
		return PR{}, false, err
	}
	return prs[0], true, nil
}

// PRsForRepo lists the newest 200 PRs, newest first; callers match head
// branches themselves so one call serves every member of a repository.
func (c *Client) PRsForRepo(ctx context.Context, owner, repo string) ([]PR, error) {
	return c.prList(ctx, "--repo", owner+"/"+repo, "--state", "all", "--limit", "200")
}

// CreatePR runs in dir (the worktree) so --fill reads its commits, and returns
// the URL gh prints last.
func (c *Client) CreatePR(ctx context.Context, dir, base, branch string) (string, error) {
	out, err := c.run.Output(ctx, dir, "pr", "create", "--fill", "--base", base, "--head", branch)
	if err != nil {
		return "", err
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 || !strings.HasPrefix(fields[len(fields)-1], "https://") {
		return "", errors.New("gh pr create printed no PR URL")
	}
	return fields[len(fields)-1], nil
}

// MarkReady marks a draft PR ready for review.
func (c *Client) MarkReady(ctx context.Context, owner, repo string, number int) error {
	_, err := c.run.Output(ctx, "", "pr", "ready", strconv.Itoa(number), "--repo", owner+"/"+repo)
	return err
}
