package main

import (
	"bytes"
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
func (f *minionFixture) terminalClient(session string) *terminalProcess {
	cmd := exec.Command(f.tmuxBin, "-L", f.socket, "attach-session", "-t", "="+session)
	cmd.Env = withoutTmux()
	p := startTerminal(f.t, cmd)
	eventually(f.t, func() bool { return f.clientName(p) != "" })
	return p
}
func (f *minionFixture) clientName(p *terminalProcess) string {
	for _, line := range strings.Split(f.tmux("list-clients", "-F", "#{client_pid}\t#{client_name}"), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) == 2 && fields[0] == strconv.Itoa(p.cmd.Process.Pid) {
			return fields[1]
		}
	}
	return ""
}
func (f *minionFixture) clientSession(name string) string {
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
	bin := filepath.Join(t.TempDir(), "colony")
	commandOutput(t, "go", "build", "-o", bin, ".")
	f := newMinionFixture(t, bin, "main")
	f.colony("spawn", "--repo", "api", "--branch", "feat/overview", "--name", "Overview fixture", "--detach")
	id := "feat-overview"

	// A normal in-tmux TUI switches its own client and restores the pane on exit.
	f.tmux("new-session", "-d", "-s", "overview", "/bin/sh")
	overview := f.terminalClient("overview")
	overviewName := f.clientName(overview)
	f.tmux("send-keys", "-t", "=overview:", "-l", quoteShell(bin))
	f.tmux("send-keys", "-t", "=overview:", "Enter")
	eventually(t, func() bool {
		return strings.Contains(f.tmux("capture-pane", "-p", "-t", "=overview:"), "Overview fixture")
	})
	overview.send(t, "\r")
	eventually(t, func() bool { return f.clientSession(overviewName) == id })

	// Start the persistent monitor from this work tab; the process stays running
	// when the client detaches, and running colony monitor reuses the same session.
	f.tmux("send-keys", "-t", "="+id+":", "-l", quoteShell(bin)+" monitor")
	f.tmux("send-keys", "-t", "="+id+":", "Enter")
	eventually(t, func() bool { return f.clientSession(overviewName) == "_colony" })
	monitorPane := f.tmux("display-message", "-p", "-t", "=_colony:", "#{pane_id}")
	monitorPID := f.tmux("display-message", "-p", "-t", "=_colony:", "#{pane_pid}")
	eventually(t, func() bool {
		return strings.Contains(f.tmux("capture-pane", "-p", "-t", "=_colony:"), "colony monitor")
	})
	overview.send(t, "\r")
	eventually(t, func() bool {
		return strings.Contains(f.tmux("capture-pane", "-p", "-t", "=_colony:"), "open another tab")
	})
	if f.clientSession(overviewName) != "_colony" {
		t.Fatal("monitor took over its only client")
	}
	work := f.terminalClient("fixture")
	defer func() {
		if t.Failed() {
			t.Logf("work terminal output:\n%s", ansi.Strip(work.text()))
			t.Logf("tmux messages:\n%s", f.tmux("show-messages", "-t", f.clientName(work)))
		}
	}()
	workName := f.clientName(work)
	eventually(t, func() bool { return strings.Contains(f.tmux("capture-pane", "-p", "-t", "=_colony:"), workName) })
	overview.send(t, "\r")
	eventually(t, func() bool { return f.clientSession(workName) == id })
	if f.clientSession(overviewName) != "_colony" {
		t.Fatal("monitor client moved during jump")
	}

	// Pin the older client, then attach another work client. Enter must still
	// switch the pinned one, and the second work tab must remain untouched.
	overview.send(t, "T")
	eventually(t, func() bool { return strings.Contains(f.tmux("capture-pane", "-p", "-t", "=_colony:"), "Pin work tab") })
	overview.send(t, "j")
	eventually(t, func() bool { return strings.Contains(f.tmux("capture-pane", "-p", "-t", "=_colony:"), "> "+workName) })
	overview.send(t, "\r")
	eventually(t, func() bool { return strings.Contains(f.tmux("capture-pane", "-p", "-t", "=_colony:"), "(pinned)") })
	f.tmux("switch-client", "-c", workName, "-t", "=fixture")
	other := f.terminalClient("overview")
	otherName := f.clientName(other)
	overview.send(t, "\r")
	eventually(t, func() bool { return f.clientSession(workName) == id })
	if f.clientSession(otherName) != "overview" || f.clientSession(overviewName) != "_colony" {
		t.Fatal("pinned jump switched an unrelated client")
	}
	if strings.Contains(f.colony("ls"), "_colony") {
		t.Fatal("monitor listed as a minion")
	}
	overview.send(t, "q")
	eventually(t, func() bool { return f.clientSession(overviewName) == "" })
	if got := f.tmux("display-message", "-p", "-t", "=_colony:", "#{pane_pid}"); got != monitorPID {
		t.Fatal("detaching stopped the monitor")
	}
	// Re-enter through the public monitor command from another terminal pane.
	f.tmux("send-keys", "-t", "=overview:", "-l", quoteShell(bin)+" monitor")
	f.tmux("send-keys", "-t", "=overview:", "Enter")
	eventually(t, func() bool { return f.clientSession(otherName) == "_colony" })
	if f.tmux("display-message", "-p", "-t", "=_colony:", "#{pane_id}") != monitorPane {
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
	outside.send(t, "\r")
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
	f.colony("init")
	popupPath := filepath.Join(f.home, "config/colony/colony.tmux.conf")
	f.tmux("source-file", popupPath)
	binding := f.tmux("list-keys", "-T", "prefix", "h")
	if !strings.Contains(binding, "--client #{q:client_name}") {
		t.Fatalf("popup binding: %s", binding)
	}
	// The server PATH predates our test binary, as it may in daily use. Put colony
	// on its environment PATH before using the installed binding.
	f.tmux("set-environment", "-g", "PATH", filepath.Dir(bin)+string(os.PathListSeparator)+os.Getenv("PATH"))
	f.tmux("set-environment", "-t", "="+id, "PATH", filepath.Dir(bin)+string(os.PathListSeparator)+os.Getenv("PATH"))
	f.tmux("set-environment", "-t", "=fixture", "PATH", filepath.Dir(bin)+string(os.PathListSeparator)+os.Getenv("PATH"))
	f.tmux("switch-client", "-c", workName, "-t", "=fixture")
	work.send(t, "\x02")
	eventually(t, func() bool { return f.tmux("display-message", "-p", "-c", workName, "#{client_prefix}") == "1" })
	work.send(t, "h")
	eventually(t, func() bool {
		return strings.Contains(work.text(), "Overview fixture") && strings.Contains(work.text(), "pgup/pgdn")
	})
	work.send(t, "\r")
	eventually(t, func() bool { return f.clientSession(workName) == id })
	if f.clientSession(otherName) != "_colony" {
		t.Fatal("popup changed monitor client")
	}
	// The polling overview notices session death without being restarted.
	f.tmux("kill-session", "-t", "="+id)
	eventually(t, func() bool {
		return strings.Contains(f.tmux("capture-pane", "-p", "-t", "=_colony:"), "0 alive · 1 dead")
	})
}

func TestOverviewNonTerminal(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cmd := newRootCommand()
	cmd.SetArgs(nil)
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "requires a terminal") {
		t.Fatalf("non-terminal error: %v", err)
	}
}
