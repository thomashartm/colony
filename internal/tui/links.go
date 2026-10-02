package tui

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type linkOpened struct {
	url string
	err error
}

// Hit-test the rendered hyperlink, not an estimated table column. This also
// covers wrapped detail links, scrolled lists, popups and wide Unicode cells.
func hyperlinkAt(view string, x, y int) string {
	if x < 0 || y < 0 {
		return ""
	}
	column, row, target := 0, 0, ""
	var state byte
	for len(view) > 0 {
		seq, width, n, next := ansi.DecodeSequence(view, state, nil)
		if n == 0 {
			break
		}
		state, view = next, view[n:]
		if seq == "\n" {
			row++
			column = 0
			if row > y {
				break
			}
			continue
		}
		if url, ok := hyperlinkTarget(seq); ok {
			target = url
		}
		if row == y && width > 0 && x >= column && x < column+width {
			return target
		}
		column += width
	}
	return ""
}

func (m Model) openLink(target string) (tea.Model, tea.Cmd) {
	openURL := m.openURL
	if openURL == nil {
		openURL = openWebURL
	}
	return m, func() tea.Msg { return linkOpened{target, openURL(target)} }
}

func openWebURL(target string) error {
	if safeWebURL(target) == nil {
		return fmt.Errorf("only http and https links can be opened")
	}
	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	// Pass the URL as one argument; never interpolate it into a shell command.
	if err := exec.Command(opener, target).Run(); err != nil {
		return fmt.Errorf("%s: %w", opener, err)
	}
	return nil
}

func hyperlinkTarget(seq string) (string, bool) {
	if !strings.HasPrefix(seq, "\x1b]8;") {
		return "", false
	}
	data := strings.TrimSuffix(strings.TrimSuffix(seq, "\x1b\\"), "\a")
	_, target, _ := strings.Cut(strings.TrimPrefix(data, "\x1b]8;"), ";")
	return target, true
}

// End each physical line's hyperlink before labels, padding or the other panel
// are added. Restore it only around the continued value on the next line.
func wrapLinkedValue(value string, width int) []string {
	lines := strings.Split(ansi.Wrap(value, width, ""), "\n")
	active := ""
	for i, line := range lines {
		prefix := active
		var state byte
		for rest := line; len(rest) > 0; {
			seq, _, n, next := ansi.DecodeSequence(rest, state, nil)
			if n == 0 {
				break
			}
			state, rest = next, rest[n:]
			if target, ok := hyperlinkTarget(seq); ok {
				active = seq
				if target == "" {
					active = ""
				}
			}
		}
		lines[i] = prefix + line
		if active != "" {
			lines[i] += "\x1b]8;;\x1b\\"
		}
	}
	return lines
}
