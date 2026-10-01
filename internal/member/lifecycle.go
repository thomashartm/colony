package member

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/pelletier/go-toml/v2"
	"github.com/thomashartm/motley/internal/agents"
	"github.com/thomashartm/motley/internal/crew"
	"github.com/thomashartm/motley/internal/gitx"
	"github.com/thomashartm/motley/internal/state"
	"github.com/thomashartm/motley/internal/tmux"
	"github.com/thomashartm/motley/internal/worktree"
)

type RetireCheck struct {
	Manifest   Manifest
	Dirty      bool
	Ahead      int
	ComparedTo string
}

func (c RetireCheck) Risks() []string {
	var risks []string
	if c.Dirty {
		risks = append(risks, "uncommitted or untracked files")
	}
	if c.Ahead > 0 {
		risks = append(risks, fmt.Sprintf("%d commits not on %s", c.Ahead, c.ComparedTo))
	}
	return risks
}

func InspectRetire(id string) (RetireCheck, error) {
	dir, err := state.MembersDir()
	if err != nil {
		return RetireCheck{}, err
	}
	m, err := Load(dir, id)
	if err != nil {
		return RetireCheck{}, err
	}
	return inspectRetire(m, true)
}
func inspectRetire(m Manifest, checkChanges bool) (RetireCheck, error) {
	c := RetireCheck{Manifest: m}
	if !filepath.IsAbs(m.Worktree) || !filepath.IsAbs(m.RepoPath) {
		return c, fmt.Errorf("manifest worktree and repo paths must be absolute")
	}
	if _, err := worktree.Linked(m.RepoPath, m.Worktree, m.Branch); err != nil {
		return c, err
	}
	// Killing the caller's pane would interrupt cleanup halfway through.
	if os.Getenv("TMUX") != "" && os.Getenv("TMUX_PANE") != "" {
		session, err := tmux.CurrentSession()
		if err != nil {
			return c, err
		}
		if session == tmux.SessionName(m.ID) {
			return c, fmt.Errorf("open motley monitor to retire %s; this overview is inside the target session (or use another tmux session)", m.ID)
		}
	}
	if !checkChanges {
		return c, nil
	}
	exists := false
	if _, err := os.Stat(m.Worktree); err == nil {
		exists = true
	} else if !os.IsNotExist(err) {
		return c, err
	}
	if exists {
		status, err := gitx.Output(m.Worktree, "status", "--porcelain")
		if err != nil {
			return c, err
		}
		c.Dirty = status != ""
	}
	head := "refs/heads/" + m.Branch
	if m.Branch == "" {
		if !exists {
			return c, nil
		}
		var err error
		head, err = gitx.Output(m.Worktree, "rev-parse", "HEAD")
		if err != nil {
			return c, err
		}
	} else if _, err := gitx.Output(m.RepoPath, "show-ref", "--verify", "--quiet", head); err != nil {
		if !exists {
			return c, nil
		}
		return c, fmt.Errorf("member branch %q is missing", m.Branch)
	}
	upstream := ""
	if m.Branch != "" {
		upstream, _ = gitx.Output(m.RepoPath, "rev-parse", "--verify", m.Branch+"@{upstream}")
	}
	c.ComparedTo = "upstream"
	if upstream == "" {
		upstream = "refs/heads/" + m.Base
		c.ComparedTo = "base " + m.Base + " (no upstream)"
	}
	count, err := gitx.Output(m.RepoPath, "rev-list", "--count", upstream+".."+head, "--")
	if err != nil {
		return c, fmt.Errorf("cannot check unpushed commits: %w", err)
	}
	c.Ahead, err = strconv.Atoi(count)
	return c, err
}

