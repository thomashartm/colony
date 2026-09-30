package worktree

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/thomashartm/colony/internal/gitx"
)

func Protected(branch string) bool {
	return branch == "main" || branch == "master" || branch == "develop"
}

// Linked verifies ownership before any removal, even for --force. A missing
// directory/registration is allowed so retirement can resume after partial cleanup.
func Linked(repo, path, branch string) (bool, error) {
	rows, err := gitx.Worktrees(repo)
	if err != nil {
		return false, err
	}
	target, err := Physical(path)
	if err != nil {
		return false, err
	}
	main, err := Physical(rows[0].Path)
	if err != nil {
		return false, err
	}
	if target == main || target == string(filepath.Separator) {
		return false, fmt.Errorf("refusing to remove the main worktree: %s", path)
	}
	for _, r := range rows[1:] {
		p, err := Physical(r.Path)
		if err != nil {
			return false, err
		}
		if p != target {
			continue
		}
		if r.Bare || r.Branch != branch {
			return false, fmt.Errorf("worktree identity changed at %s (branch %q, expected %q)", path, r.Branch, branch)
		}
		return true, nil
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		return false, fmt.Errorf("%s is not a registered linked worktree; refusing removal", path)
	}
	return false, nil
}

func Physical(path string) (string, error) {
	p, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(p)
	if os.IsNotExist(err) {
		parent, parentErr := Physical(filepath.Dir(p))
		if parentErr != nil {
			return "", parentErr
		}
		return filepath.Join(parent, filepath.Base(p)), nil
	}
	return resolved, err
}

// CleanOne ports wt-clean's double-force removal, fallback, unlock, prune and
// verification. Local protected branches and all remote branches are retained.
func CleanOne(repo, path, branch string, keepBranch bool) error {
	path, err := Physical(path)
	if err != nil {
		return err
	}
	registered, err := Linked(repo, path, branch)
	if err != nil {
		return err
	}
	var failures []error
	if registered {
		_, _ = gitx.Output(repo, "worktree", "remove", "--force", "--force", path)
		if err := os.RemoveAll(path); err != nil {
			failures = append(failures, fmt.Errorf("remove worktree directory: %w", err))
		}
		_, _ = gitx.Output(repo, "worktree", "unlock", path)
		_, _ = gitx.Output(repo, "worktree", "prune")
		rows, err := gitx.Worktrees(repo)
		if err != nil {
			return err
		}
		for _, r := range rows {
			if filepath.Clean(r.Path) == filepath.Clean(path) {
				return fmt.Errorf("git still lists worktree %s; branch retained", path)
			}
		}
	}
	if branch != "" && !keepBranch && !Protected(branch) {
		// A previous attempt may already have removed the branch.
		if _, err := gitx.Output(repo, "show-ref", "--verify", "--quiet", "refs/heads/"+branch); err == nil {
			if _, err := gitx.Output(repo, "branch", "-D", "--", branch); err != nil {
				failures = append(failures, err)
			}
		}
	}
	return errors.Join(failures...)
}
