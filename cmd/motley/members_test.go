package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pelletier/go-toml/v2"
	"github.com/thomashartm/motley/internal/member"
)

func TestMemberLifecycle(t *testing.T) {
	for _, tool := range []string{"git", "tmux", "cp"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Fatalf("integration tests require %s: %v", tool, err)
		}
	}
	bin := filepath.Join(t.TempDir(), "motley")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	for _, base := range []string{"main", "master"} {
		t.Run(base, func(t *testing.T) {
			f := newMemberFixture(t, bin, base)
			before := f.git(f.repo, "status", "--porcelain")
			agentNames := []string{"claude", "codex", "opencode"}
			if base == "master" {
				agentNames = agentNames[:1]
			}
			var firstID string
			for _, agent := range agentNames {
				branch, id := "feat/"+agent+".session", "feat-"+agent+".session"
				args := []string{"spawn", "--repo", "api", "--branch", branch, "--detach", "--agent", agent}
				if agent == "claude" {
					args = append(args, "--ticket", "412", "--name", "FX cache")
					id = "412-fx-cache"
					firstID = id
				}
				out := f.motley(args...)
				if !strings.Contains(out, "Created "+id) {
					t.Fatalf("spawn output: %s", out)
				}
				data, err := os.ReadFile(filepath.Join(f.state, "motley/members", id+".toml"))
				if err != nil {
					t.Fatal(err)
				}
				var m member.Manifest
				if err := toml.Unmarshal(data, &m); err != nil {
					t.Fatal(err)
				}
				wantPath := filepath.Join(f.trees, "api", strings.ReplaceAll(branch, "/", "-"))
				if m.Schema != 1 || m.ID != id || m.Branch != branch || m.Base != base || m.Agent != agent || m.Worktree != wantPath || m.CreatedAt.IsZero() {
					t.Fatalf("manifest: %+v", m)
				}
				if got := f.git(wantPath, "rev-parse", "--abbrev-ref", "@{upstream}"); got != "origin/"+branch {
					t.Fatalf("upstream: %s", got)
				}
				if f.git(wantPath, "rev-parse", "HEAD") != f.git(f.remote, "rev-parse", "refs/heads/"+branch) {
					t.Fatal("branch was not pushed")
				}
				copied, err := os.ReadFile(filepath.Join(wantPath, ".env"))
				if err != nil || string(copied) != "fixture env" {
					t.Fatalf("env copy: %q %v", copied, err)
				}
				session := strings.ReplaceAll(id, ".", "_")
				options := f.tmux("display-message", "-p", "-t", "="+session+":", "#{@motley_member}|#{@motley_ticket}|#{@motley_agent}")
				if options != id+"|"+m.Ticket+"|"+agent {
					t.Fatalf("session options: %q", options)
				}
				marker := filepath.Join(f.home, "agent-"+id+".txt")
				eventually(t, func() bool { _, err := os.Stat(marker); return err == nil })
				receipt, err := os.ReadFile(marker)
				if err != nil {
					t.Fatal(err)
				}
				lines := strings.Split(strings.TrimSpace(string(receipt)), "\n")
				argc := "0"
				if agent == "codex" {
					argc = "1"
				} // --no-daemon isolates hook environment
				if len(lines) != 4 || lines[0] != agent || lines[2] != id || lines[3] != argc {
					t.Fatalf("agent receipt (no prompt expected): %q", receipt)
				}
				cwd, err := filepath.EvalSymlinks(lines[1])
				if err != nil {
					t.Fatal(err)
				}
				physical, err := filepath.EvalSymlinks(wantPath)
				if err != nil || cwd != physical {
					t.Fatalf("agent cwd=%s, want %s (%v)", cwd, physical, err)
				}
				assertListState(t, f.motley("ls"), id, "alive")
			}
			if after := f.git(f.repo, "status", "--porcelain"); before != after {
				t.Fatalf("source working tree changed: before %q, after %q", before, after)
			}
			// After the fake agent exits, the pane remains an interactive shell.
			f.tmux("send-keys", "-t", "="+firstID+":", "-l", `printf shell-alive > "$HOME/shell-alive"`)
			f.tmux("send-keys", "-t", "="+firstID+":", "Enter")
			eventually(t, func() bool {
				data, _ := os.ReadFile(filepath.Join(f.home, "shell-alive"))
				return string(data) == "shell-alive"
			})

			// A real control-mode client verifies switching without any user terminal.
			client := exec.Command(f.tmuxBin, "-L", f.socket, "-C", "attach-session", "-t", "fixture")
			input, err := client.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			client.Stdout, client.Stderr = io.Discard, io.Discard
			if err := client.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = input.Close(); _ = client.Process.Kill(); _ = client.Wait() })
			eventually(t, func() bool { return strings.TrimSpace(f.tmux("list-clients", "-F", "#{client_session}")) == "fixture" })
			f.motley("switch", firstID)
			if got := f.tmux("list-clients", "-F", "#{client_session}"); got != firstID {
				t.Fatalf("switch client: got %q", got)
			}

			beforeTrees := f.git(f.repo, "worktree", "list", "--porcelain")
			for _, args := range [][]string{
				{"spawn", "--repo", "api", "--branch", "feat/claude.session", "--detach"},
				{"spawn", "--repo", "api", "--branch", "feat/new", "--agent", "invalid", "--detach"},
				{"spawn", "--repo", "../api", "--branch", "feat/new", "--detach"},
			} {
				if out, err := exec.Command(bin, args...).CombinedOutput(); err == nil {
					t.Fatalf("expected refusal for %v: %s", args, out)
				}
			}
			if f.git(f.repo, "worktree", "list", "--porcelain") != beforeTrees {
				t.Fatal("rejected spawn changed worktrees")
			}
			f.tmux("kill-session", "-t", "="+firstID)
			assertListState(t, f.motley("ls"), firstID, "dead")
			if out, err := exec.Command(bin, "switch", firstID).CombinedOutput(); err == nil || !strings.Contains(string(out), "is dead") {
				t.Fatalf("switch dead member must fail clearly: %v %s", err, out)
			}
			f.tmux("kill-server")
			eventually(t, func() bool {
				out, err := exec.Command(bin, "ls").CombinedOutput()
				return err == nil && !strings.Contains(string(out), "alive")
			})
			assertListState(t, f.motley("ls"), firstID, "dead")
		})
	}
	t.Run("attach and switch argv", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("XDG_STATE_HOME", home)
		writeFixture(t, filepath.Join(home, "motley/members/feat-example.v2.toml"), "schema = 1\nid = 'feat-example.v2'\n", 0o600)
		fakeBin := filepath.Join(home, "bin")
		writeFixture(t, filepath.Join(fakeBin, "tmux"), `#!/bin/sh
if [ "$1" = -u ]; then shift; fi
if [ "$1" = list-sessions ]; then
  printf 'feat-example_v2\tfeat-example.v2\n'
elif [ "$1" = list-keys ]; then
  printf 'unknown key\n'
  exit 1
else
  printf '%s\n' "$@" > "$HOME/tmux-args"
fi
`, 0o755)
		t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
		for _, test := range []struct{ command, context, want string }{
			{"attach", "", "attach-session"},
			{"switch", "", "attach-session"},
			{"switch", "isolated-test-context", "switch-client"},
		} {
			t.Setenv("TMUX", test.context)
			commandOutput(t, bin, test.command, "feat-example.v2")
			data, err := os.ReadFile(filepath.Join(home, "tmux-args"))
			if err != nil || string(data) != test.want+"\n-t\n=feat-example_v2\n" {
				t.Fatalf("%s in context %q: %q (%v)", test.command, test.context, data, err)
			}
		}
	})
}

