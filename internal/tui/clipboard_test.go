package tui

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/thomashartm/motley/internal/member"
	"github.com/thomashartm/motley/internal/tmux"
)

type copyCall struct{ client, text string }

// copyModel is a sized overview whose clipboard records every copy.
func copyModel(monitor bool, result error) (Model, *[]copyCall) {
	calls := &[]copyCall{}
	m := newModel(monitor, true, "work-client", nil)
	m.copyText = func(client, text string) error {
		*calls = append(*calls, copyCall{client, text})
		return result
	}
	m = update(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	return update(m, snapshot{rows: []member.Row{row("a", true)}}), calls
}

// run executes a command chain until it yields a message the model handles.
func run(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		t.Fatal("no command")
	}
	return update(m, cmd())
}

func messageRow(m Model) string {
	lines := strings.Split(m.View(), "\n")
	return lines[len(lines)-1-m.footerRows()]
}

func TestCopyMessageCopiesFullText(t *testing.T) {
	m, calls := copyModel(false, nil)
	long := "focus Ghostty: 228:309: execution error: " + strings.Repeat("very long detail ", 10) + "END"
	m.message = long
	line := messageRow(m)
	if !strings.HasSuffix(line, " [c copy]") || strings.Contains(line, "END") || ansi.StringWidth(line) > m.width {
		t.Fatalf("message line %q", line)
	}
	next, cmd := m.Update(key("c"))
	m = run(t, next.(Model), cmd)
	if len(*calls) != 1 || (*calls)[0] != (copyCall{"work-client", long}) {
		t.Fatalf("copied %q", *calls)
	}
	if line := messageRow(m); !strings.HasSuffix(line, " ✓ copied") || m.message != long {
		t.Fatalf("after copy %q", line)
	}
	m.message = "Another message"
	if line := messageRow(m); !strings.HasPrefix(line, "Another message") || !strings.HasSuffix(line, " [c copy]") {
		t.Fatalf("new message %q", line)
	}
}

func TestCopyMessagePrefersPollErrorAndSkipsTransientText(t *testing.T) {
	m, calls := copyModel(false, nil)
	m.message = "older"
	m = update(m, snapshot{err: errors.New("server unavailable")})
	next, cmd := m.Update(key("c"))
	run(t, next.(Model), cmd)
	if len(*calls) != 1 || (*calls)[0].text != "server unavailable" {
		t.Fatalf("copied %q", *calls)
	}
	m.busy = true
	if m.copyableMessage() != "" || strings.Contains(messageRow(m), "[c copy]") {
		t.Fatal("busy text offered for copying")
	}
	m.busy, m.searching = false, true
	if m.copyableMessage() != "" || strings.Contains(messageRow(m), "[c copy]") {
		t.Fatal("filter input offered for copying")
	}
}

func TestCopyMessageWithoutMessage(t *testing.T) {
	m, calls := copyModel(false, nil)
	for _, a := range m.actions() {
		if a.key == "c" {
			t.Fatal("copy offered without a message")
		}
	}
	next, cmd := m.Update(key("c"))
	m = next.(Model)
	if cmd != nil || len(*calls) != 0 || m.message != "No message to copy." {
		t.Fatalf("message %q calls %v", m.message, *calls)
	}
	actions := m.actions()
	if last := actions[len(actions)-1]; last.label != "Copy message (c)" || last.key != "c" {
		t.Fatalf("copy action must be last: %v", actions)
	}
}

func TestCopyMessageFromClickAndActions(t *testing.T) {
	m, calls := copyModel(false, nil)
	m.message = "Copy me"
	next, cmd := m.Update(tea.MouseMsg{X: 2, Y: m.height - 1 - m.footerRows(), Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m = run(t, next.(Model), cmd)
	m.copied = ""
	m = update(m, key("3"))
	actions := m.actions()
	m.actionCursor = len(actions) - 1
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	run(t, next.(Model), cmd)
	if len(*calls) != 2 || (*calls)[0].text != "Copy me" || (*calls)[1].text != "Copy me" {
		t.Fatalf("copied %q", *calls)
	}
	// A click on the panels above never copies.
	next, _ = m.Update(tea.MouseMsg{X: 2, Y: m.height - 2 - m.footerRows(), Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if len(*calls) != 2 || next.(Model).message != "Copy me" {
		t.Fatal("panel click copied")
	}
}

func TestCopyMessageFailureIsReported(t *testing.T) {
	m, _ := copyModel(false, errors.New("saved as a tmux paste buffer only; tmux set-clipboard is off"))
	m.message = "Copy me"
	next, cmd := m.Update(key("c"))
	m = run(t, next.(Model), cmd)
	if m.message != "Copy failed: saved as a tmux paste buffer only; tmux set-clipboard is off" || m.copied != "" {
		t.Fatalf("message %q", m.message)
	}
}

// Every monitor tab shows the same process; copy to the tab used last.
func TestCopyMessageTargetsActiveMonitorTab(t *testing.T) {
	m, calls := copyModel(true, nil)
	m.clients = []tmux.Client{
		{Name: "/dev/ttys001", Session: tmux.MonitorSession, Activity: 10},
		{Name: "/dev/ttys002", Session: tmux.MonitorSession, Activity: 30},
		{Name: "/dev/ttys003", Session: "work", Activity: 50},
	}
	m.message = "Copy me"
	next, cmd := m.Update(key("c"))
	run(t, next.(Model), cmd)
	if len(*calls) != 1 || (*calls)[0].client != "/dev/ttys002" {
		t.Fatalf("copied %q", *calls)
	}
}

func TestCopyOutsideTmuxWritesOSC52(t *testing.T) {
	var out bytes.Buffer
	if err := copyToClipboard(false, &out)("ignored", "héllo\nworld"); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "\x1b]52;c;aMOpbGxvCndvcmxk\a" {
		t.Fatalf("%q", got)
	}
}
