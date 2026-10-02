package main

import (
	"encoding/base64"
	"path/filepath"
	"strings"
	"testing"
)

// The overview runs in a real tmux pane; a pty client stands in for Ghostty.
// tmux must forward the copy to that terminal as OSC 52.
func TestCopyMessageReachesTerminalClipboard(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "motley")
	commandOutput(t, "go", "build", "-o", bin, ".")
	f := newMemberFixture(t, bin, "main")
	f.tmux("set-option", "-s", "set-clipboard", "external")
	terminal := f.terminalClient("fixture")
	f.tmux("new-window", "-t", "=fixture:", bin)
	eventually(t, func() bool { return strings.Contains(terminal.text(), "No members yet") })
	terminal.send(t, "c")
	eventually(t, func() bool { return strings.Contains(terminal.text(), "No message to copy. [c copy]") })
	terminal.send(t, "c")
	sequence := "\x1b]52;"
	payload := base64.StdEncoding.EncodeToString([]byte("No message to copy."))
	eventually(t, func() bool {
		out := terminal.text()
		at := strings.LastIndex(out, sequence)
		return at >= 0 && strings.Contains(out[at:], payload+"\a") && strings.Contains(out, "✓ copied")
	})
	if got := f.tmux("show-buffer"); got != "No message to copy." {
		t.Fatalf("tmux buffer %q", got)
	}
	// Without forwarding, the copy must not claim the clipboard.
	f.tmux("set-option", "-s", "set-clipboard", "off")
	terminal.send(t, "c")
	eventually(t, func() bool {
		return strings.Contains(terminal.text(), "Copy failed: saved as a tmux paste buffer only; tmux set-clipboard is off")
	})
}
