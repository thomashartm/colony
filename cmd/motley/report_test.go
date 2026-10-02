package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/thomashartm/motley/internal/state"
)

func (f *memberFixture) report(id, payload string, args ...string) {
	f.t.Helper()
	if len(args) == 0 {
		args = []string{"--agent", "claude"}
	}
	cmd := exec.Command(f.bin, append([]string{"report"}, args...)...)
	cmd.Env = append(os.Environ(), "MOTLEY_MEMBER="+id)
	cmd.Stdin = strings.NewReader(payload)
	out, err := cmd.CombinedOutput()
	if err != nil || len(out) != 0 {
		f.t.Fatalf("hook must be silent and exit 0: %q %v", out, err)
	}
}
func TestClaudeReportingEndToEnd(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "motley")
	commandOutput(t, "go", "build", "-o", bin, ".")
	f := newMemberFixture(t, bin, "main")
	f.keepAgentRunning("claude")
	f.motley("spawn", "--repo", "api", "--branch", "feat/hooks", "--detach")
	id := "feat-hooks"
	dir := filepath.Join(f.state, "motley/members")
	manifestPath := filepath.Join(dir, id+".toml")
	before, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	beforeInfo, err := os.Stat(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, id+".events.jsonl")
	count := func() int { data, _ := os.ReadFile(logPath); return bytes.Count(data, []byte{'\n'}) }
	status := func() string { return f.tmux("show-options", "-v", "-t", "="+id+":", "@motley_status") }
	event := func(payload, want string, n int) {
		t.Helper()
		f.report(id, payload)
		if got := status(); got != want {
			diagnostic, _ := os.ReadFile(filepath.Join(f.state, "motley/report.log"))
			t.Fatalf("status=%s want %s after %s\n%s", got, want, payload, diagnostic)
		}
		if got := count(); got != n {
			t.Fatalf("events=%d want %d after %s", got, n, payload)
		}
	}
	if status() != "starting" {
		t.Fatal("spawn must start in starting state")
	}
	event(`{"hook_event_name":"SessionStart","session_id":"session-1"}`, "idle", 1)
	event(`{"hook_event_name":"UserPromptSubmit","session_id":"session-1","prompt":"build fixture"}`, "working", 2)
	f.tmux("set-option", "-t", "="+id+":", "@motley_since", "1")
	f.tmux("set-option", "-t", "="+id+":", "@motley_seen", "1")
	event(`{"hook_event_name":"PreToolUse","session_id":"session-1","tool_name":"Bash","tool_input":{"command":"pnpm test"}}`, "working", 2)
	options := f.tmux("display-message", "-p", "-t", "="+id+":", "#{@motley_since}|#{@motley_seen}")
	if !strings.HasPrefix(options, "1|") || options == "1|1" {
		t.Fatalf("seen/since semantics: %s", options)
	}
	event(`{"hook_event_name":"Notification","session_id":"session-1","notification_type":"permission_prompt","message":"Claude needs your permission"}`, "permission", 3)
	last, err := state.LatestEvent(logPath)
	if err != nil || last.Detail["input"] != "pnpm test" {
		t.Fatalf("lost permission command: %+v %v", last, err)
	}
	event(`{"hook_event_name":"PostToolUse","session_id":"session-1","tool_name":"Bash","tool_input":{"command":"pnpm test"}}`, "working", 4)
	question := `{"hook_event_name":"PreToolUse","session_id":"session-1","tool_name":"AskUserQuestion","tool_input":{"questions":[{"question":"Which fixture option?","options":[{"label":"Alpha","description":"First"},{"label":"Beta","description":"Second"}]}]}}`
	event(question, "question", 5)
	event(`{"hook_event_name":"Notification","session_id":"session-1","notification_type":"permission_prompt","message":"Claude needs your permission"}`, "question", 6)
	last, err = state.LatestEvent(logPath)
	if err != nil || !strings.Contains(last.Detail["question"], "Beta") {
		t.Fatal("question overwritten by generic permission notification", err)
	}
	// Selected detail must refresh without navigation, in a real monitor terminal.
	configPath := filepath.Join(f.home, "config/motley/config.toml")
	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, configPath, string(configData)+"monitor_bell = true\n", 0600)
	cmd := exec.Command(bin, "--monitor")
	terminal := startTerminal(t, cmd)
	eventually(t, func() bool { return strings.Contains(terminal.text(), "Which fixture option?") })
	event(`{"hook_event_name":"PostToolUse","session_id":"session-1","tool_name":"AskUserQuestion","tool_input":{}}`, "working", 7)
	eventually(t, func() bool { return strings.Contains(terminal.text(), "WORKING") })
	bells := strings.Count(terminal.text(), "\a")
	tree := filepath.Join(f.trees, "api/feat-hooks")
	writeFixture(t, filepath.Join(tree, "change.txt"), "new work\n", 0600)
	f.git(tree, "add", "change.txt")
	f.git(tree, "commit", "-m", "Agent work")
	event(`{"hook_event_name":"Stop","session_id":"session-1","last_assistant_message":"Fixture implementation finished"}`, "ready", 8)
	eventually(t, func() bool {
		out := terminal.text()
		return strings.Contains(out, "Fixture implementation finished") && strings.Contains(out, "NEW ATTENTION") && strings.Count(out, "\a") > bells
	})
	// The footer reserves two rows. Long macOS fixture paths can put the diff
	// below the viewport; scroll to inspect it rather than requiring it above the fold.
	terminal.send(t, "\x1b[6~") // Page Down
	eventually(t, func() bool { return strings.Contains(terminal.text(), "change.txt") })
	terminal.send(t, "\x1b[5~") // Page Up
	event(`{"hook_event_name":"Stop","session_id":"session-1","last_assistant_message":"Updated final response"}`, "ready", 9)
	eventually(t, func() bool { return strings.Contains(terminal.text(), "Updated final response") })
	if !strings.Contains(f.motley("ls"), "ready") {
		t.Fatal("ls did not expose status")
	}
	if got := f.tmux("display-message", "-p", "-t", "="+id+":", "#{E:status-left}"); !strings.Contains(got, "ready") {
		t.Fatalf("tmux status-left: %q", got)
	}
	event(`{"hook_event_name":"Notification","session_id":"session-1","notification_type":"idle_prompt","message":"Waiting for input"}`, "idle", 10)
	event(`{"hook_event_name":"SessionEnd","session_id":"session-1"}`, "ended", 11)
	f.report(id, "not JSON")
	if status() != "ended" || count() != 11 {
		t.Fatal("bad JSON changed status/history")
	}
	f.report(id, "{}", "--bad-flag")
	f.report("", "not JSON", "--bad-flag")
	f.report("missing-session", `{ "hook_event_name":"Stop" }`)
	if _, err := os.Stat(filepath.Join(f.state, "motley/report.log")); err != nil {
		t.Fatal("errors not logged", err)
	}
	after, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	afterInfo, err := os.Stat(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) || !beforeInfo.ModTime().Equal(afterInfo.ModTime()) {
		t.Fatal("hook modified manifest")
	}
	// Count real tmux invocations through a transparent wrapper.
	wrapper := filepath.Join(f.home, "count-tools")
	calls := filepath.Join(f.home, "tmux-calls")
	writeFixture(t, filepath.Join(wrapper, "tmux"), "#!/bin/sh\necho call >> "+quoteShell(calls)+"\nexec "+quoteShell(f.tmuxBin)+" \"$@\"\n", 0755)
	t.Setenv("PATH", wrapper+":"+os.Getenv("PATH"))
	commandOutput(t, filepath.Join(wrapper, "tmux"), "-V")
	if err := os.Remove(calls); err != nil {
		t.Fatal(err)
	}
	f.report(id, `{"hook_event_name":"SessionStart","session_id":"session-2"}`)
	data, err := os.ReadFile(calls)
	if err != nil || bytes.Count(data, []byte{'\n'}) != 2 {
		diagnostic, _ := os.ReadFile(filepath.Join(f.state, "motley/report.log"))
		t.Fatalf("report needs exactly 2 tmux invocations: %q %v\n%s", data, err, diagnostic)
	}
}

