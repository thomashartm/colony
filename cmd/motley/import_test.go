package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/thomashartm/motley/internal/agents/claude"
	"github.com/thomashartm/motley/internal/member"
)

// A harmless process stands in for a running Claude. All discovery/control stays
// in the disposable fixture; no real user sessions are imported or signalled.
func fakeExternalClaude(t *testing.T, f *memberFixture, cwd string) (string, *exec.Cmd) {
	t.Helper()
	process := exec.Command("sleep", "120")
	if err := process.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { _ = process.Wait(); close(done) }()
	t.Cleanup(func() { _ = process.Process.Kill(); <-done })
	sid := "11111111-1111-4111-8111-111111111111"
	data, err := json.Marshal([]claude.Session{{SessionID: sid, PID: process.Process.Pid, Cwd: cwd, Kind: "interactive", Name: "Existing Claude", Status: "busy", StartedAt: 12345}})
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, filepath.Join(f.home, "sessions.json"), string(data), 0600)
	script := fmt.Sprintf(`#!/bin/sh
if [ "$1" = agents ]; then
 if [ -f "$HOME/discovery-error" ]; then exit 1; fi
 if kill -0 %d 2>/dev/null; then cat "$HOME/sessions.json"; else echo '[]'; fi
else
 printf '%%s\n' "$@" > "$HOME/resumed-args"
fi
`, process.Process.Pid)
	writeFixture(t, filepath.Join(f.home, "fake agents", "claude"), script, 0755)
	return sid, process
}

func TestImportClaudeLifecycle(t *testing.T) {
	bin := buildLifecycleBinary(t)
	for _, kind := range []string{"main", "linked", "non-git"} {
		t.Run(kind, func(t *testing.T) {
			f := newMemberFixture(t, bin, "main")
			cwd := f.repo
			if kind == "linked" {
				cwd = filepath.Join(f.trees, "existing")
				f.git(f.repo, "worktree", "add", "-b", "existing", cwd)
			}
			if kind == "non-git" {
				cwd = filepath.Join(f.home, "notes")
				if err := os.MkdirAll(cwd, 0700); err != nil {
					t.Fatal(err)
				}
			}
			sid, process := fakeExternalClaude(t, f, cwd)
			writeFixture(t, filepath.Join(cwd, "unfinished.txt"), "keep this work", 0600)
			beforeTrees := f.git(f.repo, "worktree", "list", "--porcelain")
			beforeBranches := f.git(f.repo, "branch", "--list")
			if out := f.motley("import", "--list"); !strings.Contains(out, sid) {
				t.Fatal(out)
			}
			if out := f.motley("import", sid); !strings.Contains(out, "keeps running") {
				t.Fatal(out)
			}
			id := "claude-" + sid
			if err := process.Process.Signal(syscall.Signal(0)); err != nil {
				t.Fatal("import interrupted Claude", err)
			}
			if out := f.motley("import", "--list"); strings.Contains(out, sid) {
				t.Fatal("duplicate offered")
			}
			f.refused("import", sid)
			assertListState(t, f.motley("ls"), id, "alive")
			rows, err := member.List()
			if err != nil || len(rows) != 1 || !rows[0].External || rows[0].CurrentStatus() != "working" {
				t.Fatal(rows, err)
			}
			if out := f.refused("revive", id); !strings.Contains(out, "still running") {
				t.Fatal(out)
			}
			if err := member.Terminate(id); err != nil {
				t.Fatal(err)
			}
			assertListState(t, f.motley("ls"), id, "dead")
			f.motley("revive", id)
			eventually(t, func() bool {
				data, _ := os.ReadFile(filepath.Join(f.home, "resumed-args"))
				return string(data) == "--resume="+sid+"\n"
			})
			assertListState(t, f.motley("ls"), id, "alive")
			f.motley("retire", id, "--force")
			data, err := os.ReadFile(filepath.Join(cwd, "unfinished.txt"))
			if err != nil || string(data) != "keep this work" {
				t.Fatal("imported work deleted", err)
			}
			if beforeTrees != f.git(f.repo, "worktree", "list", "--porcelain") || beforeBranches != f.git(f.repo, "branch", "--list") {
				t.Fatal("imported Git checkout changed")
			}
		})
	}
}

func TestImportRetireRunningAndDiscoveryFailure(t *testing.T) {
	f := newMemberFixture(t, buildLifecycleBinary(t), "main")
	sid, _ := fakeExternalClaude(t, f, f.repo)
	f.motley("import", sid)
	id := "claude-" + sid
	writeFixture(t, filepath.Join(f.home, "discovery-error"), "fail", 0600)
	if err := member.Terminate(id); err == nil {
		t.Fatal("discovery failure authorized stopping")
	}
	if err := member.Retire(id, true, false); err == nil {
		t.Fatal("discovery failure authorized retirement")
	}
	if _, err := member.List(); err == nil {
		t.Fatal("discovery failure silently marked dead")
	}
	if err := os.Remove(filepath.Join(f.home, "discovery-error")); err != nil {
		t.Fatal(err)
	}
	assertListState(t, f.motley("ls"), id, "alive")
	f.motley("retire", id)
	if _, err := os.Stat(f.repo); err != nil {
		t.Fatal("retired main checkout", err)
	}
}

func TestImportPickerTerminal(t *testing.T) {
	bin := buildLifecycleBinary(t)
	f := newMemberFixture(t, bin, "main")
	sid, _ := fakeExternalClaude(t, f, f.repo)
	terminal := startTerminal(t, exec.Command(bin, "--monitor"))
	eventually(t, func() bool { return strings.Contains(terminal.text(), "[3 Actions]") })
	terminal.send(t, "a")
	eventually(t, func() bool { return strings.Contains(terminal.text(), "Existing Claude") })
	terminal.send(t, "\r")
	eventually(t, func() bool { return strings.Contains(terminal.text(), "Added Existing Claude") })
	rows, err := member.List()
	if err != nil || len(rows) != 1 || rows[0].ClaudeSession != sid || !rows[0].Alive || !rows[0].External {
		t.Fatal("picker did not register running external session", rows, err)
	}
}