type memberFixture struct {
	t                                *testing.T
	bin, tmuxBin, socket             string
	home, state, trees, repo, remote string
}

func newMemberFixture(t *testing.T, bin, base string) *memberFixture {
	t.Helper()
	root := t.TempDir()
	f := &memberFixture{t: t, bin: bin, home: root, state: filepath.Join(root, "state"), trees: filepath.Join(root, "work trees 'quoted'"), repo: filepath.Join(root, "repos/api"), remote: filepath.Join(root, "origin.git"), socket: fmt.Sprintf("motley-test-%d", time.Now().UnixNano())}
	var err error
	f.tmuxBin, err = exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{
		"HOME": root, "XDG_CONFIG_HOME": filepath.Join(root, "config"), "XDG_STATE_HOME": f.state,
		"TMUX": "", "TMUX_PANE": "", "SHELL": "/bin/sh", "GIT_CONFIG_GLOBAL": "/dev/null", "GIT_CONFIG_NOSYSTEM": "1",
		"HISTFILE":        "/dev/null",
		"GIT_AUTHOR_NAME": "Motley Test", "GIT_AUTHOR_EMAIL": "motley@example.invalid", "GIT_COMMITTER_NAME": "Motley Test", "GIT_COMMITTER_EMAIL": "motley@example.invalid",
		"GIT_DIR": "", "GIT_WORK_TREE": "", "GIT_INDEX_FILE": "",
	} {
		// Unset git path overrides rather than supplying an empty Git path.
		if key == "GIT_DIR" || key == "GIT_WORK_TREE" || key == "GIT_INDEX_FILE" {
			t.Setenv(key, "")
			if err := os.Unsetenv(key); err != nil {
				t.Fatal(err)
			}
		} else {
			t.Setenv(key, value)
		}
	}
	f.git(root, "init", "--bare", "--initial-branch="+base, f.remote)
	f.git(root, "init", "--initial-branch="+base, f.repo)
	f.git(f.repo, "config", "commit.gpgsign", "false")
	writeFixture(t, filepath.Join(f.repo, ".gitignore"), ".env\n.env.*\ngraphify-out/\n", 0o644)
	writeFixture(t, filepath.Join(f.repo, "tracked.txt"), "committed", 0o644)
	f.git(f.repo, "add", ".")
	f.git(f.repo, "commit", "-m", "Fixture")
	f.git(f.repo, "remote", "add", "origin", f.remote)
	f.git(f.repo, "push", "-u", "origin", base)
	writeFixture(t, filepath.Join(f.repo, "tracked.txt"), "uncommitted main work", 0o644)
	writeFixture(t, filepath.Join(f.repo, ".env"), "fixture env", 0o600)
	writeFixture(t, filepath.Join(f.repo, "graphify-out/graph.json"), "fixture graph", 0o640)
	cfg, err := toml.Marshal(map[string]any{"schema": 1, "repos_root": filepath.Dir(f.repo), "worktrees_root": f.trees})
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, filepath.Join(root, "config/motley/config.toml"), string(cfg), 0o600)
	// Start the server before changing PATH, exercising per-session environment.
	f.tmux("-f", "/dev/null", "new-session", "-d", "-s", "fixture", "/bin/sh")
	// Tests write a prefix and its key in one burst, often within a millisecond
	// of attaching. tmux would treat that as a paste and send the key to the pane.
	f.tmux("set-option", "-g", "assume-paste-time", "0")
	t.Cleanup(func() { _ = exec.Command(f.tmuxBin, "-L", f.socket, "kill-server").Run() })
	socketPath := f.tmux("display-message", "-p", "-t", "fixture", "#{socket_path}")
	t.Setenv("TMUX", socketPath+","+f.tmux("display-message", "-p", "-t", "fixture", "#{pid}")+",0")
	t.Setenv("TMUX_PANE", f.tmux("display-message", "-p", "-t", "fixture", "#{pane_id}"))
	fakeBin := filepath.Join(root, "fake agents")
	for _, agent := range []string{"claude", "codex", "opencode"} {
		writeFixture(t, filepath.Join(fakeBin, agent), "#!/bin/sh\nprintf '%s\\n' \"$(basename \"$0\")\" \"$PWD\" \"$MOTLEY_MEMBER\" \"$#\" > \"$HOME/agent-$MOTLEY_MEMBER.txt\"\n", 0o755)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return f
}

func (f *memberFixture) git(dir string, args ...string) string {
	f.t.Helper()
	return commandOutput(f.t, "git", append([]string{"-C", dir}, args...)...)
}

func (f *memberFixture) tmux(args ...string) string {
	f.t.Helper()
	return commandOutput(f.t, f.tmuxBin, append([]string{"-L", f.socket}, args...)...)
}

func (f *memberFixture) motley(args ...string) string {
	f.t.Helper()
	return commandOutput(f.t, f.bin, args...)
}

func commandOutput(t *testing.T, bin string, args ...string) string {
	t.Helper()
	out, err := exec.Command(bin, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", bin, args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func writeFixture(t *testing.T, path, data string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), mode); err != nil {
		t.Fatal(err)
	}
}

func eventually(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("timed out waiting for fixture session")
}

func assertListState(t *testing.T, out, id, state string) {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == id {
			if fields[len(fields)-1] != state {
				t.Fatalf("want %s %s, got %s", id, state, line)
			}
			return
		}
	}
	t.Fatalf("member %s absent from list:\n%s", id, out)
}
