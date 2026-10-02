package tmux

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/thomashartm/motley/internal/shellx"
)

// Exercise real mouse input and copy-mode dispatch, without writing to the
// developer's clipboard. The pipe destination stands in for pbcopy on all OSes.
func TestClipboardMouseBindings(t *testing.T) {
	bin, err := exec.LookPath("tmux")
	if err != nil {
		t.Fatal(err)
	}
	socket := fmt.Sprintf("motley-clipboard-%d", time.Now().UnixNano())
	call := func(args ...string) string {
		t.Helper()
		out, err := exec.Command(bin, append([]string{"-L", socket}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("tmux %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	call("-f", "/dev/null", "new-session", "-d", "-s", "other", "/bin/sh")
	t.Cleanup(func() { _ = exec.Command(bin, "-L", socket, "kill-server").Run() })
	t.Setenv("TMUX", call("display-message", "-p", "-t", "other", "#{socket_path},#{pid},0"))
	t.Setenv("TMUX_PANE", "")
	call("set-option", "-s", "set-clipboard", "off")
	call("set-option", "-g", "mouse", "off")
	call("set-option", "-g", "copy-command", "user-copy-command")
	serverOptions := call("show-options", "-s")
	rootClicks := bindingCommand(call("list-keys", "-T", "root"), "MouseDown1Pane")
	wheel := bindingCommand(call("list-keys", "-T", "root"), "WheelDownPane")

	dir := t.TempDir()
	copied, fallback := filepath.Join(dir, "copied text"), filepath.Join(dir, "original copy")
	pipe := "cat > " + shellx.Quote(copied)
	for _, table := range []string{"copy-mode", "copy-mode-vi"} {
		call("bind-key", "-T", table, "MouseDragEnd1Pane", "send-keys", "-X", "copy-pipe-and-cancel", "cat > "+shellx.Quote(fallback))
	}
	originalDrag := bindingCommand(call("list-keys", "-T", "root"), "MouseDrag1Pane")
	call("bind-key", "-T", "root", "MouseDrag1Pane", "set-option -g @original-drag yes ; "+originalDrag)

	// Both member reattachment and monitor setup install the production bindings.
	for _, session := range []string{"member", MonitorSession} {
		call("new-session", "-d", "-s", session, "/bin/sh")
		var err error
		if session == "member" {
			call("set-option", "-t", session, "@motley_member", session)
			err = showShortcuts(session)
		} else {
			call("set-option", "-t", session, "@motley_monitor", "1")
			err = monitorNavigation()
		}
		if err != nil {
			t.Fatal(err)
		}
		if got := call("show-options", "-v", "-t", session, "mouse"); got != "on" {
			t.Fatalf("%s mouse = %q", session, got)
		}
		binding := bindingCommand(call("list-keys", "-T", "copy-mode"), "MouseDragEnd1Pane")
		if strings.Contains(binding, "/usr/bin/pbcopy") != (runtime.GOOS == "darwin") {
			t.Fatalf("unexpected platform clipboard binding: %s", binding)
		}
	}
	if err := clipboardBindings(pipe); err != nil {
		t.Fatal(err)
	}
	bindings := call("list-keys")
	if err := clipboardBindings(pipe); err != nil {
		t.Fatal(err)
	}
	if got := call("list-keys"); got != bindings {
		t.Fatal("repeated setup nested the bindings")
	}
	if got := call("show-options", "-s"); got != serverOptions {
		t.Fatal("server options changed")
	}
	if got := call("show-options", "-gqv", "mouse"); got != "off" {
		t.Fatal("global mouse changed")
	}
	if got := call("show-options", "-gqv", "copy-command"); got != "user-copy-command" {
		t.Fatal("global copy-command changed")
	}
	root := call("list-keys", "-T", "root")
	if bindingCommand(root, "MouseDown1Pane") != rootClicks || bindingCommand(root, "WheelDownPane") != wheel {
		t.Fatal("pane click or wheel binding changed")
	}

	cmd := exec.Command(bin, "-L", socket, "attach-session", "-t", "member")
	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, "TMUX=") && !strings.HasPrefix(env, "TERM=") {
			cmd.Env = append(cmd.Env, env)
		}
	}
	cmd.Env = append(cmd.Env, "TERM=xterm-256color")
	terminal, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 30, Cols: 120})
	if err != nil {
		t.Fatal(err)
	}
	go func() { _, _ = io.Copy(io.Discard, terminal) }()
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = terminal.Close(); _ = cmd.Wait() })
	wait := func(t *testing.T, check func() bool) {
		t.Helper()
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
			if check() {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatal("timed out waiting for tmux")
	}
	wait(t, func() bool { return call("list-clients", "-F", "#{client_name}") != "" })
	client := call("list-clients", "-F", "#{client_name}")
	send := func(s string) {
		t.Helper()
		if _, err := io.WriteString(terminal, s); err != nil {
			t.Fatal(err)
		}
	}
	for _, session := range []string{"member", MonitorSession, "other"} {
		for _, mode := range []string{"emacs", "vi"} {
			t.Run(session+"/"+mode, func(t *testing.T) {
				defer func() {
					if t.Failed() {
						t.Log(bindingCommand(call("list-keys", "-T", "root"), "MouseDrag1Pane"))
						t.Log(call("show-options", "-gqv", "@original-drag"))
						t.Log(call("capture-pane", "-p", "-t", session))
						data, _ := os.ReadFile(fallback)
						t.Logf("fallback data = %q", data)
						t.Log(call("display-message", "-p", "-t", session, "#{pane_in_mode}|#{selection_present}|#{copy_cursor_x}|#{copy_cursor_y}"))
					}
				}()
				call("switch-client", "-c", client, "-t", session)
				call("set-window-option", "-t", session, "mode-keys", mode)
				// Enable application mouse reporting, as the monitor and agents do.
				mouse, destination := "1", copied
				fixture := "stty raw -echo; printf '\\033[2J\\033[HCLIPBOARD_FIXTURE\\033[?1000h\\033[?1006h'; exec cat > /dev/null"
				if session == "other" {
					mouse, destination = "0", fallback
					call("set-option", "-t", session, "mouse", "on")
					fixture = "stty raw -echo; printf '\\033[2J\\033[HCLIPBOARD_FIXTURE'; exec cat > /dev/null"
				}
				call("respawn-pane", "-k", "-t", session, "/bin/sh", "-c", fixture)
				wait(t, func() bool {
					return call("display-message", "-p", "-t", session, "#{mouse_any_flag}|#{pane_current_command}") == mouse+"|cat" &&
						strings.Contains(call("capture-pane", "-p", "-t", session), "CLIPBOARD_FIXTURE")
				})
				_ = os.Remove(destination)
				send("\x1b[<0;1;1M\x1b[<32;9;1M")
				wait(t, func() bool { return call("display-message", "-p", "-t", session, "#{pane_in_mode}") == "1" })
				send("\x1b[<32;18;1M\x1b[<0;18;1m")
				wait(t, func() bool {
					data, _ := os.ReadFile(destination)
					return strings.TrimSpace(string(data)) == "CLIPBOARD_FIXTURE"
				})
				if got := call("show-buffer"); got != "CLIPBOARD_FIXTURE" {
					t.Fatalf("tmux buffer = %q", got)
				}
				if got := call("display-message", "-p", "-t", session, "#{pane_in_mode}"); got != "0" {
					t.Fatal("drag release did not leave copy mode")
				}
				got := call("show-options", "-gqv", "@original-drag")
				if (got == "yes") != (session == "other") {
					t.Fatalf("unexpected fallback dispatch: %q", got)
				}
			})
		}
	}
}