func Retire(id string, force, keepBranch bool) error {
	dir, err := state.MembersDir()
	if err != nil {
		return err
	}
	lock, err := state.LockSpawn(dir)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Close() }()
	m, err := Load(dir, id)
	if err != nil {
		return err
	}
	check, err := inspectRetire(m, !force)
	if err != nil {
		return err
	}
	if risks := check.Risks(); len(risks) > 0 && !force {
		return fmt.Errorf("refusing to retire %s: %s; use --force to discard this work", id, strings.Join(risks, "; "))
	}
	sessions, err := tmux.Sessions()
	if err != nil {
		return err
	}
	live := false
	for _, s := range sessions {
		if s.Name == tmux.SessionName(id) {
			if s.MemberID != id {
				return fmt.Errorf("session %s is not owned by this member", s.Name)
			}
			live = true
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "archive"), 0700); err != nil {
		return err
	}
	if live {
		if err := tmux.Kill(id); err != nil {
			return err
		}
	}
	if live && !force {
		latest, err := inspectRetire(m, true)
		if err != nil {
			return err
		}
		if risks := latest.Risks(); len(risks) > 0 {
			return fmt.Errorf("work changed while stopping the session; worktree and manifest retained: %s", strings.Join(risks, "; "))
		}
	}
	if err := worktree.CleanOne(m.RepoPath, m.Worktree, m.Branch, keepBranch); err != nil {
		return fmt.Errorf("cleanup incomplete; manifest retained for retry: %w", err)
	}
	return archive(dir, m)
}

func archive(dir string, m Manifest) error {
	now := time.Now().UTC()
	m.RetiredAt = &now
	prefix := filepath.Join(dir, "archive", m.ID)
	// Preserve earlier retirements when an id has been reused.
	for _, suffix := range []string{".toml", ".events.jsonl", ".prompt.md"} {
		if _, err := os.Lstat(prefix + suffix); err == nil {
			prefix += "-" + now.Format("20060102T150405.000000000Z")
			break
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	// Copy atomically before removing any active files. A write failure leaves
	// the active manifest and its data available for retry or manual recovery.
	for _, suffix := range []string{".events.jsonl", ".prompt.md"} {
		data, err := os.ReadFile(filepath.Join(dir, m.ID+suffix))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if err := state.WriteAtomic(prefix+suffix, data); err != nil {
			return err
		}
	}
	data, err := toml.Marshal(m)
	if err != nil {
		return err
	}
	if err := state.WriteAtomic(prefix+".toml", data); err != nil {
		return err
	}
	for _, suffix := range []string{".events.jsonl", ".prompt.md"} {
		// Move the original inode too: a hook already holding the event file open
		// can finish its append in the archive instead of an unlinked active log.
		if err := os.Rename(filepath.Join(dir, m.ID+suffix), prefix+suffix); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("archive saved at %s.toml; move active file: %w", prefix, err)
		}
	}
	if err := os.Remove(filepath.Join(dir, m.ID+".toml")); err != nil {
		return fmt.Errorf("archive saved at %s.toml; remove active manifest: %w", prefix, err)
	}
	return nil
}

// Terminate stops only the owned tmux session. Files, branch and history remain
// available for inspection or revival, including uncommitted work.
func Terminate(id string) error {
	dir, err := state.MembersDir()
	if err != nil {
		return err
	}
	lock, err := state.LockSpawn(dir)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Close() }()
	if err := RequireLive(id); err != nil {
		return err
	}
	if os.Getenv("TMUX") != "" && os.Getenv("TMUX_PANE") != "" {
		session, err := tmux.CurrentSession()
		if err != nil {
			return err
		}
		if session == tmux.SessionName(id) {
			return fmt.Errorf("open motley monitor to terminate %s; this overview is inside the target session", id)
		}
	}
	return tmux.Kill(id)
}

func Revive(id string) error {
	dir, err := state.MembersDir()
	if err != nil {
		return err
	}
	lock, err := state.LockSpawn(dir)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Close() }()
	m, err := Load(dir, id)
	if err != nil {
		return err
	}
	sessions, err := tmux.Sessions()
	if err != nil {
		return err
	}
	for _, s := range sessions {
		if s.Name == tmux.SessionName(id) {
			return fmt.Errorf("member %s is already alive or its session name is occupied", id)
		}
	}
	registered, err := worktree.Linked(m.RepoPath, m.Worktree, m.Branch)
	if err != nil {
		return err
	}
	if !registered {
		return fmt.Errorf("worktree for %s is missing; revive does not recreate worktrees", id)
	}
	if info, err := os.Stat(m.Worktree); err != nil || !info.IsDir() {
		return fmt.Errorf("worktree for %s is unavailable", id)
	}
	if _, err := agents.Binary(m.Agent); err != nil {
		return err
	}
	crews, err := crew.Load()
	if err != nil {
		return err
	}
	if err := tmux.Revive(m.ID, m.Worktree, m.Ticket, m.Agent); err != nil {
		return err
	}
	return applyAppearance(m, crews)
}

