package member

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thomashartm/motley/internal/blueprint"
	"github.com/thomashartm/motley/internal/config"
)

// prepareRepo builds a repository with one commit, origin set to remote, and
// fake claude and tmux binaries on PATH.
func prepareRepo(t *testing.T, remote string) (cfg config.Config, root, repo, bin string) {
	t.Helper()
	root = t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(root, "state"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	repo = filepath.Join(root, "repos/api")
	if err := os.MkdirAll(repo, 0700); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-b", "main"}, {"config", "user.name", "Fixture"}, {"config", "user.email", "fixture@example.invalid"}, {"commit", "--allow-empty", "-m", "Fixture"}, {"remote", "add", "origin", strings.ReplaceAll(remote, "ROOT", root)}} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("%v %s", err, out)
		}
	}
	bin = filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"claude", "tmux"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\nexit 99\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return config.Config{ReposRoot: filepath.Dir(repo), WorktreesRoot: filepath.Join(root, "trees")}, root, repo, bin
}

func writeBlueprint(t *testing.T, repo, name, body string) {
	t.Helper()
	dir := filepath.Join(repo, ".motley/blueprints")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".md"), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestPrepareIsReadOnly(t *testing.T) {
	cfg, root, repo, bin := prepareRepo(t, "ROOT/remote.git")
	git := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("%v %s", err, out)
		}
	}
	bp := filepath.Join(repo, ".motley/blueprints")
	if err := os.MkdirAll(bp, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bp, "plan.md"), []byte("+++\nargs=['--permission-mode','plan']\n+++\n{{.Repo}} {{.Branch}} {{.Base}} {{.Worktree}} {{.Vars.constraints}}"), 0600); err != nil {
		t.Fatal(err)
	}
	p, err := Prepare(cfg, SpawnOptions{Repo: "api", Branch: "feat/preview", Blueprint: "plan", Vars: []string{"constraints=Read only"}})
	if err != nil {
		t.Fatal(err)
	}
	if p.Manifest.ID != "" || p.Manifest.Agent != "claude" || p.Manifest.Name != "preview" || !strings.Contains(p.Prompt, "Read only") || !p.HasPrompt {
		t.Fatal(p)
	}
	for _, path := range []string{p.Manifest.Worktree, filepath.Join(root, "state")} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("prepare wrote %s: %v", path, err)
		}
	}
	if _, err := exec.Command("git", "-C", repo, "show-ref", "--verify", "refs/heads/feat/preview").Output(); err == nil {
		t.Fatal("prepare created branch")
	}

	// Launch with a fake tmux after preview; real git still exercises creation,
	// push and durable state while socket creation is unavailable in a sandbox.
	if out, err := exec.Command("git", "init", "--bare", filepath.Join(root, "remote.git")).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	git("push", "-u", "origin", "main")
	script := "#!/bin/sh\ncase \"$*\" in *list-sessions*) echo 'no server running on fixture' >&2; exit 1;; *list-keys*) echo 'unknown key'; exit 1;; *) exit 0;; esac\n"
	if err := os.WriteFile(filepath.Join(bin, "tmux"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	p.Prompt = "Reviewed and edited prompt"
	if err := os.WriteFile(filepath.Join(bp, "plan.md"), []byte("+++\n+++\nChanged after preview"), 0600); err != nil {
		t.Fatal(err)
	}
	created, err := SpawnPrepared(p, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if !created.Prompt || created.Blueprint != "plan" {
		t.Fatal(created)
	}
	path := filepath.Join(root, "state/motley/members", created.ID+".prompt.md")
	got, err := blueprint.ReadPrompt(path)
	if err != nil || got != p.Prompt {
		t.Fatal("reviewed prompt changed", got, err)
	}
	p, err = Prepare(cfg, SpawnOptions{Repo: "api", Branch: "feat/manual"})
	if err != nil {
		t.Fatal(err)
	}
	p.HasPrompt = true
	p.Prompt = "Manual {{.Literal}} prompt"
	created, err = SpawnPrepared(p, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if !created.Prompt || created.Blueprint != "" {
		t.Fatal("manual prompt metadata", created)
	}
}
