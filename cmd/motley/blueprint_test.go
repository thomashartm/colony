package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/thomashartm/motley/internal/blueprint"
)

func TestBlueprintSpawnAndResume(t *testing.T) {
	bin := buildLifecycleBinary(t)
	f := newMemberFixture(t, bin, "main")
	f.motley("crew", "add", "--title", "FX Banking", "--url", "https://example.com/work")
	for _, agent := range []string{"claude", "codex", "opencode"} {
		writeFixture(t, filepath.Join(f.home, "fake agents", agent), "#!/bin/sh\nfor arg do printf '%s\\0' \"$arg\"; done > \"$HOME/prompt-$MOTLEY_MEMBER.args\"\n", 0755)
	}
	global := filepath.Join(f.home, "config/motley/blueprints")
	local := filepath.Join(f.repo, ".motley/blueprints")
	writeFixture(t, filepath.Join(global, "shared.md"), "+++\nname='shared'\n+++\nGlobal\n", 0600)
	writeFixture(t, filepath.Join(local, "shared.md"), "+++\nname='shared'\nrepos=['api']\n+++\nRepository\n", 0600)
	if got := f.motley("blueprint", "show", "shared", "--repo", "api"); !strings.Contains(got, "Repository") {
		t.Fatal(got)
	}
	if got := f.motley("blueprint", "show", "shared"); !strings.Contains(got, "Global") {
		t.Fatal(got)
	}
	f.motley("blueprint", "validate", "--repo", "api")
	read := func(id string) []string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(f.home, "prompt-"+id+".args"))
		if err != nil {
			t.Fatal(err)
		}
		if len(data) == 0 {
			return nil
		}
		return strings.Split(strings.TrimSuffix(string(data), "\x00"), "\x00")
	}
	marker := filepath.Join(f.home, "shell-must-not-run")
	constraint := "line one, x=y\n$(touch " + marker + ") `pwd` \"quoted\""
	for _, agent := range []string{"claude", "codex", "opencode"} {
		name := "plan-" + agent
		branch := "feat/" + name
		id := "feat-" + name
		src := fmt.Sprintf("+++\nschema=1\nname=%q\nagent=%q\nargs=['--model','fixture-model']\nvars=['constraints']\n+++\n--literal prompt\n{{.Repo}} | {{.Name}} | {{.Branch}} | {{.Base}}\n{{.Crew.Title}} {{.Crew.URL}} {{.Crew.Kind}}\n{{.Worktree}}\n{{.Vars.constraints}}\n[{{.Vars.missing}}]\n", name, agent)
		writeFixture(t, filepath.Join(global, name+".md"), src, 0600)
		f.motley("spawn", "--repo", "api", "--branch", branch, "--blueprint", name, "--var", "constraints="+constraint, "--crew", "fx-banking", "--detach")
		m := f.manifest(id)
		want := fmt.Sprintf("--literal prompt\napi | %s | %s | main\nFX Banking https://example.com/work link\n%s\n%s\n[]\n", name, branch, m.Worktree, constraint)
		eventually(t, func() bool { _, err := os.Stat(filepath.Join(f.home, "prompt-"+id+".args")); return err == nil })
		expected := []string{"--model", "fixture-model", "--", want}
		if agent == "opencode" {
			expected = []string{"--model", "fixture-model", "--prompt=" + want}
		}
		if got := read(id); !reflect.DeepEqual(got, expected) {
			t.Fatalf("%s argv: %q want %q", agent, got, expected)
		}
		if m.Blueprint != name || m.Agent != agent || !reflect.DeepEqual(m.AgentArgs, expected[:2]) {
			t.Fatalf("manifest: %+v", m)
		}
		stored, err := os.ReadFile(filepath.Join(f.state, "motley/members", id+".prompt.md"))
		if err != nil || string(stored) != blueprint.PromptHeader+want {
			t.Fatalf("stored prompt %q %v", stored, err)
		}
		if _, err := os.Stat(marker); !os.IsNotExist(err) {
			t.Fatal("prompt was executed by shell", err)
		}
		// Revive reads stored arguments, ignores changed/deleted source and never replays.
		f.tmux("kill-session", "-t", "="+id)
		if err := os.Remove(filepath.Join(global, name+".md")); err != nil {
			t.Fatal(err)
		}
		receipt := filepath.Join(f.home, "prompt-"+id+".args")
		if err := os.Remove(receipt); err != nil {
			t.Fatal(err)
		}
		sessionID := "11111111-1111-4111-8111-111111111111"
		writeFixture(t, filepath.Join(f.state, "motley/members", id+".events.jsonl"), fmt.Sprintf("{\"schema\":1,\"agent\":%q,\"agent_session_id\":%q}\n", agent, sessionID), 0600)
		f.motley("revive", id)
		eventually(t, func() bool { _, err := os.Stat(receipt); return err == nil })
		resume := []string{"--model", "fixture-model"}
		if agent == "claude" {
			resume = append(resume, "--resume="+sessionID)
		}
		if got := read(id); !reflect.DeepEqual(got, resume) {
			t.Fatalf("resume %s: %q want %q", agent, got, resume)
		}
		f.motley("retire", id, "--force")
		archived, err := os.ReadFile(filepath.Join(f.state, "motley/members/archive", id+".prompt.md"))
		if err != nil || !bytes.Equal(stored, archived) {
			t.Fatal("prompt not archived", err)
		}
	}
	// An explicit agent overrides a blueprint with no agent-specific arguments.
	f.motley("spawn", "--repo", "api", "--branch", "feat/override", "--blueprint", "shared", "--agent", "codex", "--detach")
	if m := f.manifest("feat-override"); m.Agent != "codex" {
		t.Fatal(m.Agent)
	}
	eventually(t, func() bool { _, err := os.Stat(filepath.Join(f.home, "prompt-feat-override.args")); return err == nil })
	if got := read("feat-override"); !reflect.DeepEqual(got, []string{"--", "Repository\n"}) {
		t.Fatal(got)
	}
}

