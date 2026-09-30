// Package tmux invokes tmux, using its normal TMUX environment/socket selection.
package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

type Session struct {
	Name     string
	MinionID string
	Monitor  bool
}

func run(args ...string) (string, error) {
	out, err := exec.Command("tmux", args...).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("tmux: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func Sessions() ([]Session, error) {
	out, err := run("list-sessions", "-F", "#{session_name}\t#{@colony_minion}\t#{@colony_monitor}")
	if err != nil {
		if strings.Contains(out, "no server running on ") || strings.Contains(out, "no sessions") ||
			(strings.Contains(out, "error connecting to ") && strings.Contains(out, "No such file or directory")) {
			return nil, nil
		}
		return nil, err
	}
	var sessions []Session
	for _, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) < 2 {
			return nil, fmt.Errorf("unexpected tmux session record %q", line)
		}
		sessions = append(sessions, Session{Name: fields[0], MinionID: fields[1], Monitor: len(fields) > 2 && fields[2] == "1"})
	}
	return sessions, nil
}

func SessionName(id string) string {
	return strings.NewReplacer(".", "_", ":", "_").Replace(id)
}

func Start(id, worktree, ticket, agent string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	name := SessionName(id)
	args := []string{"new-session", "-d", "-s", name, "-c", worktree, "-e", "COLONY_MINION=" + id}
	// A long-running tmux server may have stale paths or state/config locations.
	for _, key := range []string{"PATH", "HOME", "XDG_STATE_HOME", "XDG_CONFIG_HOME", "SHELL"} {
		args = append(args, "-e", key+"="+os.Getenv(key))
	}
	// Multiple shell-command arguments bypass tmux's shell-string interpretation.
	// User-controlled values are positional arguments, never interpolated code.
	args = append(args, "/bin/sh", "-c", `"$1" exec-agent "$2"; exec "$3" -l`, "colony", self, id, shell)
	for _, option := range [][2]string{{"@colony_minion", id}, {"@colony_ticket", ticket}, {"@colony_agent", agent}} {
		args = append(args, ";", "set-option", "-t", "="+name+":", option[0], option[1])
	}
	_, err = run(args...)
	return err
}

func Switch(id string) error {
	_, err := run("switch-client", "-t", "="+SessionName(id))
	return err
}

func Attach(id string) error {
	bin, err := exec.LookPath("tmux")
	if err != nil {
		return err
	}
	return syscall.Exec(bin, []string{"tmux", "attach-session", "-t", "=" + SessionName(id)}, os.Environ())
}

const MonitorSession = "_colony"

type Client struct {
	Name     string
	TTY      string
	Session  string
	Activity int64
}

func Clients() ([]Client, error) {
	out, err := run("list-clients", "-F", "#{client_name}\t#{client_tty}\t#{client_session}\t#{client_activity}")
	if err != nil {
		if strings.Contains(out, "no server running on ") || strings.Contains(out, "no sessions") ||
			(strings.Contains(out, "error connecting to ") && strings.Contains(out, "No such file or directory")) {
			return nil, nil
		}
		return nil, err
	}
	var clients []Client
	for _, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 4 {
			return nil, fmt.Errorf("unexpected tmux client record %q", line)
		}
		activity, err := strconv.ParseInt(fields[3], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid tmux client activity: %w", err)
		}
		clients = append(clients, Client{Name: fields[0], TTY: fields[1], Session: fields[2], Activity: activity})
	}
	return clients, nil
}

func CurrentClient() (string, error) {
	out, err := run("display-message", "-p", "#{client_name}")
	return strings.TrimSpace(out), err
}

func SwitchClient(client, id string) error {
	if client == "" {
		return fmt.Errorf("no tmux client available; open a tab and run colony attach <id>")
	}
	_, err := run("switch-client", "-c", client, "-t", "="+SessionName(id))
	return err
}

func DetachClient(client string) error {
	_, err := run("detach-client", "-t", client)
	return err
}

// EnsureMonitor creates a persistent overview on this tmux server.
func EnsureMonitor() error {
	sessions, err := Sessions()
	if err != nil {
		return err
	}
	for _, session := range sessions {
		if session.Name == MonitorSession {
			if !session.Monitor || session.MinionID != "" {
				return fmt.Errorf("tmux session %s already exists and is not a colony monitor", MonitorSession)
			}
			return nil
		}
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	args := []string{"new-session", "-d", "-s", MonitorSession}
	for _, key := range []string{"PATH", "HOME", "XDG_STATE_HOME", "XDG_CONFIG_HOME", "SHELL"} {
		args = append(args, "-e", key+"="+os.Getenv(key))
	}
	args = append(args, self, "--monitor", ";", "set-option", "-t", "="+MonitorSession+":", "@colony_monitor", "1")
	_, err = run(args...)
	return err
}
