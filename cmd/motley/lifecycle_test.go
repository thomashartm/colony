package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/thomashartm/motley/internal/member"
)

func (f *memberFixture) refused(args ...string) string {
	f.t.Helper()
	out, err := exec.Command(f.bin, args...).CombinedOutput()
	if err == nil {
		f.t.Fatalf("expected refusal for %v: %s", args, out)
	}
	return string(out)
}
func (f *memberFixture) manifest(id string) member.Manifest {
	f.t.Helper()
	m, err := member.Load(filepath.Join(f.state, "motley/members"), id)
	if err != nil {
		f.t.Fatal(err)
	}
	return m
}
func buildLifecycleBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "motley")
	commandOutput(t, "go", "build", "-o", bin, ".")
	return bin
}
func TestFinishAndResume(t *testing.T) {
	bin := buildLifecycleBinary(t)
	t.Run("retire checks force and archive", func(t *testing.T) {
		f := newMemberFixture(t, bin, "main")
		f.motley("spawn", "--repo", "api", "--branch", "feat/retire", "--detach")
		id := "feat-retire"
		m := f.manifest(id)
		writeFixture(t, filepath.Join(m.Worktree, "dirty.txt"), "unfinished", 0600)
		if out := f.refused("retire", id); !strings.Contains(out, "uncommitted") {
			t.Fatal(out)
		}
		assertListState(t, f.motley("ls"), id, "alive")
		f.git(m.Worktree, "add", "dirty.txt")
		f.git(m.Worktree, "commit", "-m", "Unpushed work")
		if out := f.refused("retire", id); !strings.Contains(out, "1 commits") {
			t.Fatal(out)
		}
		f.git(m.Worktree, "branch", "--unset-upstream")
		if out := f.refused("retire", id); !strings.Contains(out, "no upstream") {
			t.Fatal(out)
		}
		dir := filepath.Join(f.state, "motley/members")
		events := "{\"schema\":1,\"agent\":\"claude\",\"agent_session_id\":\"recorded\"}\n"
		prompt := "<!-- schema = 1 -->\nOriginal prompt\n"
		writeFixture(t, filepath.Join(dir, id+".events.jsonl"), events, 0600)
		writeFixture(t, filepath.Join(dir, id+".prompt.md"), prompt, 0600)
		f.git(f.repo, "worktree", "lock", m.Worktree)
		f.motley("retire", id, "--force")
		if _, err := os.Stat(m.Worktree); !os.IsNotExist(err) {
			t.Fatal("worktree not removed", err)
		}
		if strings.Contains(f.git(f.repo, "worktree", "list", "--porcelain"), m.Worktree) {
			t.Fatal("worktree still registered")
		}
		if strings.Contains(f.motley("ls"), id) {
			t.Fatal("retired member still active")
		}
		if _, err := exec.Command("git", "-C", f.repo, "show-ref", "--verify", "refs/heads/"+m.Branch).Output(); err == nil {
			t.Fatal("local branch survived")
		}
		if f.git(f.remote, "rev-parse", "refs/heads/"+m.Branch) == "" {
			t.Fatal("remote branch deleted")
		}
		archived, err := member.Load(filepath.Join(dir, "archive"), id)
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
	t.Run("native resume and fresh starts for all agents", func(t *testing.T) {
		f := newMemberFixture(t, bin, "main")
		for _, agent := range []string{"claude", "codex", "opencode"} {
			id := "feat-" + agent
			f.motley("spawn", "--repo", "api", "--branch", "feat/"+agent, "--agent", agent, "--detach")
			marker := filepath.Join(f.home, "agent-"+id+".txt")
			eventually(t, func() bool { _, err := os.Stat(marker); return err == nil })
			m := f.manifest(id)
			f.refused("revive", id)
			f.tmux("kill-session", "-t", "="+id)
			sessionID := "11111111-1111-4111-8111-111111111111"
			log := fmt.Sprintf("{\"schema\":1,\"agent\":%q,\"agent_session_id\":%q}\n", agent, sessionID) + strings.Repeat("{\"schema\":1,\"event\":\"Notification\"}\n", 2500)
			writeFixture(t, filepath.Join(f.state, "motley/members", id+".events.jsonl"), log, 0600)
			receipt := filepath.Join(f.home, "resume-"+id)
			agentPath := filepath.Join(f.home, "fake agents", agent)
			writeFixture(t, agentPath, "#!/bin/sh\nprintf '%s\\n' \"$#\" \"$@\" > "+quoteShell(receipt)+"\n", 0755)
			f.motley("revive", id)
			eventually(t, func() bool { _, err := os.Stat(receipt); return err == nil })
			got, err := os.ReadFile(receipt)
			if err != nil {
				t.Fatal(err)
			}
			if agent == "claude" {
				if string(got) != "1\n--resume="+sessionID+"\n" {
					t.Fatalf("Claude resume argv: %q", got)
				}
			} else if agent == "codex" {
				if string(got) != "4\nresume\n--no-daemon\n--\n"+sessionID+"\n" {
					t.Fatalf("Codex resume argv: %q", got)
				}
			} else if string(got) != "1\n--session="+sessionID+"\n" {
				t.Fatalf("OpenCode resume argv: %q", got)
			}
			assertListState(t, f.motley("ls"), id, "alive")
			if after := f.manifest(id); after.CreatedAt != m.CreatedAt {
				t.Fatal("revive rewrote manifest")
			}
			f.tmux("kill-session", "-t", "="+id)
			if err := os.Remove(filepath.Join(f.state, "motley/members", id+".events.jsonl")); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(receipt); err != nil {
				t.Fatal(err)
			}
			f.motley("revive", id)
			fresh := "0"
			if agent == "codex" {
				fresh = "1\n--no-daemon"
			}
			eventually(t, func() bool { data, _ := os.ReadFile(receipt); return strings.TrimSpace(string(data)) == fresh })
			f.motley("retire", id, "--keep-branch")
			f.git(f.repo, "show-ref", "--verify", "refs/heads/"+m.Branch)
		}
	})
}

func TestReviveMainCheckoutPreservesWork(t *testing.T) {
	bin := buildLifecycleBinary(t)
	f := newMemberFixture(t, bin, "main")
	f.motley("spawn", "--repo", "api", "--branch", "feat/main-resume", "--detach")
	id := "feat-main-resume"
	f.tmux("kill-session", "-t", "="+id)
	m := f.manifest(id)
	m.Worktree = m.RepoPath
	save := func() {
		t.Helper()
		data, err := toml.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		writeFixture(t, filepath.Join(f.state, "motley/members", id+".toml"), string(data), 0600)
	}
	save()
	// A main checkout is allowed only when it still matches the recorded branch.
	if out := f.refused("revive", id); !strings.Contains(out, "identity changed") || strings.Contains(out, "remov") {
		t.Fatal("revive reused cleanup validation", out)
	}
	assertListState(t, f.motley("ls"), id, "dead")
	m.Branch = "main"
	save()
	writeFixture(t, filepath.Join(f.repo, "unfinished.txt"), "keep this work", 0600)
	beforeStatus := f.git(f.repo, "status", "--porcelain")
	beforeHead := f.git(f.repo, "rev-parse", "HEAD")
	beforeTrees := f.git(f.repo, "worktree", "list", "--porcelain")
	sessionID := "11111111-1111-4111-8111-111111111111"
	writeFixture(t, filepath.Join(f.state, "motley/members", id+".events.jsonl"),
		fmt.Sprintf("{\"schema\":1,\"agent\":\"claude\",\"agent_session_id\":%q}\n", sessionID), 0600)
	receipt := filepath.Join(f.home, "main-resume-receipt")
	writeFixture(t, filepath.Join(f.home, "fake agents", "claude"),
		"#!/bin/sh\nprintf '%s\n' \"$PWD\" \"$@\" > "+quoteShell(receipt)+"\n", 0755)
	f.motley("revive", id)
	eventually(t, func() bool { _, err := os.Stat(receipt); return err == nil })
	got, err := os.ReadFile(receipt)
	if err != nil || string(got) != m.Worktree+"\n--resume="+sessionID+"\n" {
		t.Fatalf("resume did not use main checkout and saved session: %q, %v", got, err)
	}
	assertListState(t, f.motley("ls"), id, "alive")
	// Allowing a restart must not weaken the main-checkout deletion safeguard.
	if out := f.refused("retire", id, "--force"); !strings.Contains(out, "main worktree") {
		t.Fatal(out)
	}
	assertListState(t, f.motley("ls"), id, "alive")
	if f.git(f.repo, "status", "--porcelain") != beforeStatus ||
		f.git(f.repo, "rev-parse", "HEAD") != beforeHead ||
		f.git(f.repo, "worktree", "list", "--porcelain") != beforeTrees {
		t.Fatal("revive changed files, branch or worktree registration")
	}
	data, err := os.ReadFile(filepath.Join(f.repo, "unfinished.txt"))
	if err != nil || string(data) != "keep this work" {
		t.Fatal("main checkout work was lost", err)
	}
}

func TestAdoptAndRetireSafeguards(t *testing.T) {
	bin := buildLifecycleBinary(t)
	t.Run("adopt linked worktree and protected branch", func(t *testing.T) {
		f := newMemberFixture(t, bin, "main")
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
		if got := f.tmux("display-message", "-p", "-t", "="+id+":", "#{@motley_member}|#{@motley_agent}"); got != id+"|claude" {
			t.Fatal(got)
		}
		if got := f.tmux("show-environment", "-t", "="+id, "MOTLEY_MEMBER"); got != "MOTLEY_MEMBER="+id {
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
		f.motley("retire", id)
		f.git(f.repo, "show-ref", "--verify", "refs/heads/develop")
		cmd = exec.Command(bin, "adopt")
		cmd.Dir = f.repo
		if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "main checkout") {
			t.Fatalf("adopt main: %s %v", out, err)
		}
	})
	t.Run("force cannot remove changed ownership", func(t *testing.T) {
		f := newMemberFixture(t, bin, "main")
		f.motley("spawn", "--repo", "api", "--branch", "feat/guard", "--detach")
		id := "feat-guard"
		m := f.manifest(id)
		f.git(m.Worktree, "checkout", "-b", "different")
		if out := f.refused("retire", id, "--force"); !strings.Contains(out, "identity changed") {
			t.Fatal(out)
		}
		assertListState(t, f.motley("ls"), id, "alive")
		f.git(m.Worktree, "checkout", m.Branch)
		// A malformed manifest must never authorize removal of the main checkout.
		m.Worktree = m.RepoPath
		data, err := toml.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		writeFixture(t, filepath.Join(f.state, "motley/members", id+".toml"), string(data), 0600)
		if out := f.refused("retire", id, "--force"); !strings.Contains(out, "main worktree") {
			t.Fatal(out)
		}
		if got := f.git(f.repo, "status", "--porcelain"); !strings.Contains(got, "tracked.txt") {
			t.Fatal("main checkout changed")
		}
	})
	t.Run("fallback verification and retry", func(t *testing.T) {
		f := newMemberFixture(t, bin, "main")
		f.motley("spawn", "--repo", "api", "--branch", "feat/fallback", "--detach")
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
		assertListState(t, f.motley("ls"), id, "dead")
		t.Setenv("PATH", originalPath)
		f.motley("retire", id)
		if strings.Contains(f.git(f.repo, "worktree", "list", "--porcelain"), "feat-fallback") {
			t.Fatal("retry did not prune registration")
		}
	})
	t.Run("retry cleanup with missing worktree and reused archive id", func(t *testing.T) {
		f := newMemberFixture(t, bin, "main")
		f.motley("spawn", "--repo", "api", "--branch", "feat/retry", "--detach")
		id := "feat-retry"
		m := f.manifest(id)
		f.tmux("kill-session", "-t", "="+id)
		f.git(f.repo, "worktree", "remove", "--force", m.Worktree)
		if out := f.refused("revive", id); !strings.Contains(out, "missing") {
			t.Fatal(out)
		}
		archive := filepath.Join(f.state, "motley/members/archive", id+".toml")
		writeFixture(t, archive, "schema = 1\n# previous retirement\n", 0600)
		f.motley("retire", id)
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
	f := newMemberFixture(t, bin, "main")
	f.motley("spawn", "--repo", "api", "--branch", "feat/dialog", "--detach")
	id := "feat-dialog"
	m := f.manifest(id)
	terminal := f.terminalClient(id)
	client := f.clientName(terminal)
	// Reproduce opening the overview in the very agent we want to retire.
	eventually(t, func() bool { _, err := os.Stat(filepath.Join(f.home, "agent-"+id+".txt")); return err == nil })
	f.tmux("send-keys", "-t", "="+id+":", "-l", quoteShell(bin))
	f.tmux("send-keys", "-t", "="+id+":", "Enter")
	eventually(t, func() bool { return f.clientSession(client) == "_motley" })
	screen := func() string { return f.tmux("capture-pane", "-p", "-t", "=_motley:") }
	defer func() {
		if t.Failed() {
			t.Log(screen())
		}
	}()
	eventually(t, func() bool { return strings.Contains(screen(), "1 alive") })
	writeFixture(t, filepath.Join(m.Worktree, "dirty.txt"), "unfinished", 0600)
	terminal.send(t, "d")
	eventually(t, func() bool { return strings.Contains(screen(), "Terminate "+id+"?") })
	terminal.send(t, "y")
	eventually(t, func() bool {
		return strings.Contains(screen(), "Terminated "+id) && strings.Contains(screen(), "0 alive")
	})
	assertListState(t, f.motley("ls"), id, "dead")
	data, err := os.ReadFile(filepath.Join(m.Worktree, "dirty.txt"))
	if err != nil || string(data) != "unfinished" {
		t.Fatal("termination lost work", err)
	}
	f.git(f.repo, "show-ref", "--verify", "refs/heads/"+m.Branch)
	if after := f.manifest(id); after.CreatedAt != m.CreatedAt {
		t.Fatal("termination rewrote manifest")
	}
	terminal.send(t, "r")
	eventually(t, func() bool { return strings.Contains(screen(), "Revived "+id) && strings.Contains(screen(), "1 alive") })
	terminal.send(t, "x")
	eventually(t, func() bool { return strings.Contains(screen(), "Dirty/untracked files: YES") })
	terminal.send(t, "y")
	eventually(t, func() bool { return strings.Contains(screen(), "Work would be discarded") })
	assertListState(t, f.motley("ls"), id, "alive")
	terminal.send(t, "f")
	eventually(t, func() bool { return strings.Contains(screen(), "Force: true") })
	terminal.send(t, "y")
	eventually(t, func() bool { return strings.Contains(screen(), "Retired "+id) })
	if _, err := os.Stat(m.Worktree); !os.IsNotExist(err) {
		t.Fatal("TUI did not remove worktree", err)
	}
	if f.clientSession(client) != "_motley" {
		t.Fatal("retirement killed or left monitor")
	}
}

func TestTerminateOwnership(t *testing.T) {
	f := newMemberFixture(t, buildLifecycleBinary(t), "main")
	f.motley("spawn", "--repo", "api", "--branch", "feat/stop", "--detach")
	id := "feat-stop"
	f.tmux("set-option", "-t", "="+id+":", "@motley_member", "someone-else")
	if err := member.Terminate(id); err == nil {
		t.Fatal("terminated another owner's session")
	}
	f.tmux("has-session", "-t", "="+id)
	f.tmux("set-option", "-t", "="+id+":", "@motley_member", id)
	t.Setenv("TMUX_PANE", f.tmux("display-message", "-p", "-t", "="+id+":", "#{pane_id}"))
	if err := member.Terminate(id); err == nil || !strings.Contains(err.Error(), "motley monitor") {
		t.Fatal("self termination not redirected", err)
	}
}
