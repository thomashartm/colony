package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/creack/pty"
)

type terminalProcess struct {
	file   *os.File
	cmd    *exec.Cmd
	mu     sync.Mutex
	output bytes.Buffer
	done   chan error
}

func startTerminal(t *testing.T, cmd *exec.Cmd) *terminalProcess {
	t.Helper()
	terminal, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 30, Cols: 120})
	if err != nil {
		t.Fatal(err)
	}
	p := &terminalProcess{file: terminal, cmd: cmd, done: make(chan error, 1)}
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := terminal.Read(buf)
			if n > 0 {
				p.mu.Lock()
				_, _ = p.output.Write(buf[:n])
				p.mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	go func() { p.done <- cmd.Wait() }()
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = terminal.Close() })
	return p
}
func (p *terminalProcess) text() string { p.mu.Lock(); defer p.mu.Unlock(); return p.output.String() }
func (p *terminalProcess) send(t *testing.T, s string) {
	t.Helper()
	if _, err := io.WriteString(p.file, s); err != nil {
		t.Fatal(err)
	}
}
func withoutTmux() []string {
	var env []string
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "TMUX=") && !strings.HasPrefix(e, "TMUX_PANE=") && !strings.HasPrefix(e, "TERM=") {
			env = append(env, e)
		}
	}
	return append(env, "TERM=xterm-256color")
}
func (f *memberFixture) terminalClient(session string) *terminalProcess {
	cmd := exec.Command(f.tmuxBin, "-L", f.socket, "attach-session", "-t", "="+session)
	cmd.Env = withoutTmux()
	p := startTerminal(f.t, cmd)
	eventually(f.t, func() bool { return f.clientName(p) != "" })
	return p
}
func (f *memberFixture) clientName(p *terminalProcess) string {
	for _, line := range strings.Split(f.tmux("list-clients", "-F", "#{client_pid}\t#{client_name}"), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) == 2 && fields[0] == strconv.Itoa(p.cmd.Process.Pid) {
			return fields[1]
		}
	}
	return ""
}
func (f *memberFixture) clientSession(name string) string {
	for _, line := range strings.Split(f.tmux("list-clients", "-F", "#{client_name}\t#{client_session}"), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) == 2 && fields[0] == name {
			return fields[1]
		}
	}
	return ""
}
func quoteShell(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }

