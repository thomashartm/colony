// Package tmux invokes tmux, using its normal TMUX environment/socket selection.
package tmux

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Session struct {
	Name     string
	MemberID string
	Monitor  bool
	Status   string
	Since    int64
	Seen     int64
}

func run(args ...string) (string, error) {
	out, err := exec.Command("tmux", args...).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("tmux: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func Sessions() ([]Session, error) {
	// Older tmux releases replace tabs with underscores for non-UTF-8 clients.
	// Force UTF-8 for machine-readable records, including outside tmux.
	out, err := run("-u", "list-sessions", "-F", "#{session_name}\t#{@motley_member}\t#{@motley_monitor}\t#{@motley_status}\t#{@motley_since}\t#{@motley_seen}")
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
		s := Session{Name: fields[0], MemberID: fields[1], Monitor: len(fields) > 2 && fields[2] == "1"}
		if len(fields) >= 6 {
			s.Status = fields[3]
			s.Since, _ = strconv.ParseInt(fields[4], 10, 64)
			s.Seen, _ = strconv.ParseInt(fields[5], 10, 64)
		}
		sessions = append(sessions, s)
	}
	return sessions, nil
}

func SessionName(id string) string {
	return strings.NewReplacer(".", "_", ":", "_").Replace(id)
}

const StatusLeft = "#{?#{@motley_member},#{@motley_status} #{@motley_ticket} ,}"

func Start(id, worktree, ticket, agent string) error {
	return start(id, worktree, ticket, agent, false)
}

func Revive(id, worktree, ticket, agent string) error {
	return start(id, worktree, ticket, agent, true)
}

func start(id, worktree, ticket, agent string, resume bool) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	name := SessionName(id)
	args := []string{"new-session", "-d", "-s", name, "-c", worktree, "-e", "MOTLEY_MEMBER=" + id}
	// A long-running tmux server may have stale paths or state/config locations.
	for _, key := range []string{"PATH", "HOME", "XDG_STATE_HOME", "XDG_CONFIG_HOME", "SHELL"} {
		args = append(args, "-e", key+"="+os.Getenv(key))
	}
	// Multiple shell-command arguments bypass tmux's shell-string interpretation.
	// User-controlled values are positional arguments, never interpolated code.
	command := `"$1" exec-agent "$2"; exec "$3" -l`
	if resume {
		command = `"$1" exec-agent "$2" --resume; exec "$3" -l`
	}
	args = append(args, "/bin/sh", "-c", command, "motley", self, id, shell)
	now := strconv.FormatInt(time.Now().Unix(), 10)
	for _, option := range [][2]string{{"@motley_member", id}, {"@motley_ticket", ticket}, {"@motley_agent", agent}, {"@motley_status", "starting"}, {"@motley_since", now}, {"@motley_seen", now}} {
		args = append(args, ";", "set-option", "-t", "="+name+":", option[0], option[1])
	}
	args = append(args, ";", "set-option", "-t", "="+name+":", "status-left", StatusLeft, ";", "set-option", "-t", "="+name+":", "status-left-length", "50", ";", "set-option", "-t", "="+name+":", "status-interval", "2")
	_, err = run(args...)
	return err
}

// ReportStatus and ReportUpdate together use at most two tmux processes.
type HookState struct {
	Status  string
	Context string
}

func ReportStatus(ctx context.Context, id string) (HookState, error) {
	out, err := exec.CommandContext(ctx, "tmux", "-u", "display-message", "-p", "-t", "="+SessionName(id)+":", "#{@motley_member}\t#{@motley_status}\t#{@motley_context}").CombinedOutput()
	if err != nil {
		return HookState{}, fmt.Errorf("tmux report lookup: %w: %s", err, strings.TrimSpace(string(out)))
	}
	fields := strings.SplitN(strings.TrimSuffix(string(out), "\n"), "\t", 3)
	if len(fields) != 3 || fields[0] != id {
		return HookState{}, fmt.Errorf("session is not marked as member %q", id)
	}
	return HookState{Status: fields[1], Context: fields[2]}, nil
}
func ReportUpdate(ctx context.Context, id, status, contextText string, changed bool, seen int64) error {
	target := "=" + SessionName(id) + ":"
	stamp := strconv.FormatInt(seen, 10)
	args := []string{"set-option", "-t", target, "@motley_seen", stamp}
	if contextText != "" {
		args = append(args, ";", "set-option", "-t", target, "@motley_context", contextText)
	}
	if changed {
		args = append(args, ";", "set-option", "-t", target, "@motley_status", status, ";", "set-option", "-t", target, "@motley_since", stamp)
	}
	out, err := exec.CommandContext(ctx, "tmux", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("tmux report update: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
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

const MonitorSession = "_motley"

type Client struct {
	Name     string
	TTY      string
	Session  string
	Activity int64
}

func Clients() ([]Client, error) {
	out, err := run("-u", "list-clients", "-F", "#{client_name}\t#{client_tty}\t#{client_session}\t#{client_activity}")
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
		return fmt.Errorf("no tmux client available; open a tab and run motley attach <id>")
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
			if !session.Monitor || session.MemberID != "" {
				return fmt.Errorf("tmux session %s already exists and is not a motley monitor", MonitorSession)
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
	args = append(args, self, "--monitor", ";", "set-option", "-t", "="+MonitorSession+":", "@motley_monitor", "1")
	_, err = run(args...)
	return err
}
