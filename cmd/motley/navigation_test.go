package main

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestArrowEditorTerminal(t *testing.T) {
	bin := buildLifecycleBinary(t)
	f := newMemberFixture(t, bin, "main")
	f.motley("spawn", "--repo", "api", "--branch", "feat/arrows", "--name", "Arrow fixture", "--detach")
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
	eventually(t, func() bool { return strings.Contains(terminal.text(), "Arrow fixture") })
	send("\x1b[C", "[Details]")
	send("\x1b[C", "[Actions]")
	send("\r", "Edit feat-arrows")
	send(" changed\x1b[B\x1b[B\x1b[B\x1b[B", "> Save")
	send("\r", "Saved")
	eventually(t, func() bool { return f.manifest("feat-arrows").Name == "Arrow fixture changed" })
	send("\r", "Edit feat-arrows")
	send(" discarded\x1b[B\x1b[B\x1b[B\x1b[B\x1b[B", "> Cancel")
	send("\r", "[Actions]")
	if f.manifest("feat-arrows").Name != "Arrow fixture changed" {
		t.Fatal("cancel wrote changes")
	}
	send("\r", "Edit feat-arrows")
	send(" discarded\x1b", "[Actions]")
	if f.manifest("feat-arrows").Name != "Arrow fixture changed" {
		t.Fatal("escape wrote changes")
	}
	send("\x1b[D", "[Details]")
	send("\x1b[D", "[List]")
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
