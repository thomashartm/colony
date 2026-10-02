package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/thomashartm/motley/internal/member"
)

func TestTmuxMouseTicketLinksAndPrefix(t *testing.T) {
	bin := buildLifecycleBinary(t)
	f := newMemberFixture(t, bin, "main")
	log := filepath.Join(f.home, "opened-links")
	for _, name := range []string{"open", "xdg-open"} {
		writeFixture(t, filepath.Join(f.home, "fake agents", name), "#!/bin/sh\nprintf '%s\\n' \"$1\" >> "+quoteShell(log)+"\n", 0755)
	}
	f.motley("spawn", "--repo", "api", "--branch", "feat/links", "--name", "Ticket fixture", "--detach")
	target := "https://example.com/issues/42"
	if err := member.EditIdentity("feat-links", member.IdentityEdit{Ticket: &target}); err != nil {
		t.Fatal(err)
	}
	f.motley("init")
	configPath := filepath.Join(f.home, "config/motley/motley.tmux.conf")
	f.tmux("source-file", configPath)
	f.tmux("source-file", configPath)
	for option, want := range map[string]string{"prefix": "C-a", "mouse": "on"} {
		if got := f.tmux("show-options", "-gqv", option); got != want {
			t.Fatalf("%s=%s", option, got)
		}
	}
	if !strings.Contains(f.tmux("list-keys", "-T", "prefix"), "C-a") {
		t.Fatal("prefix passthrough missing")
	}
	f.tmux("set-environment", "-g", "PATH", filepath.Dir(bin)+string(os.PathListSeparator)+os.Getenv("PATH"))
	terminal := f.terminalClient("fixture")
	f.tmux("new-window", "-t", "=fixture:", bin)
	defer func() {
		if t.Failed() {
			out := ansi.Strip(terminal.text())
			if len(out) > 7000 {
				out = out[len(out)-7000:]
			}
			t.Log(out)
			b, _ := os.ReadFile(log)
			t.Logf("opened=%q", b)
		}
	}()
	view := func() string { return f.tmux("capture-pane", "-p", "-t", "=fixture:") }
	eventually(t, func() bool { return strings.Contains(view(), "#42") })
	x, y := -1, -1
	for row, line := range strings.Split(view(), "\n") {
		if at := strings.Index(line, "#42"); at >= 0 {
			x = ansi.StringWidth(line[:at])
			y = row
			break
		}
	}
	if x < 0 {
		t.Fatal("ticket not visible")
	}
	clickTicket := func() { terminal.send(t, fmt.Sprintf("\x1b[<0;%d;%dM\x1b[<0;%d;%dm", x+1, y+1, x+1, y+1)) }
	opened := func(count int) bool {
		b, err := os.ReadFile(log)
		return err == nil && string(b) == strings.Repeat(target+"\n", count)
	}
	clickTicket()
	eventually(t, func() bool { return opened(1) })
	// Non-link clicks still reach the UI through tmux.
	terminal.send(t, "\x1b[<0;4;3M\x1b[<0;4;3m")
	eventually(t, func() bool { return strings.Contains(view(), "Overview actions") })
	terminal.send(t, "q")
	eventually(t, func() bool { return !strings.Contains(view(), "Overview actions") })
	// A full-size borderless popup gives the same cell coordinates as the pane.
	// Use a fixture binding; the generated Ctrl-a h popup is exercised separately.
	f.tmux("bind-key", "l", "display-popup", "-B", "-x", "0", "-y", "0", "-w", "100%", "-h", "100%", "-e", "PATH="+os.Getenv("PATH"), "-E", quoteShell(bin)+" --client "+quoteShell(f.clientName(terminal)))
	offset := len(terminal.text())
	terminal.send(t, "\x01l")
	eventually(t, func() bool { return strings.Contains(ansi.Strip(terminal.text()[offset:]), "#42") })
	clickTicket()
	eventually(t, func() bool { return opened(2) })
	f.tmux("display-popup", "-C", "-c", f.clientName(terminal))
	// Ctrl-a d still detaches, leaving the session and member running.
	terminal.send(t, "\x01d")
	eventually(t, func() bool { return f.clientSession(f.clientName(terminal)) == "" })
}
