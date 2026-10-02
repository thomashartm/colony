package gh

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
)

// Ref is a titled GitHub link, such as a parent issue or milestone.
type Ref struct{ Title, URL string }

// Issue is the issue data spawn and crew lookups use.
type Issue struct {
	Title, Body, URL  string
	Parent, Milestone *Ref
}

// One GraphQL call returns the issue and the crew-suggestion candidates (§11, §13).
const issueQuery = `query($owner:String!,$repo:String!,$number:Int!){repository(owner:$owner,name:$repo){issue(number:$number){title body url parent{title url} milestone{title url}}}}`

// Issue fetches issue number in owner/repo.
func (c *Client) Issue(ctx context.Context, owner, repo string, number int) (Issue, error) {
	out, err := c.run.Output(ctx, "", "api", "graphql", "-f", "query="+issueQuery, "-f", "owner="+owner, "-f", "repo="+repo, "-F", "number="+strconv.Itoa(number))
	if err != nil {
		return Issue{}, err
	}
	var resp struct {
		Data struct {
			Repository struct {
				Issue *Issue `json:"issue"`
			} `json:"repository"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return Issue{}, fmt.Errorf("read gh issue response: %w", err)
	}
	if resp.Data.Repository.Issue == nil {
		return Issue{}, fmt.Errorf("issue #%d not found in %s/%s", number, owner, repo)
	}
	return *resp.Data.Repository.Issue, nil
}

// ProjectTitle fetches the title of project number owned by owner.
func (c *Client) ProjectTitle(ctx context.Context, owner string, number int) (string, error) {
	out, err := c.run.Output(ctx, "", "project", "view", strconv.Itoa(number), "--owner", owner, "--format", "json")
	if err != nil {
		return "", err
	}
	var p struct{ Title string }
	if err := json.Unmarshal(out, &p); err != nil || p.Title == "" {
		return "", fmt.Errorf("read gh project %s/%d: no title", owner, number)
	}
	return p.Title, nil
}
