package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Exercise the built executable, including config discovery and process exits.
func TestBinarySmoke(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "colony")
	build := exec.Command("go", "build", "-o", bin, "-ldflags=-X main.version=smoke", ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	// W0 deliberately has no environment override precedence.
	t.Setenv("COLONY_WORKTREES_ROOT", "/ignored-in-w0")
	t.Setenv("WT_WORKTREE_DIR", "/also-ignored-in-w0")

	run := func(args ...string) (string, error) {
		out, err := exec.Command(bin, args...).CombinedOutput()
		return string(out), err
	}
	out, err := run()
	if err != nil || !strings.Contains(out, "repos_root: "+filepath.Join(home, "projects")) ||
		!strings.Contains(out, "worktrees_root: "+filepath.Join(home, "worktrees")) {
		t.Fatalf("defaults: %v\n%s", err, out)
	}
	path := filepath.Join(home, "config", "colony", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("schema = 1\nrepos_root = '~/my repos'\nworktrees_root = '/tmp/my trees'\nfuture_setting = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err = run()
	want := "colony smoke\nrepos_root: " + filepath.Join(home, "my repos") + "\nworktrees_root: /tmp/my trees\n"
	if err != nil || out != want {
		t.Fatalf("config: %v\ngot %q\nwant %q", err, out, want)
	}
	out, err = run("--help")
	if err != nil || !strings.Contains(out, "version") || !strings.Contains(out, "config.toml") {
		t.Fatalf("help: %v\n%s", err, out)
	}
	if err := os.WriteFile(path, []byte("schema = ["), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err = run()
	if err == nil || !strings.Contains(out, "parse config") {
		t.Fatalf("bad config must fail: %v\n%s", err, out)
	}
	out, err = run("version")
	if err != nil || out != "colony smoke\n" {
		t.Fatalf("version must work with bad config: %v\n%s", err, out)
	}
	if out, err := run("unknown"); err == nil {
		t.Fatalf("unknown command must fail: %s", out)
	}
}
