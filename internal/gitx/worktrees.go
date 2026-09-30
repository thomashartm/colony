package gitx

import (
	"fmt"
	"strings"
)

type Worktree struct {
	Path, HEAD, Branch string
	Bare               bool
}

// Worktrees uses NUL delimiters so spaces, quotes and newlines in paths remain
// literal rather than git's quoted porcelain representation.
func Worktrees(repo string) ([]Worktree, error) {
	out, err := Output(repo, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, err
	}
	return ParseWorktrees(out)
}
func ParseWorktrees(out string) ([]Worktree, error) {
	var rows []Worktree
	for _, field := range strings.Split(out, "\x00") {
		key, value, _ := strings.Cut(field, " ")
		if key == "worktree" {
			rows = append(rows, Worktree{Path: value})
			continue
		}
		if key == "" {
			continue
		}
		if len(rows) == 0 {
			return nil, fmt.Errorf("worktree record has no path")
		}
		r := &rows[len(rows)-1]
		switch key {
		case "HEAD":
			r.HEAD = value
		case "branch":
			r.Branch = strings.TrimPrefix(value, "refs/heads/")
		case "bare":
			r.Bare = true
		}
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("git returned no worktrees")
	}
	return rows, nil
}
