package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/thomashartm/colony/internal/minion"
)

func (f *minionFixture) refused(args ...string) string {
	f.t.Helper()
	out, err := exec.Command(f.bin, args...).CombinedOutput()
	if err == nil {
		f.t.Fatalf("expected refusal for %v: %s", args, out)
	}
	return string(out)
}
func (f *minionFixture) manifest(id string) minion.Manifest {
	f.t.Helper()
	m, err := minion.Load(filepath.Join(f.state, "colony/minions"), id)
	if err != nil {
		f.t.Fatal(err)
	}
	return m
}
func buildLifecycleBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "colony")
	commandOutput(t, "go", "build", "-o", bin, ".")
	return bin
}
func TestFinishAndResume(t *testing.T) {
	bin := buildLifecycleBinary(t)
	t.Run("retire checks force and archive", func(t *testing.T) {
		f := newMinionFixture(t, bin, "main")
		f.colony("spawn", "--repo", "api", "--branch", "feat/retire", "--detach")
		id := "feat-retire"
		m := f.manifest(id)
		writeFixture(t, filepath.Join(m.Worktree, "dirty.txt"), "unfinished", 0600)
		if out := f.refused("retire", id); !strings.Contains(out, "uncommitted") {
			t.Fatal(out)
		}
		assertListState(t, f.colony("ls"), id, "alive")
		f.git(m.Worktree, "add", "dirty.txt")
		f.git(m.Worktree, "commit", "-m", "Unpushed work")
		if out := f.refused("retire", id); !strings.Contains(out, "1 commits") {
			t.Fatal(out)
		}
		f.git(m.Worktree, "branch", "--unset-upstream")
		if out := f.refused("retire", id); !strings.Contains(out, "no upstream") {
			t.Fatal(out)
		}
		dir := filepath.Join(f.state, "colony/minions")
		events := "{\"schema\":1,\"agent\":\"claude\",\"agent_session_id\":\"recorded\"}\n"
		prompt := "<!-- schema = 1 -->\nOriginal prompt\n"
		writeFixture(t, filepath.Join(dir, id+".events.jsonl"), events, 0600)
		writeFixture(t, filepath.Join(dir, id+".prompt.md"), prompt, 0600)
		f.git(f.repo, "worktree", "lock", m.Worktree)
		f.colony("retire", id, "--force")
		if _, err := os.Stat(m.Worktree); !os.IsNotExist(err) {
			t.Fatal("worktree not removed", err)
		}
		if strings.Contains(f.git(f.repo, "worktree", "list", "--porcelain"), m.Worktree) {
			t.Fatal("worktree still registered")
		}
		if strings.Contains(f.colony("ls"), id) {
			t.Fatal("retired minion still active")
		}
		if _, err := exec.Command("git", "-C", f.repo, "show-ref", "--verify", "refs/heads/"+m.Branch).Output(); err == nil {
			t.Fatal("local branch survived")
		}
		if f.git(f.remote, "rev-parse", "refs/heads/"+m.Branch) == "" {
			t.Fatal("remote branch deleted")
		}
		archived, err := minion.Load(filepath.Join(dir, "archive"), id)
		if err != nil || archived.RetiredAt == nil || archived.RetiredAt.Before(m.CreatedAt) {
			t.Fatal("missing retirement timestamp", err)
		}
		for suffix, want := range map[string]string{".events.jsonl": events, ".prompt.md": prompt} {
			got, err := os.ReadFile(filepath.Join(dir, "archive", id+suffix))
			if err != nil || string(got) != want {
				t.Fatal("archive differs", suffix, err)
			}
			if _, err := os.Stat(filepath.Join(dir, id+suffix)); !os.IsNotExist(err) {
				t.Fatal("active file retained", suffix)
			}
		}
		f.refused("revive", id)
	})
	t.Run("Claude resume and fresh other agents", func(t *testing.T) {
		f := newMinionFixture(t, bin, "main")
		for _, agent := range []string{"claude", "codex", "opencode"} {
			id := "feat-" + agent
			f.colony("spawn", "--repo", "api", "--branch", "feat/"+agent, "--agent", agent, "--detach")
			marker := filepath.Join(f.home, "agent-"+id+".txt")
			eventually(t, func() bool { _, err := os.Stat(marker); return err == nil })
			m := f.manifest(id)
			f.refused("revive", id)
			f.tmux("kill-session", "-t", "="+id)
			sessionID := "11111111-1111-4111-8111-111111111111"
			log := fmt.Sprintf("{\"schema\":1,\"agent\":%q,\"agent_session_id\":%q}\n", agent, sessionID) + strings.Repeat("{\"schema\":1,\"event\":\"Notification\"}\n", 2500)
			writeFixture(t, filepath.Join(f.state, "colony/minions", id+".events.jsonl"), log, 0600)
			receipt := filepath.Join(f.home, "resume-"+id)
			agentPath := filepath.Join(f.home, "fake agents", agent)
			writeFixture(t, agentPath, "#!/bin/sh\nprintf '%s\\n' \"$#\" \"$@\" > "+quoteShell(receipt)+"\n", 0755)
			f.colony("revive", id)
			eventually(t, func() bool { _, err := os.Stat(receipt); return err == nil })
			got, err := os.ReadFile(receipt)
			if err != nil {
				t.Fatal(err)
			}
			if agent == "claude" {
				if string(got) != "1\n--resume="+sessionID+"\n" {
					t.Fatalf("Claude resume argv: %q", got)
				}
			} else if strings.TrimSpace(string(got)) != "0" {
				t.Fatalf("%s must start fresh: %q", agent, got)
			}
			assertListState(t, f.colony("ls"), id, "alive")
			if after := f.manifest(id); after.CreatedAt != m.CreatedAt {
				t.Fatal("revive rewrote manifest")
			}
			f.tmux("kill-session", "-t", "="+id)
			if err := os.Remove(filepath.Join(f.state, "colony/minions", id+".events.jsonl")); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(receipt); err != nil {
				t.Fatal(err)
			}
			f.colony("revive", id)
			eventually(t, func() bool { data, _ := os.ReadFile(receipt); return strings.TrimSpace(string(data)) == "0" })
			f.colony("retire", id, "--keep-branch")
			f.git(f.repo, "show-ref", "--verify", "refs/heads/"+m.Branch)
		}
	})
}