type AdoptOptions struct{ Ticket, Name, Agent, Crew, Color string }

func Adopt(opts AdoptOptions) (Manifest, error) {
	for _, v := range []string{opts.Name, opts.Ticket} {
		if strings.IndexFunc(v, unicode.IsControl) >= 0 {
			return Manifest{}, fmt.Errorf("name and ticket must not contain control characters")
		}
	}
	if _, err := agents.Binary(opts.Agent); err != nil {
		return Manifest{}, err
	}
	session, err := tmux.CurrentSession()
	if err != nil {
		return Manifest{}, err
	}
	if session == tmux.MonitorSession {
		return Manifest{}, fmt.Errorf("cannot adopt the monitor session")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return Manifest{}, err
	}
	path, err := gitx.Output(cwd, "rev-parse", "--show-toplevel")
	if err != nil {
		return Manifest{}, err
	}
	path, err = worktree.Physical(path)
	if err != nil {
		return Manifest{}, err
	}
	rows, err := gitx.Worktrees(path)
	if err != nil {
		return Manifest{}, err
	}
	repo, err := worktree.Physical(rows[0].Path)
	if err != nil {
		return Manifest{}, err
	}
	branch := ""
	found := false
	for _, r := range rows {
		p, e := worktree.Physical(r.Path)
		if e != nil {
			return Manifest{}, e
		}
		if p == path {
			branch = r.Branch
			found = true
		}
	}
	if !found || repo == path {
		return Manifest{}, fmt.Errorf("adopt requires a linked worktree; the main checkout is never managed")
	}
	base, err := worktree.Base(repo)
	if err != nil {
		return Manifest{}, err
	}
	dir, err := state.MembersDir()
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
	for _, m := range manifests {
		p, e := worktree.Physical(m.Worktree)
		if e != nil {
			return Manifest{}, e
		}
		if p == path {
			return Manifest{}, fmt.Errorf("worktree already belongs to member %s", m.ID)
		}
	}
	sessions, err := tmux.Sessions()
	if err != nil {
		return Manifest{}, err
	}
	var others []tmux.Session
	current := false
	for _, s := range sessions {
		if s.Name == session {
			current = true
			if s.MemberID != "" || s.Monitor {
				return Manifest{}, fmt.Errorf("session is already managed by motley")
			}
		} else {
			others = append(others, s)
		}
	}
	if !current {
		return Manifest{}, fmt.Errorf("current tmux session no longer exists")
	}
	identityBranch := branch
	if branch == "" {
		head, e := gitx.Output(path, "rev-parse", "--short=8", "HEAD")
		if e != nil {
			return Manifest{}, e
		}
		identityBranch = "detached-" + head
	}
	repoName := filepath.Base(repo)
	id, name, err := identity(SpawnOptions{Repo: repoName, Branch: identityBranch, Ticket: opts.Ticket, Name: opts.Name}, manifests, others)
	if err != nil {
		return Manifest{}, err
	}
	remote, _ := gitx.Output(repo, "remote", "get-url", "origin")
	m := Manifest{Schema: 1, ID: id, Name: name, Repo: repoName, RepoPath: repo, Worktree: path, Branch: branch, Base: base, RemoteURL: remote, Ticket: opts.Ticket, Agent: opts.Agent, Crew: opts.Crew, Color: opts.Color, CreatedAt: time.Now().UTC()}
	data, err := toml.Marshal(m)
	if err != nil {
		return Manifest{}, err
	}
	if err := state.WriteAtomic(filepath.Join(dir, id+".toml"), data); err != nil {
		return Manifest{}, err
	}
	if err := tmux.Adopt(session, id, m.Ticket, m.Agent); err != nil {
		return m, fmt.Errorf("manifest retained for %s; tmux adoption incomplete: %w", id, err)
	}
	return m, applyAppearance(m, crews)
}
