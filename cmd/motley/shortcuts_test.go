package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestMemberShortcutFooter(t *testing.T) {
	bin := buildLifecycleBinary(t)
	f := newMemberFixture(t, bin, "main")
	f.tmux("set-option", "-g", "status-format[0]", "Custom window list")
	f.tmux("set-option", "-g", "status-position", "top")
	f.tmux("bind-key", "-T", "prefix", "H", "set-option", "-g", "@original-H", "kept")
	f.motley("spawn", "--repo", "api", "--branch", "feat/footer", "--detach")
	id, target := "feat-footer", "=feat-footer:"
	if got := f.tmux("show-options", "-A", "-v", "-t", target, "status"); got != "4" {
		t.Fatalf("footer rows: %s", got)
	}
	if got := f.tmux("show-options", "-A", "-v", "-t", "=fixture:", "status"); got != "on" {
		t.Fatalf("unrelated session changed: %s", got)
	}
	if got := f.tmux("show-options", "-A", "-v", "-t", target, "status-format[0]"); got != "Custom window list" {
		t.Fatalf("original status row changed: %s", got)
	}
	// The global key binding retains its original action outside Motley.
	other := f.terminalClient("fixture")
	other.send(t, "\x02H")
	eventually(t, func() bool { return f.tmux("show-options", "-gqv", "@original-H") == "kept" })
	// Existing sessions gain the footer on reattach, even with status disabled.
	f.tmux("set-option", "-t", target, "status", "off")
	f.tmux("set-option", "-t", target, "prefix", "C-a")
	wrapper := filepath.Join(f.home, "isolated-bin")
	writeFixture(t, filepath.Join(wrapper, "tmux"), "#!/bin/sh\nexec "+quoteShell(f.tmuxBin)+" -L "+quoteShell(f.socket)+" \"$@\"\n", 0o755)
	t.Setenv("PATH", wrapper+string(os.PathListSeparator)+os.Getenv("PATH"))
	cmd := exec.Command(bin, "attach", id)
	cmd.Env = withoutTmux()
	client := startTerminal(t, cmd)
	t.Cleanup(func() {
		if t.Failed() {
			out := ansi.Strip(client.text())
			if len(out) > 1800 {
				out = out[len(out)-1800:]
			}
			t.Logf("terminal tail: %s", out)
			t.Logf("windows: %s", f.tmux("list-windows", "-t", "="+id))
			t.Logf("client: %s", f.tmux("display-message", "-p", "-c", f.clientName(client), "#{client_key_table}|#{client_prefix}"))
			t.Logf("formats: %s", f.tmux("show-options", "-t", target, "status-format"))
		}
	})
	eventually(t, func() bool {
		out := ansi.Strip(client.text())
		return strings.Contains(out, "Back to monitor") && strings.Contains(out, "mtly attach "+id)
	})
	if got := f.tmux("show-options", "-A", "-v", "-t", target, "status-position"); got != "bottom" {
		t.Fatalf("footer position: %s", got)
	}
	// Prefix H opens the monitor without typing into the agent; H returns.
	clientName := f.clientName(client)
	client.send(t, "\x01H")
	eventually(t, func() bool { return f.clientSession(clientName) == "_motley" })
	client.send(t, "\x02H") // monitor inherits the default prefix
	eventually(t, func() bool { return f.clientSession(clientName) == id })
	// Click the underlined Monitor control on status row 1 (30-row terminal).
	client.send(t, "\x1b[<0;14;28M\x1b[<0;14;28m")
	eventually(t, func() bool { return f.clientSession(clientName) == "_motley" })
	client.send(t, "\x02H")
	eventually(t, func() bool { return f.clientSession(clientName) == id })
	// Window and scroll controls target this tab even with another client active.
	f.tmux("new-window", "-d", "-t", "="+id, "/bin/sh")
	f.motley("navigation", "--client", clientName, "--action", "next")
	if got := f.tmux("display-message", "-p", "-t", target, "#{window_index}"); got != "1" {
		t.Fatalf("direct next: %s", got)
	}
	f.motley("navigation", "--client", clientName, "--action", "previous")
	client.send(t, "\x1b[<0;34;29M\x1b[<0;34;29m")
	eventually(t, func() bool { return f.tmux("display-message", "-p", "-t", target, "#{window_index}") == "1" })
	client.send(t, "\x1b[<0;26;29M\x1b[<0;26;29m")
	eventually(t, func() bool { return f.tmux("display-message", "-p", "-t", target, "#{window_index}") == "0" })
	client.send(t, "\x1b[<0;63;29M\x1b[<0;63;29m")
	eventually(t, func() bool { return f.tmux("display-message", "-p", "-t", target, "#{pane_in_mode}") == "1" })
	client.send(t, "q")
	eventually(t, func() bool { return f.tmux("display-message", "-p", "-t", target, "#{pane_in_mode}") == "0" })
	client.send(t, "\x01d")
	eventually(t, func() bool {
		select {
		case err := <-client.done:
			if err != nil {
				t.Fatal(err)
			}
			return true
		default:
			return false
		}
	})
	if got := f.tmux("has-session", "-t", target); got != "" {
		t.Fatalf("detach ended session: %s", got)
	}
}