func TestAdoptAndRetireSafeguards(t *testing.T) {
	bin := buildLifecycleBinary(t)
	t.Run("adopt linked worktree and protected branch", func(t *testing.T) {
		f := newMinionFixture(t, bin, "main")
		path := filepath.Join(f.trees, "manual")
		f.git(f.repo, "worktree", "add", "-b", "develop", path, "main")
		f.tmux("new-session", "-d", "-s", "manual", "-c", path, "/bin/sh")
		pane := f.tmux("display-message", "-p", "-t", "=manual:", "#{pane_id}")
		originalPane := os.Getenv("TMUX_PANE")
		t.Setenv("TMUX_PANE", pane)
		cmd := exec.Command(bin, "adopt", "--agent", "claude", "--ticket", "42", "--name", "Existing work")
		cmd.Dir = path
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("adopt: %v %s", err, out)
		}
		id := "42-existing-work"
		m := f.manifest(id)
		if m.Branch != "develop" || m.Agent != "claude" || m.Ticket != "42" {
			t.Fatalf("adopted manifest: %+v", m)
		}
		if got := f.tmux("display-message", "-p", "-t", "="+id+":", "#{@colony_minion}|#{@colony_agent}"); got != id+"|claude" {
			t.Fatal(got)
		}
		if got := f.tmux("show-environment", "-t", "="+id, "COLONY_MINION"); got != "COLONY_MINION="+id {
			t.Fatal(got)
		}
		cmd = exec.Command(bin, "adopt")
		cmd.Dir = path
		if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "already belongs") {
			t.Fatalf("duplicate adoption: %s %v", out, err)
		}
		if out := f.refused("retire", id, "--force"); !strings.Contains(out, "another tmux session") {
			t.Fatal("self-retirement should refuse", out)
		}
		t.Setenv("TMUX_PANE", originalPane)
		f.colony("retire", id)
		f.git(f.repo, "show-ref", "--verify", "refs/heads/develop")
		cmd = exec.Command(bin, "adopt")
		cmd.Dir = f.repo
		if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "main checkout") {
			t.Fatalf("adopt main: %s %v", out, err)
		}
	})
	t.Run("force cannot remove changed ownership", func(t *testing.T) {
		f := newMinionFixture(t, bin, "main")
		f.colony("spawn", "--repo", "api", "--branch", "feat/guard", "--detach")
		id := "feat-guard"
		m := f.manifest(id)
		f.git(m.Worktree, "checkout", "-b", "different")
		if out := f.refused("retire", id, "--force"); !strings.Contains(out, "identity changed") {
			t.Fatal(out)
		}
		assertListState(t, f.colony("ls"), id, "alive")
		f.git(m.Worktree, "checkout", m.Branch)
		// A malformed manifest must never authorize removal of the main checkout.
		m.Worktree = m.RepoPath
		data, err := toml.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		writeFixture(t, filepath.Join(f.state, "colony/minions", id+".toml"), string(data), 0600)
		if out := f.refused("retire", id, "--force"); !strings.Contains(out, "main worktree") {
			t.Fatal(out)
		}
		if got := f.git(f.repo, "status", "--porcelain"); !strings.Contains(got, "tracked.txt") {
			t.Fatal("main checkout changed")
		}
	})
	t.Run("fallback verification and retry", func(t *testing.T) {
		f := newMinionFixture(t, bin, "main")
		f.colony("spawn", "--repo", "api", "--branch", "feat/fallback", "--detach")
		id := "feat-fallback"
		m := f.manifest(id)
		f.git(f.repo, "worktree", "lock", m.Worktree)
		gitBin, err := exec.LookPath("git")
		if err != nil {
			t.Fatal(err)
		}
		originalPath := os.Getenv("PATH")
		wrapper := filepath.Join(f.home, "git-wrapper")
		script := "#!/bin/sh\nif [ \"$3\" = worktree ]; then\n case \"$4\" in remove|unlock) exit 1;; esac\nfi\nexec " + quoteShell(gitBin) + " \"$@\"\n"
		writeFixture(t, filepath.Join(wrapper, "git"), script, 0755)
		t.Setenv("PATH", wrapper+":"+originalPath)
		if out := f.refused("retire", id); !strings.Contains(out, "git still lists") {
			t.Fatal(out)
		}
		if _, err := os.Stat(m.Worktree); !os.IsNotExist(err) {
			t.Fatal("fallback did not remove directory", err)
		}
		f.git(f.repo, "show-ref", "--verify", "refs/heads/"+m.Branch)
		assertListState(t, f.colony("ls"), id, "dead")
		t.Setenv("PATH", originalPath)
		f.colony("retire", id)
		if strings.Contains(f.git(f.repo, "worktree", "list", "--porcelain"), "feat-fallback") {
			t.Fatal("retry did not prune registration")
		}
	})
	t.Run("retry cleanup with missing worktree and reused archive id", func(t *testing.T) {
		f := newMinionFixture(t, bin, "main")
		f.colony("spawn", "--repo", "api", "--branch", "feat/retry", "--detach")
		id := "feat-retry"
		m := f.manifest(id)
		f.tmux("kill-session", "-t", "="+id)
		f.git(f.repo, "worktree", "remove", "--force", m.Worktree)
		if out := f.refused("revive", id); !strings.Contains(out, "missing") {
			t.Fatal(out)
		}
		archive := filepath.Join(f.state, "colony/minions/archive", id+".toml")
		writeFixture(t, archive, "schema = 1\n# previous retirement\n", 0600)
		f.colony("retire", id)
		data, err := os.ReadFile(archive)
		if err != nil || !strings.Contains(string(data), "previous retirement") {
			t.Fatal("archive overwritten")
		}
		files, err := filepath.Glob(filepath.Join(filepath.Dir(archive), id+"-*.toml"))
		if err != nil || len(files) != 1 {
			t.Fatal("collision archive missing", files, err)
		}
	})
}