func TestBlueprintFailuresBeforeWorktree(t *testing.T) {
	bin := buildLifecycleBinary(t)
	f := newMemberFixture(t, bin, "main")
	dir := filepath.Join(f.home, "config/motley/blueprints")
	writeFixture(t, filepath.Join(dir, "restricted.md"), "+++\nrepos=['other']\n+++\nhi", 0600)
	writeFixture(t, filepath.Join(dir, "claude-plan.md"), "+++\nargs=['--permission-mode','plan']\n+++\nhi", 0600)
	writeFixture(t, filepath.Join(dir, "bad-field.md"), "+++\n+++\n{{.Typo}}", 0600)
	before := f.git(f.repo, "worktree", "list", "--porcelain")
	for _, extra := range [][]string{{"--blueprint", "missing"}, {"--blueprint", "restricted"}, {"--blueprint", "claude-plan", "--agent", "codex"}, {"--blueprint", "claude-plan", "--var", "broken"}, {"--blueprint", "bad-field"}, {"--var", "x=y"}} {
		args := append([]string{"spawn", "--repo", "api", "--branch", "feat/invalid", "--detach"}, extra...)
		f.refused(args...)
	}
	if got := f.git(f.repo, "worktree", "list", "--porcelain"); got != before {
		t.Fatal("invalid blueprint created worktree")
	}
	if _, err := exec.Command("git", "-C", f.repo, "show-ref", "--verify", "refs/heads/feat/invalid").Output(); err == nil {
		t.Fatal("invalid blueprint created branch")
	}
	if out := f.refused("blueprint", "validate", "bad-field"); !strings.Contains(out, "Typo") {
		t.Fatal(out)
	}
	if out := f.motley("blueprint", "list", "--repo", "api"); strings.Contains(out, "restricted") {
		t.Fatal(out)
	}
}
