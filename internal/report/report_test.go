package report

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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
	data, err := os.ReadFile(filepath.Join(os.Getenv("XDG_STATE_HOME"), "motley/report.log"))
	if err != nil || !strings.Contains(string(data), "deadline exceeded") {
		t.Fatal("missing timeout diagnostic", err)
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
	data, err := os.ReadFile(filepath.Join(os.Getenv("XDG_STATE_HOME"), "motley/report.log"))
	if err != nil || !strings.Contains(string(data), "flag provided but not defined") {
		t.Fatal("missing flag diagnostic", err)
	}
}
