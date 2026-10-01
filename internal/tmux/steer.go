package tmux

import "strings"

// SendText delivers literal text and Enter to the session's active pane.
func SendText(id, text string) error {
	// tmux parses a trailing semicolon even when argv bypasses the shell.
	if strings.HasSuffix(text, ";") {
		text = strings.TrimSuffix(text, ";") + `\;`
	}
	target := "=" + SessionName(id) + ":"
	_, err := run("send-keys", "-t", target, "-l", "--", text, ";", "send-keys", "-t", target, "Enter")
	return err
}
