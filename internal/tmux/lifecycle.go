package tmux

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func CurrentSession() (string, error) {
	if os.Getenv("TMUX") == "" || os.Getenv("TMUX_PANE") == "" {
		return "", fmt.Errorf("adopt must run inside a tmux pane")
	}
	out, err := run("display-message", "-p", "-t", os.Getenv("TMUX_PANE"), "#{session_name}")
	return strings.TrimSpace(out), err
}
func Kill(id string) error { _, err := run("kill-session", "-t", "="+SessionName(id)); return err }

func Adopt(session, id, ticket, agent string) error {
	// Rename first so all lifecycle commands can use the normal id-based target.
	if session != SessionName(id) {
		if _, err := run("rename-session", "-t", "="+session, SessionName(id)); err != nil {
			return err
		}
	}
	target := "=" + SessionName(id) + ":"
	args := []string{"set-environment", "-t", target, "MOTLEY_MEMBER", id}
	now := strconv.FormatInt(time.Now().Unix(), 10)
	for _, option := range [][2]string{{"@motley_member", id}, {"@motley_ticket", ticket}, {"@motley_agent", agent}, {"@motley_status", "idle"}, {"@motley_since", now}, {"@motley_seen", now}, {"status-left", StatusLeft}, {"status-left-length", "50"}, {"status-interval", "2"}} {
		args = append(args, ";", "set-option", "-t", target, option[0], option[1])
	}
	_, err := run(args...)
	if err != nil {
		return err
	}
	return showShortcuts(id)
}
