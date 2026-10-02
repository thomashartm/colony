package member

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/thomashartm/motley/internal/gh"
	"github.com/thomashartm/motley/internal/gitx"
	"github.com/thomashartm/motley/internal/state"
)

const (
	// RetireLookupTimeout bounds the open-PR warning so retire never stalls.
	RetireLookupTimeout = 5 * time.Second
	// CreateTimeout bounds gh pr create, which pushes nothing but talks to GitHub.
	CreateTimeout = 30 * time.Second
)

func prInfo(pr gh.PR, found bool, now time.Time) *PRInfo {
	if !found {
		return &PRInfo{FetchedAt: now}
	}
	return &PRInfo{PR: pr.Number, URL: pr.URL, State: pr.State, Draft: pr.IsDraft, Review: pr.ReviewDecision, Checks: gh.Rollup(pr.StatusCheckRollup), FetchedAt: now}
}

func githubRemote(m Manifest) (gitx.Remote, error) {
	r, ok := gitx.WebURL(m.RemoteURL)
	if !ok || m.Branch == "" {
		return r, fmt.Errorf("%s: origin is not on GitHub or there is no branch; GitHub data is unavailable", m.ID)
	}
	return r, nil
}

// updateManifest reloads id under the lifecycle lock and applies change, so
// edits saved while gh was running survive; change touches GitHub fields only.
func updateManifest(id string, change func(*Manifest)) (Manifest, error) {
	dir, err := state.MembersDir()
	if err != nil {
		return Manifest{}, err
	}
	lock, err := state.LockSpawn(dir)
	if err != nil {
		return Manifest{}, err
	}
	defer func() { _ = lock.Close() }()
	m, err := Load(dir, id)
	if err != nil {
		return Manifest{}, err
	}
	change(&m)
	return m, saveManifest(dir, m)
}

func loadMember(id string) (Manifest, error) {
	dir, err := state.MembersDir()
	if err != nil {
		return Manifest{}, err
	}
	return Load(dir, id)
}

// RefreshGitHub records the member's newest PR and, for a numeric ticket, the
// issue's current title. A failed gh call leaves the manifest unchanged.
func RefreshGitHub(ctx context.Context, client *gh.Client, id string) (Manifest, error) {
	m, err := loadMember(id)
	if err != nil {
		return Manifest{}, err
	}
	r, err := githubRemote(m)
	if err != nil {
		return m, err
	}
	pr, found, err := client.PRForBranch(ctx, r.Owner, r.Name, m.Branch, "all")
	if err != nil {
		return m, err
	}
	var issue *IssueRef
	if n, ok := IssueNumber(m.Ticket); ok {
		got, err := client.Issue(ctx, r.Owner, r.Name, n)
		if err != nil {
			return m, err
		}
		issue = &IssueRef{Title: got.Title, URL: got.URL}
	}
	info := prInfo(pr, found, time.Now().UTC())
	return updateManifest(id, func(m *Manifest) {
		m.GH = info
		if issue != nil {
			m.Issue = issue
		}
	})
}

// RefreshSummary reports a refresh of every GitHub member.
type RefreshSummary struct {
	Members, Repos int
	Failures       []string
}

// RefreshAllGitHub lists PRs once per GitHub repository and records each
// member's newest PR. A missing or unauthenticated gh stops the run; other
// failures are collected per repository.
func RefreshAllGitHub(ctx context.Context, client *gh.Client) (RefreshSummary, error) {
	var sum RefreshSummary
	dir, err := state.MembersDir()
	if err != nil {
		return sum, err
	}
	manifests, err := loadAll(dir)
	if err != nil {
		return sum, err
	}
	byRepo := map[string][]Manifest{}
	remotes := map[string]gitx.Remote{}
	var order []string
	for _, m := range manifests {
		r, err := githubRemote(m)
		if err != nil {
			continue
		}
		key := r.Owner + "/" + r.Name
		if _, seen := remotes[key]; !seen {
			order = append(order, key)
			remotes[key] = r
		}
		byRepo[key] = append(byRepo[key], m)
	}
	now := time.Now().UTC()
	for _, key := range order {
		prs, err := client.PRsForRepo(ctx, remotes[key].Owner, remotes[key].Name)
		if errors.Is(err, gh.ErrMissing) || errors.Is(err, gh.ErrAuth) {
			return sum, err
		}
		if err != nil {
			sum.Failures = append(sum.Failures, key+": "+gh.Hint(err))
			continue
		}
		newest := map[string]gh.PR{}
		for _, pr := range prs { // gh lists newest first
			if _, ok := newest[pr.HeadRefName]; !ok {
				newest[pr.HeadRefName] = pr
			}
		}
		for _, m := range byRepo[key] {
			pr, found := newest[m.Branch]
			if _, err := updateManifest(m.ID, func(m *Manifest) { m.GH = prInfo(pr, found, now) }); err != nil {
				sum.Failures = append(sum.Failures, m.ID+": "+err.Error())
				continue
			}
			sum.Members++
		}
		sum.Repos++
	}
	return sum, nil
}

// unpushedWarning says how many commits in worktree its upstream lacks; it is
// empty when everything is pushed or git cannot tell.
func unpushedWarning(worktree string) string {
	out, err := gitx.Output(worktree, "rev-list", "--count", "@{upstream}..HEAD")
	if err != nil {
		return ""
	}
	switch n, _ := strconv.Atoi(strings.TrimSpace(out)); {
	case n == 1:
		return "1 local commit is not pushed; the PR shows pushed commits only"
	case n > 1:
		return fmt.Sprintf("%d local commits are not pushed; the PR shows pushed commits only", n)
	}
	return ""
}

// CreatePR opens a PR from the member's branch against its base with --fill,
// then refreshes the member. The warning reports unpushed commits.
func CreatePR(ctx context.Context, client *gh.Client, id string) (url, warning string, err error) {
	m, err := loadMember(id)
	if err != nil {
		return "", "", err
	}
	if _, err := githubRemote(m); err != nil {
		return "", "", err
	}
	warning = unpushedWarning(m.Worktree)
	url, err = client.CreatePR(ctx, m.Worktree, m.Base, m.Branch)
	if err != nil {
		return "", warning, err
	}
	_, err = RefreshGitHub(ctx, client, id)
	return url, warning, err
}

// MarkPRReady marks the member's recorded draft PR ready, then refreshes it.
func MarkPRReady(ctx context.Context, client *gh.Client, id string) error {
	m, err := loadMember(id)
	if err != nil {
		return err
	}
	r, err := githubRemote(m)
	if err != nil {
		return err
	}
	if m.GH == nil || m.GH.PR == 0 {
		return errors.New("no known PR; refresh with u first")
	}
	if err := client.MarkReady(ctx, r.Owner, r.Name, m.GH.PR); err != nil {
		return err
	}
	_, err = RefreshGitHub(ctx, client, id)
	return err
}

// OpenPR returns the member's open PR for the retire warning. It is best
// effort: any failure, an imported session or a non-GitHub origin returns nil.
func OpenPR(ctx context.Context, client *gh.Client, m Manifest) *gh.PR {
	r, err := githubRemote(m)
	if err != nil || m.ClaudeSession != "" {
		return nil
	}
	pr, found, err := client.PRForBranch(ctx, r.Owner, r.Name, m.Branch, "open")
	if err != nil || !found || !strings.EqualFold(pr.State, "OPEN") {
		return nil
	}
	return &pr
}
