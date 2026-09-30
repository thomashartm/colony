package minion

import (
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/pelletier/go-toml/v2"
	"github.com/thomashartm/colony/internal/agents"
	"github.com/thomashartm/colony/internal/blueprint"
	"github.com/thomashartm/colony/internal/config"
	"github.com/thomashartm/colony/internal/crew"
	"github.com/thomashartm/colony/internal/gitx"
	"github.com/thomashartm/colony/internal/state"
	"github.com/thomashartm/colony/internal/tmux"
	"github.com/thomashartm/colony/internal/worktree"
)

type SpawnOptions struct {
	Repo, Branch, Agent, Ticket, Name, Crew, Color, Blueprint string
	Vars                                                      []string
}

func Spawn(cfg config.Config, opts SpawnOptions, progress io.Writer) (Manifest, error) {
	for _, tool := range []string{"git", "tmux", "cp"} {
		if _, err := exec.LookPath(tool); err != nil {
			return Manifest{}, fmt.Errorf("%s is required: %w", tool, err)
		}
	}
	for _, value := range []string{opts.Name, opts.Ticket, opts.Repo} {
		if strings.IndexFunc(value, unicode.IsControl) >= 0 {
			return Manifest{}, fmt.Errorf("name, ticket and repo must not contain control characters")
		}
	}
	repo, err := ResolveRepo(cfg.ReposRoot, opts.Repo)
	if err != nil {
		return Manifest{}, err
	}
	vars, err := blueprint.Variables(opts.Vars)
	if err != nil {
		return Manifest{}, err
	}
	if opts.Blueprint == "" && len(opts.Vars) > 0 {
		return Manifest{}, fmt.Errorf("--var requires --blueprint")
	}
	var bp blueprint.Blueprint
	if opts.Blueprint != "" {
		available, err := blueprint.Discover(repo, opts.Repo)
		if err != nil {
			return Manifest{}, err
		}
		bp, err = blueprint.Find(available, opts.Blueprint)
		if err != nil {
			return Manifest{}, err
		}
		if opts.Agent != "" && opts.Agent != bp.Agent && len(bp.Args) > 0 {
			return Manifest{}, fmt.Errorf("blueprint %s args belong to %s; cannot use them with --agent %s", bp.Name, bp.Agent, opts.Agent)
		}
		if opts.Agent == "" {
			opts.Agent = bp.Agent
		}
	}
	if opts.Agent == "" {
		opts.Agent = "claude"
	}
	if _, err := agents.Binary(opts.Agent); err != nil {
		return Manifest{}, err
	}
	branchSlug := strings.ReplaceAll(opts.Branch, "/", "-")
	if err := CheckID(branchSlug); err != nil {
		return Manifest{}, fmt.Errorf("branch cannot form a minion path: %w", err)
	}
	root, err := filepath.Abs(cfg.WorktreesRoot)
	if err != nil {
		return Manifest{}, err
	}
	path := filepath.Join(root, opts.Repo, branchSlug)
	if err := worktree.CheckNew(repo, opts.Branch, path); err != nil {
		return Manifest{}, err
	}
	base, err := worktree.Base(repo)
	if err != nil {
		return Manifest{}, err
	}
	remote, err := gitx.Output(repo, "remote", "get-url", "origin")
	if err != nil {
		return Manifest{}, err
	}
	dir, err := state.MinionsDir()
	if err != nil {
		return Manifest{}, err
	}
	lock, err := state.LockSpawn(dir)
	if err != nil {
		return Manifest{}, err
	}
	defer func() { _ = lock.Close() }()
	crews, err := crew.Load()
	if err != nil {
		return Manifest{}, err
	}
	if err := validateIdentity(opts.Crew, opts.Color, crews); err != nil {
		return Manifest{}, err
	}
	manifests, err := loadAll(dir)
	if err != nil {
		return Manifest{}, err
	}
	sessions, err := tmux.Sessions()
	if err != nil {
		return Manifest{}, err
	}
	id, name, err := identity(opts, manifests, sessions)
	if err != nil {
		return Manifest{}, err
	}
	prompt := ""
	if opts.Blueprint != "" {
		c, _ := crew.Find(crews, opts.Crew)
		prompt, err = bp.Render(blueprint.Data{Repo: opts.Repo, Branch: opts.Branch, Base: base, Ticket: opts.Ticket, Name: name, Worktree: path, Crew: blueprint.Crew{Title: c.Title, URL: c.URL, Kind: c.Kind}, Vars: vars})
		if err != nil {
			return Manifest{}, err
		}
	}
	if err := worktree.Create(repo, opts.Branch, base, path, progress); err != nil {
		return Manifest{}, err
	}
	m := Manifest{
		Schema: 1, ID: id, Name: name, Repo: opts.Repo, RepoPath: repo,
		Worktree: path, Branch: opts.Branch, Base: base, RemoteURL: remote,
		Ticket: opts.Ticket, Agent: opts.Agent, Blueprint: opts.Blueprint, AgentArgs: bp.Args, Crew: opts.Crew, Color: opts.Color, CreatedAt: time.Now().UTC(),
	}
	if opts.Blueprint != "" {
		if err := state.WriteAtomic(filepath.Join(dir, id+".prompt.md"), []byte(blueprint.PromptHeader+prompt)); err != nil {
			return Manifest{}, fmt.Errorf("worktree retained at %s; prompt could not be saved: %w", path, err)
		}
	}
	data, err := toml.Marshal(m)
	if err == nil {
		err = state.WriteAtomic(filepath.Join(dir, id+".toml"), data)
	}
	if err != nil {
		return Manifest{}, fmt.Errorf("worktree retained at %s; manifest could not be saved: %w", path, err)
	}
	if err := tmux.Start(id, path, opts.Ticket, opts.Agent); err != nil {
		return Manifest{}, fmt.Errorf("worktree and manifest retained for %s; tmux startup failed: %w", id, err)
	}
	return m, applyAppearance(m, crews)
}

