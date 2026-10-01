package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/thomashartm/motley/internal/member"
	"github.com/thomashartm/motley/internal/state"
)

func TestExecNativeAgent(t *testing.T) {
	bin := buildLifecycleBinary(t)
	root := t.TempDir()
	t.Setenv("XDG_STATE_HOME", root)
	dir := filepath.Join(root, "motley/members")
	tools := t.TempDir()
	t.Setenv("PATH", tools)
	for _, agent := range []string{"codex", "opencode"} {
		m := member.Manifest{Schema: 1, ID: "test-" + agent, Agent: agent, AgentArgs: []string{"--model", "fixture"}, Worktree: t.TempDir(), Prompt: true}
		data, err := toml.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		writeFixture(t, filepath.Join(dir, m.ID+".toml"), string(data), 0600)
		writeFixture(t, filepath.Join(dir, m.ID+".prompt.md"), "<!-- schema = 1 -->\nInitial $(literal)\n", 0600)
		writeFixture(t, filepath.Join(tools, agent), "#!/bin/sh\nprintf '%s\\0' \"$MOTLEY_MEMBER\" \"$PWD\" \"$@\"\n", 0755)
		physical, err := filepath.EvalSymlinks(m.Worktree)
		if err != nil {
			t.Fatal(err)
		}
		check := func(resume bool, want []string) {
			t.Helper()
			args := []string{"exec-agent", m.ID}
			if resume {
				args = append(args, "--resume")
			}
			out, err := exec.Command(bin, args...).CombinedOutput()
			if err != nil {
				t.Fatalf("%s: %s %v", agent, out, err)
			}
			got := strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")
			want = append([]string{m.ID, physical}, want...)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("%s: %q want %q", agent, got, want)
			}
		}
		start := []string{"--model", "fixture", "--prompt=Initial $(literal)\n"}
		fresh := []string{"--model", "fixture"}
		resume := append(append([]string{}, fresh...), "--session=session-1")
		if agent == "codex" {
			start = []string{"--model", "fixture", "--no-daemon", "--", "Initial $(literal)\n"}
			fresh = append(fresh, "--no-daemon")
			resume = []string{"resume", "--model", "fixture", "--no-daemon", "--", "session-1"}
		}
		check(false, start)
		check(true, fresh)
		writeFixture(t, filepath.Join(dir, m.ID+".events.jsonl"), fmt.Sprintf("{\"schema\":1,\"agent\":%q,\"agent_session_id\":\"session-1\"}\n", agent), 0600)
		check(true, resume)
	}
}

func TestNativeAgentReporting(t *testing.T) {
	bin := buildLifecycleBinary(t)
	for _, agent := range []string{"codex", "opencode"} {
		t.Run(agent, func(t *testing.T) {
			f := newMemberFixture(t, bin, "main")
			id := "feat-" + agent
			f.motley("spawn", "--repo", "api", "--branch", "feat/"+agent, "--agent", agent, "--detach")
			dir := filepath.Join(f.state, "motley/members")
			manifest := filepath.Join(dir, id+".toml")
			before, err := os.ReadFile(manifest)
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join("../../internal/agents", agent, "testdata/events.json"))
			if err != nil {
				t.Fatal(err)
			}
			var fixtures struct {
				Cases []struct {
					Payload json.RawMessage
					Status  string
				}
			}
			if err := json.Unmarshal(data, &fixtures); err != nil {
				t.Fatal(err)
			}
			for _, tc := range fixtures.Cases {
				f.report(id, string(tc.Payload), "--agent", agent)
				if tc.Status == "" {
					continue
				}
				if got := f.tmux("show-options", "-v", "-t", "="+id+":", "@motley_status"); got != tc.Status {
					t.Fatal(got, tc.Status)
				}
			}
			f.report(id, "invalid json", "--agent", agent)
			after, _ := os.ReadFile(manifest)
			if !bytes.Equal(before, after) {
				t.Fatal("hook rewrote manifest")
			}
			sid, err := state.LatestSessionID(filepath.Join(dir, id+".events.jsonl"), agent)
			if err != nil || sid != "session-1" {
				t.Fatal(sid, err)
			}
		})
	}
}
