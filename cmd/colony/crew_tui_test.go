package main

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/thomashartm/colony/internal/crew"
)

func TestCrewEditorTerminal(t *testing.T) {
	bin := buildLifecycleBinary(t)
	f := newMinionFixture(t, bin, "main")
	f.colony("spawn", "--repo", "api", "--branch", "feat/crews", "--name", "Crew fixture", "--detach")
	terminal := startTerminal(t, exec.Command(bin))
	defer func() {
		if t.Failed() {
			t.Log(terminal.text())
		}
	}()
	send := func(keys, want string) {
		t.Helper()
		offset := len(terminal.text())
		terminal.send(t, keys)
		eventually(t, func() bool { return strings.Contains(ansi.Strip(terminal.text()[offset:]), want) })
	}
	eventually(t, func() bool { return strings.Contains(terminal.text(), "Crew fixture") })
	terminal.send(t, "G")
	eventually(t, func() bool { return strings.Contains(terminal.text(), "a add") })
	terminal.send(t, "a")
	eventually(t, func() bool { return strings.Contains(terminal.text(), "Add crew") })
	send("Banking\t\tblue\x13", "Saved")
	eventually(t, func() bool {
		cs, err := crew.Load()
		return err == nil && len(cs) == 1 && cs[0].ID == "banking" && cs[0].Color == "blue"
	})
	eventually(t, func() bool { return strings.Contains(terminal.text(), "Saved") })
	send("q", "g group")
	// Select a minion's identity editor, keeping its name and ticket.
	terminal.send(t, "e")
	eventually(t, func() bool { return strings.Contains(terminal.text(), "Edit feat-crews") })
	send("\t\tbanking\tyellow\x13", "Saved")
	eventually(t, func() bool { m := f.manifest("feat-crews"); return m.Crew == "banking" && m.Color == "yellow" })
	// The terminal remains responsive and grouping displays the member table.
	terminal.send(t, "g")
	eventually(t, func() bool {
		return strings.Contains(terminal.text(), "MINION") && strings.Contains(terminal.text(), "1 minions")
	})
	send("G", "a add")
	terminal.send(t, "x")
	eventually(t, func() bool { return strings.Contains(terminal.text(), "Delete crew banking?") })
	terminal.send(t, "y")
	eventually(t, func() bool { return strings.Contains(terminal.text(), "referenced by 1 minions") })
	terminal.send(t, "f")
	eventually(t, func() bool { return strings.Contains(terminal.text(), "Force unassign: true") })
	send("y", "Saved")
	eventually(t, func() bool {
		cs, err := crew.Load()
		return err == nil && len(cs) == 0 && f.manifest("feat-crews").Crew == ""
	})
	if m := f.manifest("feat-crews"); m.Color != "yellow" {
		t.Fatal("force removal lost override")
	}
	send("q", "tab members")
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