func ResolveRepo(root, name string) (string, error) {
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name {
		return "", fmt.Errorf("--repo must be a directory name directly under repos_root")
	}
	path, err := filepath.Abs(filepath.Join(root, name))
	if err != nil {
		return "", err
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve repository %s: %w", name, err)
	}
	top, err := gitx.Output(path, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	top, err = filepath.EvalSymlinks(top)
	if err != nil || top != path {
		return "", fmt.Errorf("%s must be a repository root", path)
	}
	gitDir, err := gitx.Output(path, "rev-parse", "--git-dir")
	if err != nil {
		return "", err
	}
	common, err := gitx.Output(path, "rev-parse", "--git-common-dir")
	if err != nil {
		return "", err
	}
	if gitDir != common {
		return "", fmt.Errorf("%s is a linked worktree; --repo must select the main repository", path)
	}
	return path, nil
}

func identity(opts SpawnOptions, manifests []Manifest, sessions []tmux.Session) (string, string, error) {
	name := opts.Name
	if name == "" {
		name = filepath.Base(opts.Branch)
	}
	id := strings.ReplaceAll(opts.Branch, "/", "-")
	if opts.Ticket != "" {
		suffix := slug(strings.TrimPrefix(name, opts.Ticket+"-"))
		if suffix == "" {
			return "", "", fmt.Errorf("name must contain letters or digits to form a minion id")
		}
		id = opts.Ticket + "-" + suffix
	}
	if err := CheckID(id); err != nil {
		return "", "", err
	}
	taken := func(id string) bool {
		for _, m := range manifests {
			if tmux.SessionName(m.ID) == tmux.SessionName(id) {
				return true
			}
		}
		for _, s := range sessions {
			if s.Name == tmux.SessionName(id) || s.MinionID == id {
				return true
			}
		}
		return false
	}
	if taken(id) {
		id = slug(opts.Repo) + "-" + id
	}
	if err := CheckID(id); err != nil {
		return "", "", err
	}
	if taken(id) {
		return "", "", fmt.Errorf("minion id %q is already taken; choose a different name or branch", id)
	}
	return id, name, nil
}

func slug(value string) string {
	var out strings.Builder
	separator := false
	for _, r := range strings.ToLower(value) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if separator && out.Len() > 0 {
				out.WriteByte('-')
			}
			out.WriteRune(r)
			separator = false
		} else {
			separator = true
		}
	}
	return out.String()
}