func TestOverviewAndMonitor(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "motley")
	commandOutput(t, "go", "build", "-o", bin, ".")
	f := newMemberFixture(t, bin, "main")
	f.motley("spawn", "--repo", "api", "--branch", "feat/overview", "--name", "Overview fixture", "--detach")
	id := "feat-overview"
	f.motley("crew", "add", "--title", "Overview crew", "--color", "blue")
	f.motley("crew", "assign", id, "overview-crew")

	// A normal in-tmux TUI switches its own client and restores the pane on exit.
	f.tmux("new-session", "-d", "-s", "overview", "/bin/sh")
	overview := f.terminalClient("overview")
	overviewName := f.clientName(overview)
	f.tmux("send-keys", "-t", "=overview:", "-l", quoteShell(bin))
	f.tmux("send-keys", "-t", "=overview:", "Enter")
	eventually(t, func() bool {
		return strings.Contains(f.tmux("capture-pane", "-p", "-t", "=overview:"), "Overview fixture")
	})
	// Main actions are reachable without a member selection. The Open picker
	// then chooses the destination explicitly and switches the same client.
	overview.send(t, "\x1b[H")
	eventually(t, func() bool {
		view := f.tmux("capture-pane", "-p", "-t", "=overview:")
		return strings.Contains(view, "Overview actions") && strings.Contains(view, "Spawn member") && !strings.Contains(view, "Edit member")
	})
	overview.send(t, "o")
	eventually(t, func() bool { return strings.Contains(f.tmux("capture-pane", "-p", "-t", "=overview:"), "[Open agent]") })
	overview.send(t, "\r")
	eventually(t, func() bool { return f.clientSession(overviewName) == id })

	// Running bare motley inside an agent must open the independent monitor; the process stays running
	// when the client detaches, and running motley monitor reuses the same session.
	f.tmux("send-keys", "-t", "="+id+":", "-l", quoteShell(bin))
	f.tmux("send-keys", "-t", "="+id+":", "Enter")
	eventually(t, func() bool { return f.clientSession(overviewName) == "_motley" })
	monitorPane := f.tmux("display-message", "-p", "-t", "=_motley:", "#{pane_id}")
	monitorPID := f.tmux("display-message", "-p", "-t", "=_motley:", "#{pane_pid}")
	eventually(t, func() bool {
		return strings.Contains(f.tmux("capture-pane", "-p", "-t", "=_motley:"), "motley monitor")
	})
	overview.send(t, "g\x1b[C\x1b[C")
	eventually(t, func() bool {
		return strings.Contains(f.tmux("capture-pane", "-p", "-t", "=_motley:"), "MEMBER")
	})
	overview.send(t, "\r")
	eventually(t, func() bool { return f.clientSession(overviewName) == id })
	// Return through the visible monitor control; the overview keeps its state.
	overview.send(t, "\x02m")
	eventually(t, func() bool { return f.clientSession(overviewName) == "_motley" })
	eventually(t, func() bool {
		return strings.Contains(f.tmux("capture-pane", "-p", "-t", "=_motley:"), "[o Open agent]")
	})
	paneHeight, err := strconv.Atoi(f.tmux("display-message", "-p", "-t", "=_motley:", "#{pane_height}"))
	if err != nil {
		t.Fatal(err)
	}
	// Open and return using visible buttons, with no tmux key sequence.
	overview.send(t, fmt.Sprintf("\x1b[<0;5;%dM\x1b[<0;5;%dm", paneHeight, paneHeight))
	eventually(t, func() bool { return f.clientSession(overviewName) == id })
	overview.send(t, "\x1b[<0;14;28M\x1b[<0;14;28m")
	eventually(t, func() bool { return f.clientSession(overviewName) == "_motley" })
	// Click each panel button through the tmux client (not directly into the model).
	for _, test := range []struct{ button, hint string }{{"[1 List]", "[List] Nav:"}, {"[2 Details]", "[Details] Nav:"}, {"[3 Actions]", "[Actions] Nav:"}} {
		view := f.tmux("capture-pane", "-p", "-t", "=_motley:")
		lines := strings.Split(view, "\n")
		x := strings.Index(lines[len(lines)-1], test.button) + 2
		if x < 2 {
			t.Fatalf("button missing: %s\n%s", test.button, view)
		}
		overview.send(t, fmt.Sprintf("\x1b[<0;%d;%dM\x1b[<0;%d;%dm", x, paneHeight, x, paneHeight))
		eventually(t, func() bool { return strings.Contains(f.tmux("capture-pane", "-p", "-t", "=_motley:"), test.hint) })
	}
	// Real no-button motion must reach the TUI (all-motion tracking, not drag-only).
	view := f.tmux("capture-pane", "-p", "-t", "=_motley:")
	hovered := false
	for y, line := range strings.Split(view, "\n") {
		if x := strings.Index(line, "Edit member (e)"); x >= 0 {
			overview.send(t, fmt.Sprintf("\x1b[<35;%d;%dM", x+1, y+1))
			hovered = true
			break
		}
	}
	if !hovered {
		t.Fatalf("Edit member action missing:\n%s", view)
	}
	eventually(t, func() bool {
		view := f.tmux("capture-pane", "-p", "-t", "=_motley:")
		return strings.Contains(view, "> Edit member (e)") && strings.Contains(view, "Opens an editor")
	})
	for _, test := range []struct{ key, hint string }{{"1", "[List] Nav:"}, {"2", "[Details] Nav:"}, {"3", "[Actions] Nav:"}} {
		overview.send(t, test.key)
		eventually(t, func() bool { return strings.Contains(f.tmux("capture-pane", "-p", "-t", "=_motley:"), test.hint) })
	}
	overview.send(t, "2")
	work := f.terminalClient("fixture")
	defer func() {
		if t.Failed() {
			t.Logf("work terminal output:\n%s", ansi.Strip(work.text()))
			t.Logf("tmux messages:\n%s", f.tmux("show-messages", "-t", f.clientName(work)))
		}
	}()
	workName := f.clientName(work)
	eventually(t, func() bool { return strings.Contains(f.tmux("capture-pane", "-p", "-t", "=_motley:"), workName) })
	overview.send(t, "\r")
	eventually(t, func() bool { return f.clientSession(workName) == id })
	if f.clientSession(overviewName) != "_motley" {
		t.Fatal("monitor client moved during jump")
	}

	// Pin the older client, then attach another work client. Enter must still
	// switch the pinned one, and the second work tab must remain untouched.
	overview.send(t, "p")
	eventually(t, func() bool { return strings.Contains(f.tmux("capture-pane", "-p", "-t", "=_motley:"), "Pin work tab") })
	overview.send(t, "j")
	eventually(t, func() bool { return strings.Contains(f.tmux("capture-pane", "-p", "-t", "=_motley:"), "> "+workName) })
	overview.send(t, "\r")
	eventually(t, func() bool { return strings.Contains(f.tmux("capture-pane", "-p", "-t", "=_motley:"), "(pinned)") })
	f.tmux("switch-client", "-c", workName, "-t", "=fixture")
	other := f.terminalClient("overview")
	otherName := f.clientName(other)
	overview.send(t, "\r")
	eventually(t, func() bool { return f.clientSession(workName) == id })
	if f.clientSession(otherName) != "overview" || f.clientSession(overviewName) != "_motley" {
		t.Fatal("pinned jump switched an unrelated client")
	}
	if strings.Contains(f.motley("ls"), "_motley") {
		t.Fatal("monitor listed as a member")
	}
	overview.send(t, "q")
	eventually(t, func() bool { return f.clientSession(overviewName) == "" })
	if got := f.tmux("display-message", "-p", "-t", "=_motley:", "#{pane_pid}"); got != monitorPID {
		t.Fatal("detaching stopped the monitor")
	}
	// Re-enter through the public monitor command from another terminal pane.
	f.tmux("send-keys", "-t", "=overview:", "-l", quoteShell(bin)+" monitor")
	f.tmux("send-keys", "-t", "=overview:", "Enter")
	eventually(t, func() bool { return f.clientSession(otherName) == "_motley" })
	if f.tmux("display-message", "-p", "-t", "=_motley:", "#{pane_id}") != monitorPane {
		t.Fatal("monitor was recreated instead of reused")
	}

	// A standalone overview replaces itself with a real tmux attach. A wrapper
	// selects our isolated server even though TMUX is deliberately unset.
	wrapperDir := filepath.Join(f.home, "isolated-bin")
	writeFixture(t, filepath.Join(wrapperDir, "tmux"), "#!/bin/sh\nexec "+quoteShell(f.tmuxBin)+" -L "+quoteShell(f.socket)+" \"$@\"\n", 0o755)
	t.Setenv("PATH", wrapperDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	cmd := exec.Command(bin)
	// Exercise older tmux's non-UTF-8 output path as well as a real attach.
	cmd.Env = append(withoutTmux(), "LC_ALL=C", "LANG=C")
	outside := startTerminal(t, cmd)
	defer func() {
		if t.Failed() {
			t.Logf("outside terminal output:\n%s", ansi.Strip(outside.text()))
		}
	}()
	eventually(t, func() bool { return strings.Contains(outside.text(), "Overview fixture") })
	outside.send(t, "\x1b[C\r")
	eventually(t, func() bool { return f.clientSession(f.clientName(outside)) == id })
	outside.send(t, "\x02d")
	eventually(t, func() bool {
		select {
		case err := <-outside.done:
			if err != nil {
				t.Fatalf("outside attach: %v\n%s", err, outside.text())
			}
			return true
		default:
			return false
		}
	})

	// The generated popup config parses on tmux and binds a known originating
	// client. Exercise the actual popup with two attached work/monitor clients.
	f.motley("init")
	popupPath := filepath.Join(f.home, "config/motley/motley.tmux.conf")
	f.tmux("source-file", popupPath)
	// Some tmux versions show a single requested key in the client's status
	// line; listing the table consistently writes machine-readable stdout.
	binding := f.tmux("list-keys", "-T", "prefix")
	if !strings.Contains(binding, "--client #{q:client_name}") {
		t.Fatalf("popup binding: %s", binding)
	}
	// The server PATH predates our test binary, as it may in daily use. Put motley
	// on its environment PATH before using the installed binding.
	f.tmux("set-environment", "-g", "PATH", filepath.Dir(bin)+string(os.PathListSeparator)+os.Getenv("PATH"))
	f.tmux("set-environment", "-t", "="+id, "PATH", filepath.Dir(bin)+string(os.PathListSeparator)+os.Getenv("PATH"))
	f.tmux("set-environment", "-t", "=fixture", "PATH", filepath.Dir(bin)+string(os.PathListSeparator)+os.Getenv("PATH"))
	f.tmux("switch-client", "-c", workName, "-t", "=fixture")
	popupOffset := len(work.text())
	work.send(t, "\x02")
	eventually(t, func() bool { return f.tmux("display-message", "-p", "-c", workName, "#{client_prefix}") == "1" })
	work.send(t, "h")
	eventually(t, func() bool {
		// Titles already contain the name; wait for a loaded popup row.
		view := ansi.Strip(work.text()[popupOffset:])
		return strings.Contains(view, "Status:") && strings.Contains(view, "[List]")
	})
	work.send(t, "\x1b[C\r")
	eventually(t, func() bool { return f.clientSession(workName) == id })
	if f.clientSession(otherName) != "_motley" {
		t.Fatal("popup changed monitor client")
	}
	// The polling overview notices session death without being restarted.
	f.tmux("kill-session", "-t", "="+id)
	eventually(t, func() bool {
		return strings.Contains(f.tmux("capture-pane", "-p", "-t", "=_motley:"), "0 alive · 1 dead")
	})
}

func TestOverviewNonTerminal(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cmd := newRootCommand()
	cmd.SetArgs(nil)
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "requires a terminal") {
		t.Fatalf("non-terminal error: %v", err)
	}
}