// BenchmarkReportCLI measures process startup, JSON parsing, two real tmux
// invocations and event append. It never contacts the user's tmux server.
func BenchmarkReportCLI(b *testing.B) {
	bin := filepath.Join(b.TempDir(), "motley")
	build := exec.Command("go", "build", "-trimpath", "-o", bin, ".")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		b.Fatalf("build: %v %s", err, out)
	}
	b.Setenv("XDG_STATE_HOME", b.TempDir())
	socket := fmt.Sprintf("motley-test-bench-%d", time.Now().UnixNano())
	tmux := func(args ...string) string {
		b.Helper()
		out, err := exec.Command("tmux", append([]string{"-L", socket}, args...)...).CombinedOutput()
		if err != nil {
			b.Fatalf("tmux: %v %s", err, out)
		}
		return strings.TrimSpace(string(out))
	}
	tmux("-f", "/dev/null", "new-session", "-d", "-s", "bench", "/bin/sh")
	b.Cleanup(func() { _ = exec.Command("tmux", "-L", socket, "kill-server").Run() })
	tmux("set-option", "-t", "bench", "@motley_member", "bench")
	b.Setenv("TMUX", tmux("display-message", "-p", "-t", "bench", "#{socket_path},#{pid},0"))
	b.Setenv("MOTLEY_MEMBER", "bench")
	times := make([]float64, b.N)
	payload := `{"hook_event_name":"Stop","session_id":"benchmark","last_assistant_message":"Finished benchmark turn"}`
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cmd := exec.Command(bin, "report", "--agent", "claude")
		cmd.Stdin = strings.NewReader(payload)
		start := time.Now()
		out, err := cmd.CombinedOutput()
		times[i] = float64(time.Since(start)) / float64(time.Millisecond)
		if err != nil || len(out) != 0 {
			b.Fatalf("report: %v %s", err, out)
		}
	}
	b.StopTimer()
	sort.Float64s(times)
	b.ReportMetric(times[(len(times)-1)*95/100], "p95-ms")
	log, err := os.ReadFile(filepath.Join(os.Getenv("XDG_STATE_HOME"), "motley/members/bench.events.jsonl"))
	if err != nil || bytes.Count(log, []byte{'\n'}) != b.N {
		b.Fatalf("benchmark lost reports: %v", err)
	}
	var last state.Event
	lines := bytes.Split(bytes.TrimSpace(log), []byte{'\n'})
	if err := json.Unmarshal(lines[len(lines)-1], &last); err != nil || last.Status != "ready" {
		b.Fatal("invalid benchmark event", err)
	}
}
