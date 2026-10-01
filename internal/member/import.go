package member

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/thomashartm/motley/internal/agents/claude"
	"github.com/thomashartm/motley/internal/crew"
	"github.com/thomashartm/motley/internal/gitx"
	"github.com/thomashartm/motley/internal/state"
	"github.com/thomashartm/motley/internal/tmux"
	"github.com/thomashartm/motley/internal/worktree"
)

func DiscoverClaude() ([]claude.Session, error) {
	sessions, err := claude.Sessions()
	if err != nil {
		return nil, err
	}
	dir, err := state.MembersDir()
	if err != nil {
		return nil, err
	}
	members, err := loadAll(dir)
	if err != nil {
		return nil, err
	}
	var available []claude.Session
	for _, s := range sessions {
		if s.PID <= 0 {
			continue
		}
		managed := false
		for _, m := range members {
			if m.ClaudeSession == s.SessionID || (m.ClaudeSession == "" && sameDirectory(m.Worktree, s.Cwd)) {
				managed = true
				break
			}
		}
		if !managed {
			available = append(available, s)
		}
	}
	return available, nil
}

func ImportClaude(sessionID, name, crewID string) (Manifest, error) {
	if err := CheckID(sessionID); err != nil {
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
	sessions, err := DiscoverClaude()
	if err != nil {
		return Manifest{}, err
	}
	var selected *claude.Session
	for i := range sessions {
		if sessions[i].SessionID == sessionID {
			selected = &sessions[i]
			break
		}
	}
	if selected == nil {
		return Manifest{}, fmt.Errorf("the Claude session %s is no longer running or is already in Motley", sessionID)
	}
	s := *selected
	cwd, err := worktree.Physical(s.Cwd)
	if err != nil {
		return Manifest{}, err
	}
	if info, err := os.Stat(cwd); err != nil || !info.IsDir() {
		return Manifest{}, fmt.Errorf("session directory is unavailable: %s", cwd)
	}
	crews, err := crew.Load()
	if err != nil {
		return Manifest{}, err
	}
	if err := validateIdentity(crewID, "", crews); err != nil {
		return Manifest{}, err
	}
	if strings.TrimSpace(name) == "" {
		name = s.Name
	}
	if strings.TrimSpace(name) == "" {
		name = filepath.Base(cwd)
	}
	// Git metadata is optional. Existing Claude sessions may run outside a repo.
	repo := ""
	branch, base, remote := "", "", ""
	if rows, e := gitx.Worktrees(cwd); e == nil && len(rows) > 0 {
		repo = rows[0].Path
		branch, _ = gitx.Output(cwd, "symbolic-ref", "--short", "HEAD")
		base, _ = worktree.Base(repo)
		remote, _ = gitx.Output(repo, "remote", "get-url", "origin")
	}
	id := "claude-" + sessionID
	if _, err := os.Lstat(filepath.Join(dir, id+".toml")); !os.IsNotExist(err) {
		return Manifest{}, fmt.Errorf("member %s already exists", id)
	}
	m := Manifest{Schema: 1, ID: id, Name: name, Repo: filepath.Base(cwd), RepoPath: repo, Worktree: cwd, Branch: branch, Base: base, RemoteURL: remote, Agent: "claude", Crew: crewID, CreatedAt: time.Now().UTC(), ClaudeSession: sessionID}
	if err := saveManifest(dir, m); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

// RefreshExternal uses one bounded discovery call for all imported members.
// Never infer that external sessions died when discovery itself fails.
func RefreshExternal(rows []Row) ([]Row, error) {
	needed := false
	for _, r := range rows {
		if r.ClaudeSession != "" && !r.Alive {
			needed = true
			break
		}
	}
	if !needed {
		return rows, nil
	}
	sessions, err := claude.Sessions()
	if err != nil {
		return rows, err
	}
	for i := range rows {
		r := &rows[i]
		if r.ClaudeSession == "" || r.Alive {
			continue
		}
		r.External = true
		for _, s := range sessions {
			if s.SessionID == r.ClaudeSession && sameDirectory(s.Cwd, r.Worktree) {
				r.Alive = s.PID > 0
				r.Status = s.MotleyStatus()
				r.Seen = time.Now().Unix()
				break
			}
		}
	}
	return rows, nil
}

func externalSession(m Manifest) (*claude.Session, error) {
	sessions, err := claude.Sessions()
	if err != nil {
		return nil, err
	}
	for _, s := range sessions {
		if s.SessionID == m.ClaudeSession && s.PID > 0 {
			if !sameDirectory(s.Cwd, m.Worktree) {
				return nil, fmt.Errorf("the Claude session directory changed; import it again")
			}
			return &s, nil
		}
	}
	return nil, nil
}

func stopExternal(m Manifest) error {
	s, err := externalSession(m)
	if err != nil || s == nil {
		return err
	}
	if s.Kind == "background" {
		if s.ID == "" || CheckID(s.ID) != nil {
			return fmt.Errorf("the Claude background session has no valid control ID")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := exec.CommandContext(ctx, "claude", "stop", s.ID).Run(); err != nil {
			return fmt.Errorf("stop Claude session: %w", err)
		}
	} else {
		if s.PID <= 1 || s.PID == os.Getpid() {
			return fmt.Errorf("invalid Claude process identity")
		}
		// Revalidate identity immediately before signalling, including its start time.
		current, err := externalSession(m)
		if err != nil {
			return err
		}
		if current == nil {
			return nil
		}
		if current.PID != s.PID || current.StartedAt != s.StartedAt {
			return fmt.Errorf("the Claude process changed; retry the action")
		}
		process, err := os.FindProcess(s.PID)
		if err != nil {
			return err
		}
		if err := process.Signal(syscall.SIGTERM); err != nil {
			return err
		}
	}
	// Do not report success or allow a duplicate resume until the process is gone.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		current, err := externalSession(m)
		if err != nil {
			return err
		}
		if current == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("the Claude is still stopping; member retained, retry after it exits")
}

// OpenExternal focuses the existing Ghostty surface; it never starts a second
// Claude writer against an already-open conversation. Ambiguous paths refuse.
func OpenExternal(id string) error {
	dir, err := state.MembersDir()
	if err != nil {
		return err
	}
	m, err := Load(dir, id)
	if err != nil {
		return err
	}
	s, err := externalSession(m)
	if err != nil {
		return err
	}
	if s == nil {
		return fmt.Errorf("session stopped; use Revive to resume it in Motley")
	}
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("this session is running in its original terminal; automatic focus currently requires Ghostty on macOS")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	script := `on run argv
 tell application "Ghostty"
  set matches to {}
  repeat with term in terminals
   if working directory of term is item 1 of argv then set end of matches to term
  end repeat
  if (count matches) is not 1 then error "Cannot identify a unique Ghostty tab for this directory. Open its original tab."
  focus (item 1 of matches)
 end tell
end run`
	out, err := exec.CommandContext(ctx, "osascript", "-e", script, m.Worktree).CombinedOutput()
	if err != nil {
		return fmt.Errorf("focus Ghostty: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func importedLive(m Manifest) (bool, error) {
	sessions, err := tmux.Sessions()
	if err != nil {
		return false, err
	}
	for _, s := range sessions {
		if s.Name == tmux.SessionName(m.ID) {
			if s.MemberID != m.ID {
				return false, fmt.Errorf("session name is owned by another member")
			}
			return true, nil
		}
	}
	return false, nil
}

func sameDirectory(a, b string) bool {
	if p, err := filepath.EvalSymlinks(a); err == nil {
		a = p
	}
	if p, err := filepath.EvalSymlinks(b); err == nil {
		b = p
	}
	return filepath.Clean(a) == filepath.Clean(b)
}
