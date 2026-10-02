package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/thomashartm/motley/internal/member"
	"github.com/thomashartm/motley/internal/state"
)

func TestReplyAndSendIntegration(t *testing.T) {
	bin := buildLifecycleBinary(t)
	f := newMemberFixture(t, bin, "main")
	// Keep a fake agent reading its terminal, so replies cannot fall into a shell.
	writeFixture(t, filepath.Join(f.home, "fake agents/claude"), "#!/bin/sh\nwhile IFS= read -r line; do [ \"$line\" = quit ] && exit; printf '%s\\n' \"$line\" >> \"$HOME/replies\"; done\n", 0755)
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
	// An agent that quits without an exit hook (OpenCode) or crashes leaves its
	// last status behind; replies must not then be typed into the shell.
	logPath := filepath.Join(f.state, "motley/members", id+".events.jsonl")
	for _, revive := range []bool{false, true} {
		if revive {
			f.tmux("kill-session", "-t", "="+id)
			f.motley("revive", id)
		}
		f.tmux("set-option", "-t", "="+id+":", "@motley_status", "ready")
		if err := member.Reply(id, "quit"); err != nil {
			t.Fatal(err)
		}
		eventually(t, func() bool { return f.tmux("show-options", "-v", "-t", "="+id+":", "@motley_status") == "ended" })
		if last, err := state.LatestEvent(logPath); err != nil || last.Event != "AgentExit" || last.Agent != "claude" || last.Summary != "Agent exited" {
			t.Fatalf("exit event: %+v %v", last, err)
		}
		if err := member.Reply(id, "touch typed-into-shell"); err == nil || !strings.Contains(err.Error(), "agent has ended") {
			t.Fatalf("reply after exit: %v", err)
		}
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
	send("\r", "Permission mode") // none: an edited prompt still works without a blueprint
	send("j", "> manual")
	send("j", "> acceptEdits")
	send("j", "> plan")
	send("\r", "feat/412-fx-cache · claude · plan")
	send("e", "Edited prompt from terminal")
	send("\r", "Created 412-fx-cache")
	m := f.manifest("412-fx-cache")
	if !m.Prompt || m.Blueprint != "" || m.Branch != "feat/412-fx-cache" || m.Mode != "plan" {
		t.Fatal(m)
	}
	eventually(t, func() bool {
		data, _ := os.ReadFile(filepath.Join(f.home, "received-prompt"))
		return string(data) == "--permission-mode\nplan\n--\nEdited prompt from terminal\n\n"
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
