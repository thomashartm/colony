package tui

import (
	"encoding/base64"
	"fmt"
	"io"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/thomashartm/motley/internal/tmux"
)

type copiedMsg struct {
	text string
	err  error
}

// copyableMessage is the full text behind the message line; the line itself
// is truncated to the terminal width. Transient states are not messages.
func (m Model) copyableMessage() string {
	if m.busy || m.searching {
		return ""
	}
	if m.pollError != "" {
		return m.pollError
	}
	return m.message
}

func (m Model) copyMessage() (tea.Model, tea.Cmd) {
	text := m.copyableMessage()
	if text == "" {
		m.message = "No message to copy."
		return m, nil
	}
	if m.copyText == nil {
		return m, nil
	}
	copyText, client := m.copyText, m.client
	if m.monitor {
		client = m.activeMonitorClient()
	}
	return m, func() tea.Msg { return copiedMsg{text: text, err: copyText(client, text)} }
}

// messageLine renders the message with its copy control; the control stays
// visible however long the message is.
func (m Model) messageLine(message string) string {
	text := m.copyableMessage()
	control := " [c copy]"
	if text == "" || text != message {
		return fit(clean(message), m.width)
	}
	if m.copied == text {
		control = " ✓ copied"
	}
	return fit(clean(message), m.width-ansi.StringWidth(control)) + control
}

// activeMonitorClient is the monitor tab where the last key was pressed, since
// every monitor tab shows the same process.
func (m Model) activeMonitorClient() string {
	var own tmux.Client
	for _, c := range m.clients {
		if c.Session == tmux.MonitorSession && (own.Name == "" || c.Activity > own.Activity) {
			own = c
		}
	}
	return own.Name
}

// copyToClipboard reaches the terminal showing Motley: through tmux inside a
// session, otherwise with the OSC 52 clipboard sequence.
func copyToClipboard(inside bool, out io.Writer) func(client, text string) error {
	return func(client, text string) error {
		if inside {
			return tmux.SetClipboard(client, text)
		}
		_, err := fmt.Fprintf(out, "\x1b]52;c;%s\a", base64.StdEncoding.EncodeToString([]byte(text)))
		return err
	}
}