func TestLifecycleTUI(t *testing.T) {
	bin := buildLifecycleBinary(t)
	f := newMinionFixture(t, bin, "main")
	f.colony("spawn", "--repo", "api", "--branch", "feat/dialog", "--detach")
	id := "feat-dialog"
	m := f.manifest(id)
	f.tmux("kill-session", "-t", "="+id)
	terminal := startTerminal(t, exec.Command(bin, "--monitor"))
	eventually(t, func() bool { return strings.Contains(terminal.text(), "dialog") })
	terminal.send(t, "r")
	eventually(t, func() bool { return strings.Contains(terminal.text(), "Revived "+id) })
	assertListState(t, f.colony("ls"), id, "alive")
	writeFixture(t, filepath.Join(m.Worktree, "dirty.txt"), "unfinished", 0600)
	terminal.send(t, "x")
	eventually(t, func() bool { return strings.Contains(terminal.text(), "Dirty/untracked files: YES") })
	terminal.send(t, "y")
	eventually(t, func() bool { return strings.Contains(terminal.text(), "Work would be discarded") })
	assertListState(t, f.colony("ls"), id, "alive")
	terminal.send(t, "f")
	// Wait for the explicit force toggle to render before confirming.
	eventually(t, func() bool { return strings.Contains(terminal.text(), "Force: true") })
	terminal.send(t, "y")
	eventually(t, func() bool { return strings.Contains(terminal.text(), "Retired "+id) })
	if _, err := os.Stat(m.Worktree); !os.IsNotExist(err) {
		t.Fatal("TUI did not remove worktree", err)
	}
}
