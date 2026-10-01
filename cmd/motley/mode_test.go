package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/thomashartm/motley/internal/agents"
)

func TestSpawnPermissionMode(t *testing.T) {
	bin := buildLifecycleBinary(t)
	f := newMemberFixture(t, bin, "main")
	// The fake Claude records its exact argv and working directory.
	writeFixture(t, filepath.Join(f.home, "fake agents", "claude"), "#!/bin/sh\npwd -P > \"$HOME/cwd-$MOTLEY_MEMBER\"\nfor arg do printf '%s\\0' \"$arg\"; done > \"$HOME/argv-$MOTLEY_MEMBER\"\n", 0755)
	launched := func(id string) ([]string, string) {
		t.Helper()
		receipt := filepath.Join(f.home, "argv-"+id)
		eventually(t, func() bool { _, err := os.Stat(receipt); return err == nil })
		data, err := os.ReadFile(receipt)
		if err != nil {
			t.Fatal(err)
		}
		cwd, err := os.ReadFile(filepath.Join(f.home, "cwd-"+id))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(receipt); err != nil {
			t.Fatal(err)
		}
		var argv []string
		if len(data) > 0 {
			argv = strings.Split(strings.TrimSuffix(string(data), "\x00"), "\x00")
		}
		return argv, strings.TrimSpace(string(cwd))
	}
	sandbox, err := agents.ModeArgs("claude", "sandbox")
	if err != nil {
		t.Fatal(err)
	}
	f.motley("spawn", "--repo", "api", "--branch", "feat/sandboxed", "--mode", "sandbox", "--detach")
	id := "feat-sandboxed"
	m := f.manifest(id)
	if m.Mode != "sandbox" || !reflect.DeepEqual(m.AgentArgs, sandbox) {
		t.Fatalf("manifest: mode %q args %q", m.Mode, m.AgentArgs)
	}
	argv, cwd := launched(id)
	if !reflect.DeepEqual(argv, sandbox) {
		t.Fatalf("argv %q want %q", argv, sandbox)
	}
	worktree, err := filepath.EvalSymlinks(m.Worktree)
	if err != nil || cwd != worktree {
		t.Fatalf("cwd %q want %q (%v)", cwd, worktree, err)
	}
	if out := f.git(m.Worktree, "status", "--porcelain"); out != "" {
		t.Fatalf("mode left files in the worktree: %q", out)
	}
	// Revive reapplies the stored mode arguments.
	f.tmux("kill-session", "-t", "="+id)
	sessionID := "22222222-2222-4222-8222-222222222222"
	writeFixture(t, filepath.Join(f.state, "motley/members", id+".events.jsonl"), fmt.Sprintf("{\"schema\":1,\"agent\":\"claude\",\"agent_session_id\":%q}\n", sessionID), 0600)
	f.motley("revive", id)
	if argv, _ := launched(id); !reflect.DeepEqual(argv, append(append([]string(nil), sandbox...), "--resume="+sessionID)) {
		t.Fatalf("revive argv %q", argv)
	}
	// Without a mode Claude starts bare, keeping its own configured default.
	f.motley("spawn", "--repo", "api", "--branch", "feat/bare", "--detach")
	if m := f.manifest("feat-bare"); m.Mode != "" || m.AgentArgs != nil {
		t.Fatalf("bare manifest: %+v", m)
	}
	if argv, _ := launched("feat-bare"); argv != nil {
		t.Fatalf("bare argv %q", argv)
	}
	// A mode adds to blueprint arguments that do not decide permissions.
	dir := filepath.Join(f.home, "config/motley/blueprints")
	writeFixture(t, filepath.Join(dir, "opus.md"), "+++\nargs=['--model','opus']\n+++\nStart.", 0600)
	f.motley("spawn", "--repo", "api", "--branch", "feat/combined", "--blueprint", "opus", "--mode", "plan", "--detach")
	if argv, _ := launched("feat-combined"); !reflect.DeepEqual(argv, []string{"--model", "opus", "--permission-mode", "plan", "--", "Start."}) {
		t.Fatalf("combined argv %q", argv)
	}
}

func TestSpawnPermissionModeFailuresBeforeWorktree(t *testing.T) {
	bin := buildLifecycleBinary(t)
	f := newMemberFixture(t, bin, "main")
	dir := filepath.Join(f.home, "config/motley/blueprints")
	writeFixture(t, filepath.Join(dir, "plan-first.md"), "+++\nargs=['--permission-mode','plan']\n+++\nPlan.", 0600)
	writeFixture(t, filepath.Join(dir, "settings.md"), "+++\nargs=['--settings','{}']\n+++\nGo.", 0600)
	before := f.git(f.repo, "worktree", "list", "--porcelain")
	for _, tc := range []struct {
		extra []string
		want  string
	}{
		{[]string{"--mode", "bogus"}, `unknown claude mode "bogus"; choose manual, acceptEdits, plan, auto, dontAsk, bypassPermissions, sandbox`},
		{[]string{"--mode", "default"}, `unknown claude mode "default"`},
		{[]string{"--mode", "plan", "--agent", "codex"}, "modes are available for claude only, not codex"},
		{[]string{"--mode", "auto", "--blueprint", "plan-first"}, "blueprint plan-first already sets --permission-mode"},
		{[]string{"--mode", "sandbox", "--blueprint", "settings"}, "blueprint settings already sets --settings"},
	} {
		args := append([]string{"spawn", "--repo", "api", "--branch", "feat/invalid", "--detach"}, tc.extra...)
		if out := f.refused(args...); !strings.Contains(out, tc.want) {
			t.Fatalf("%v: %s", tc.extra, out)
		}
	}
	if got := f.git(f.repo, "worktree", "list", "--porcelain"); got != before {
		t.Fatal("invalid mode created a worktree")
	}
	if _, err := exec.Command("git", "-C", f.repo, "show-ref", "--verify", "refs/heads/feat/invalid").Output(); err == nil {
		t.Fatal("invalid mode created a branch")
	}
	if _, err := os.Stat(filepath.Join(f.state, "motley/members", "feat-invalid.toml")); !os.IsNotExist(err) {
		t.Fatal("invalid mode wrote a manifest", err)
	}
}
