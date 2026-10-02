package report

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thomashartm/motley/internal/state"
)

// diagnosed waits for want in report.log. logError stops waiting after 10 ms so
// hooks stay fast, and the write may finish after Run returns.
func diagnosed(t *testing.T, want string) bool {
	t.Helper()
	path := filepath.Join(os.Getenv("XDG_STATE_HOME"), "motley/report.log")
	for deadline := time.Now().Add(2 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		if data, _ := os.ReadFile(path); strings.Contains(string(data), want) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
	}
}

func TestRunBoundsBlockedInputAndIgnoresUnmanagedAgent(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	reader, writer := io.Pipe()
	defer func() { _ = reader.Close() }()
	defer func() { _ = writer.Close() }()
	t.Setenv("MOTLEY_MEMBER", "")
	start := time.Now()
	Run([]string{"invalid"}, reader)
	if time.Since(start) > 50*time.Millisecond {
		t.Fatal("unmanaged hook read stdin")
	}
	t.Setenv("MOTLEY_MEMBER", "fixture")
	start = time.Now()
	Run([]string{"--agent", "claude"}, reader)
	if elapsed := time.Since(start); elapsed < timeout || elapsed > time.Second {
		t.Fatalf("input timeout: %v", elapsed)
	}
	if !diagnosed(t, "deadline exceeded") {
		t.Fatal("missing timeout diagnostic")
	}
}

func TestOtherAgentReports(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("MOTLEY_MEMBER", "fixture")
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	// Two invocations per event: status lookup then a batched update.
	log := filepath.Join(dir, "calls")
	t.Setenv("REPORT_TEST_CALLS", log)
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$REPORT_TEST_CALLS\"\nif [ \"$1\" = -u ]; then printf 'fixture\\topencode\\tquestion\\t{\"agent\":\"opencode\",\"agent_session_id\":\"session-1\"}\\n'; fi\n"
	if err := os.WriteFile(filepath.Join(dir, "tmux"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	// Warm the new shim before exercising the production 250 ms deadline;
	// macOS may inspect a newly created executable on its first invocation.
	if err := exec.Command(filepath.Join(dir, "tmux"), "-V").Run(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ agent, payload, status string }{
		{"codex", `{"hook_event_name":"PermissionRequest","session_id":"session-1","tool_name":"Bash","tool_input":{"command":"git push"}}`, "permission"},
		{"codex", `{"hook_event_name":"Stop","session_id":"session-1","last_assistant_message":"Done"}`, "ready"},
		{"opencode", `{"type":"permission.asked","properties":{"sessionID":"session-1","permission":"bash","patterns":["git push"]}}`, "permission"},
		{"opencode", `{"type":"session.idle","properties":{"sessionID":"session-1","lastAssistantMessage":"Done"}}`, "ready"},
	} {
		if err := os.WriteFile(log, nil, 0600); err != nil {
			t.Fatal(err)
		}
		Run([]string{"--agent", tc.agent}, strings.NewReader(tc.payload))
		calls, _ := os.ReadFile(log)
		if strings.Count(string(calls), "\n") != 2 || !strings.Contains(string(calls), "@motley_status "+tc.status) {
			t.Fatal(string(calls))
		}
		path := filepath.Join(os.Getenv("XDG_STATE_HOME"), "motley/members/fixture.events.jsonl")
		e, err := state.LatestEvent(path)
		if err != nil || e.Agent != tc.agent || e.Status != tc.status || e.AgentSessionID != "session-1" {
			t.Fatal(e, err)
		}
		bytes, err := json.Marshal(e)
		if err != nil || len(bytes) > state.MaxEventBytes {
			t.Fatal(err)
		}
	}
}
func TestRunBoundsTmuxAndLogsFlagErrors(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("MOTLEY_MEMBER", "fixture")
	dir := t.TempDir()
	// exec sleep ensures the child is killed without an orphan holding output pipes.
	if err := os.WriteFile(filepath.Join(dir, "tmux"), []byte("#!/bin/sh\nexec /bin/sleep 10\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	start := time.Now()
	Run([]string{"--agent", "claude"}, strings.NewReader(`{"hook_event_name":"Stop"}`))
	if time.Since(start) > time.Second {
		t.Fatal("tmux timeout not bounded")
	}
	Run([]string{"--unknown"}, strings.NewReader(""))
	if !diagnosed(t, "flag provided but not defined") {
		t.Fatal("missing flag diagnostic")
	}
}

func TestExitedEndsTheMemberOnce(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	shim := func(status string) {
		t.Helper()
		script := "#!/bin/sh\nif [ \"$1\" = -u ]; then printf 'fixture\\topencode\\t" + status + "\\t{\"agent_session_id\":\"session-1\"}\\n'; fi\n"
		if err := os.WriteFile(filepath.Join(dir, "tmux"), []byte(script), 0755); err != nil {
			t.Fatal(err)
		}
		if err := exec.Command(filepath.Join(dir, "tmux"), "-V").Run(); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(os.Getenv("XDG_STATE_HOME"), "motley/members/fixture.events.jsonl")
	for _, previous := range []string{"ready", "ended"} {
		shim(previous)
		Exited("fixture")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// The second exit follows an end already recorded, so it adds nothing.
	if n := strings.Count(string(data), "\n"); n != 1 {
		log, _ := os.ReadFile(filepath.Join(os.Getenv("XDG_STATE_HOME"), "motley/report.log"))
		t.Fatalf("events: %s\n%s", data, log)
	}
	e, err := state.LatestEvent(path)
	if err != nil || e.Agent != "opencode" || e.Event != "AgentExit" || e.Status != "ended" || e.AgentSessionID != "" {
		t.Fatal(e, err)
	}
	Exited("../escape")
	if !diagnosed(t, "invalid member id") {
		t.Fatal("invalid id not diagnosed")
	}
}
