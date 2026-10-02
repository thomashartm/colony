package member

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/thomashartm/motley/internal/config"
	"github.com/thomashartm/motley/internal/crew"
	"github.com/thomashartm/motley/internal/gh"
	"github.com/thomashartm/motley/internal/gitx"
)

// LookupTimeout bounds every on-demand GitHub read (§11).
const LookupTimeout = 10 * time.Second

// issueBodyLimit keeps rendered prompts well under their 64 KiB argv cap.
const issueBodyLimit = 32 << 10

// IssueContext is the issue data spawn renders into prompts and manifests.
type IssueContext struct {
	Number           int
	Title, Body, URL string
	Suggestion       *CrewSuggestion
}

// CrewSuggestion comes from the parent issue, else the milestone. CrewID names
// an existing crew with the same URL at lookup time; empty when none matches.
type CrewSuggestion struct{ Title, URL, Source, CrewID string }

var issueTicket = regexp.MustCompile(`^#?([1-9][0-9]{0,9})$`)

// IssueNumber reports the GitHub issue number a ticket such as 412 or #412 names.
func IssueNumber(ticket string) (int, bool) {
	match := issueTicket.FindStringSubmatch(strings.TrimSpace(ticket))
	if match == nil {
		return 0, false
	}
	n, err := strconv.Atoi(match[1])
	return n, err == nil && n > 0 && n <= 1<<31-1
}

// LookupIssue fetches the issue a numeric ticket names in repoName's origin. It
// returns nil without error when the ticket is not an issue number or origin is
// not on GitHub.
func LookupIssue(ctx context.Context, client *gh.Client, cfg config.Config, repoName, ticket string) (*IssueContext, error) {
	if _, ok := IssueNumber(ticket); !ok {
		return nil, nil
	}
	repo, err := ResolveRepo(cfg.ReposRoot, repoName)
	if err != nil {
		return nil, err
	}
	remote, err := gitx.Output(repo, "remote", "get-url", "origin")
	if err != nil {
		return nil, err
	}
	crews, err := crew.Load()
	if err != nil {
		return nil, err
	}
	return lookupIssue(ctx, client, remote, ticket, crews)
}

func lookupIssue(ctx context.Context, client *gh.Client, remote, ticket string, crews []crew.Crew) (*IssueContext, error) {
	n, ok := IssueNumber(ticket)
	r, github := gitx.WebURL(remote)
	if !ok || !github {
		return nil, nil
	}
	issue, err := client.Issue(ctx, r.Owner, r.Name, n)
	if err != nil {
		return nil, err
	}
	return &IssueContext{Number: n, Title: issue.Title, Body: capBody(issue.Body), URL: issue.URL, Suggestion: suggest(issue, crews)}, nil
}

func suggest(issue gh.Issue, crews []crew.Crew) *CrewSuggestion {
	ref, source := issue.Parent, "parent issue"
	if ref == nil || ref.URL == "" {
		ref, source = issue.Milestone, "milestone"
	}
	if ref == nil || ref.URL == "" || strings.TrimSpace(ref.Title) == "" {
		return nil
	}
	s := &CrewSuggestion{Title: strings.TrimSpace(ref.Title), URL: ref.URL, Source: source}
	if c, ok := crew.FindURL(crews, ref.URL); ok {
		s.CrewID = c.ID
	}
	return s
}

// capBody cuts long issue bodies on a rune boundary and says so.
func capBody(body string) string {
	if len(body) <= issueBodyLimit {
		return body
	}
	cut := issueBodyLimit
	for cut > 0 && !utf8.RuneStart(body[cut]) {
		cut--
	}
	return body[:cut] + "\n\n(issue body truncated)"
}

// CrewTitle fetches the title of a GitHub issue or project crew URL once.
func CrewTitle(ctx context.Context, client *gh.Client, rawURL string) (string, error) {
	switch kind, owner, repo, n := crew.GitHubRef(rawURL); kind {
	case "issue":
		issue, err := client.Issue(ctx, owner, repo, n)
		return issue.Title, err
	case "project":
		return client.ProjectTitle(ctx, owner, n)
	}
	return "", errors.New("--title is required for links other than GitHub issues and projects")
}

// AddCrewWithLookup creates a crew, fetching a missing title from GitHub (§6.5).
func AddCrewWithLookup(ctx context.Context, client *gh.Client, title, url, color, gig string) (crew.Crew, error) {
	if strings.TrimSpace(title) == "" {
		if strings.TrimSpace(url) == "" {
			return crew.Crew{}, errors.New("crew title is required")
		}
		fetched, err := CrewTitle(ctx, client, url)
		if err != nil {
			return crew.Crew{}, err
		}
		title = fetched
	}
	return AddCrew(title, url, color, gig)
}
