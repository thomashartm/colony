package member

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/thomashartm/motley/internal/agents/claude"
	"github.com/thomashartm/motley/internal/agents/codex"
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
			if m.Agent == "claude" && (m.ClaudeSession == s.SessionID || (m.ClaudeSession == "" && sameDirectory(m.Worktree, s.Cwd))) {
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
	return Import("claude", sessionID, name, crewID)
}

func Import(agent, sessionID, name, crewID string) (Manifest, error) {
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
	sessions, err := DiscoverImports(agent)
	if err != nil {
		return Manifest{}, err
	}
	var selected *ImportCandidate
	for i := range sessions {
		if sessions[i].SessionID == sessionID {
			selected = &sessions[i]
			break
		}
	}
	if selected == nil {
		return Manifest{}, fmt.Errorf("the %s session %s is no longer available or is already in Motley", agent, sessionID)
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
	// Git metadata is optional. Existing sessions may run outside a repo.
	repo := ""
	branch, base, remote := "", "", ""
	if rows, e := gitx.Worktrees(cwd); e == nil && len(rows) > 0 {
		repo = rows[0].Path
		branch, _ = gitx.Output(cwd, "symbolic-ref", "--short", "HEAD")
		base, _ = worktree.Base(repo)
		remote, _ = gitx.Output(repo, "remote", "get-url", "origin")
	}
	id := agent + "-" + sessionID
	if _, err := os.Lstat(filepath.Join(dir, id+".toml")); !os.IsNotExist(err) {
		return Manifest{}, fmt.Errorf("member %s already exists", id)
	}
	m := Manifest{Schema: 1, ID: id, Name: name, Repo: filepath.Base(cwd), RepoPath: repo, Worktree: cwd, Branch: branch, Base: base, RemoteURL: remote, Agent: agent, Crew: crewID, CreatedAt: time.Now().UTC()}
	if agent == "codex" {
		m.CodexSession, m.CodexSocket = sessionID, s.Socket
	} else {
		m.ClaudeSession = sessionID
	}
	if err := saveManifest(dir, m); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

// refreshClaude uses one bounded discovery call for all imported Claude members.
// Never infer that external sessions died when discovery itself fails.
func refreshClaude(rows []Row) ([]Row, error) {
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

// ExternalTerminal explains where an imported session runs. Motley controls
// terminals only through tmux, so it never focuses or drives the original one,
// and it never starts a second Claude writer against an open conversation.
func ExternalTerminal(id string) error {
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
	return fmt.Errorf("%s runs in its original terminal in %s; switch to it there, or Terminate and Revive to run it in Motley", m.Name, m.Worktree)
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

// ImportCandidate is the common CLI/TUI representation; control stays agent-specific.
type ImportCandidate struct{ Agent, SessionID, Name, Cwd, Status, Socket string }

func DiscoverImports(agent string) ([]ImportCandidate, error) {
	var candidates []ImportCandidate
	switch agent {
	case "claude":
		sessions, err := DiscoverClaude()
		if err != nil {
			return nil, err
		}
		for _, s := range sessions {
			candidates = append(candidates, ImportCandidate{Agent: agent, SessionID: s.SessionID, Name: s.Name, Cwd: s.Cwd, Status: s.MotleyStatus()})
		}
	case "codex":
		sessions, err := codex.Sessions("")
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
		for _, s := range sessions {
			if CheckID(s.ID) != nil {
				continue
			}
			managed := false
			for _, m := range members {
				if m.Agent == "codex" && (m.CodexSession == s.ID || (m.CodexSession == "" && sameDirectory(m.Worktree, s.Cwd))) {
					managed = true
					break
				}
			}
			if !managed {
				candidates = append(candidates, ImportCandidate{Agent: agent, SessionID: s.ID, Name: s.Name, Cwd: s.Cwd, Status: s.MotleyStatus(), Socket: s.Socket})
			}
		}
	default:
		return nil, fmt.Errorf("import supports claude or codex")
	}
	return candidates, nil
}

func RefreshExternal(rows []Row) ([]Row, error) {
	rows, err := refreshClaude(rows)
	if err != nil {
		return rows, err
	}
	bySocket := map[string][]codex.Session{}
	for i := range rows {
		r := &rows[i]
		if r.CodexSession == "" {
			continue
		}
		sessions, ok := bySocket[r.CodexSocket]
		if !ok {
			sessions, err = codex.Sessions(r.CodexSocket)
			if err != nil {
				return rows, err
			}
			bySocket[r.CodexSocket] = sessions
		}
		r.External = !r.Alive
		for _, s := range sessions {
			if s.ID == r.CodexSession {
				if !sameDirectory(s.Cwd, r.Worktree) {
					return rows, fmt.Errorf("codex session directory changed; remove and import the member again")
				}
				r.Alive, r.Status, r.Seen = true, s.MotleyStatus(), time.Now().Unix()
				break
			}
		}
	}
	return rows, nil
}

// ValidateCodex checks the saved thread on its original server before connecting.
func ValidateCodex(m Manifest) error {
	_, err := validatedCodexSession(m)
	return err
}

func validatedCodexSession(m Manifest) (codex.Session, error) {
	s, err := codex.ReadSession(m.CodexSocket, m.CodexSession)
	if err != nil {
		return s, err
	}
	if !sameDirectory(s.Cwd, m.Worktree) {
		return s, fmt.Errorf("codex session directory changed; remove and import the member again")
	}
	return s, nil
}

// PrepareOpen creates only a terminal client to the existing Codex server.
func PrepareOpen(id string) error {
	dir, err := state.MembersDir()
	if err != nil {
		return err
	}
	m, err := Load(dir, id)
	if err != nil {
		return err
	}
	if m.CodexSession == "" {
		return nil
	}
	if err := ValidateCodex(m); err != nil {
		return err
	}
	live, err := importedLive(m)
	if err != nil || live {
		return err
	}
	return Revive(id)
}

func stopImported(m Manifest) error {
	if m.CodexSession != "" {
		return nil
	} // The shared thread survives removal of the terminal client.
	return stopExternal(m)
}
