package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/thomashartm/motley/internal/member"
)

func TestReplyAndSendIntegration(t *testing.T) {
	bin := buildLifecycleBinary(t)
	f := newMemberFixture(t, bin, "main")
	// Keep a fake agent reading its terminal, so replies cannot fall into a shell.
	writeFixture(t, filepath.Join(f.home, "fake agents/claude"), "#!/bin/sh\nwhile IFS= read -r line; do printf '%s\\n' \"$line\" >> \"$HOME/replies\"; done\n", 0755)
	f.motley("spawn", "--repo", "api", "--branch", "feat/reply", "--detach")
	id := "feat-reply"
	for _, text := range []string{"hello 'quoted' $(touch never)", ";", `literal\;`} {
		if err := member.Reply(id, text); err != nil {
			t.Fatal(err)
		}
		eventually(t, func() bool {
			data, _ := os.ReadFile(filepath.Join(f.home, "replies"))
			return strings.Contains(string(data), text+"\n")
		})
	}
	f.tmux("set-option", "-t", "="+id+":", "@motley_status", "permission")
	if err := member.Reply(id, "do not send"); err == nil {
		t.Fatal("permission bypass")
	}
	work := f.terminalClient("fixture")
	name := f.clientName(work)
	if got := f.motley("tabs"); !strings.Contains(got, name) {
		t.Fatal(got)
	}
	f.motley("send", id, "--tab", name)
	if got := f.clientSession(name); got != id {
		t.Fatal(got)
	}
	f.tmux("new-session", "-d", "-s", "_motley", "/bin/sh")
	monitor := f.terminalClient("_motley")
	monitorName := f.clientName(monitor)
	if got := f.refused("send", id, "--tab", monitorName); !strings.Contains(got, "monitor") {
		t.Fatal(got)
	}
	if f.clientSession(monitorName) != "_motley" {
		t.Fatal("monitor moved")
	}
}

func TestSpawnFormTerminal(t *testing.T) {
	bin := buildLifecycleBinary(t)
	f := newMemberFixture(t, bin, "main")
	// The editor replaces only prompt text, leaving its schema comment intact.
	editor := filepath.Join(f.home, "prompt editor")
	writeFixture(t, editor, "#!/bin/sh\nprintf '<!-- schema = 1 -->\\nEdited prompt from terminal\\n' > \"$1\"\n", 0755)
	t.Setenv("EDITOR", quoteShell(editor))
	writeFixture(t, filepath.Join(f.home, "fake agents/claude"), "#!/bin/sh\nfor arg do printf '%s\\n' \"$arg\"; done > \"$HOME/received-prompt\"\n", 0755)
	terminal := startTerminal(t, exec.Command(bin))
	defer func() {
		if t.Failed() {
			t.Log(ansi.Strip(terminal.text()))
		}
	}()
	send := func(keys, want string) {
		t.Helper()
		offset := len(terminal.text())
		terminal.send(t, keys)
		eventually(t, func() bool { return strings.Contains(ansi.Strip(terminal.text()[offset:]), want) })
	}
	eventually(t, func() bool { return strings.Contains(terminal.text(), "No members yet") })
	send("s", "> api")
	send("api\r", "Ticket and name")
	send("412\tFX cache\r", "Agent")
	send("\r", "Blueprint")
	send("\r", "Prompt preview") // none: an edited prompt still works without a blueprint
	send("e", "Edited prompt from terminal")
	send("\r", "Created 412-fx-cache")
	m := f.manifest("412-fx-cache")
	if !m.Prompt || m.Blueprint != "" || m.Branch != "feat/412-fx-cache" {
		t.Fatal(m)
	}
	eventually(t, func() bool {
		data, _ := os.ReadFile(filepath.Join(f.home, "received-prompt"))
		return string(data) == "--\nEdited prompt from terminal\n\n"
	})
	terminal.send(t, "q")
	eventually(t, func() bool {
		select {
		case err := <-terminal.done:
			if err != nil {
				t.Fatal(err)
			}
			return true
		default:
			return false
		}
	})
}
