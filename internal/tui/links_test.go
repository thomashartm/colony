package tui

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/thomashartm/motley/internal/member"
)

func TestHyperlinkHitTesting(t *testing.T) {
	target := "https://example.com/issues/42"
	text := "outside " + link("界#42", target) + " outside"
	for _, tc := range []struct {
		x, y int
		want string
	}{{7, 0, ""}, {8, 0, target}, {9, 0, target}, {10, 0, target}, {12, 0, target}, {13, 0, ""}, {8, 1, ""}, {-1, 0, ""}} {
		if got := hyperlinkAt(text, tc.x, tc.y); got != tc.want {
			t.Fatalf("%d,%d got %q", tc.x, tc.y, got)
		}
	}
	wrapped := ansi.Wrap(link("one two three four", target), 5, "")
	for y, line := range strings.Split(wrapped, "\n") {
		if ansi.StringWidth(line) > 0 && hyperlinkAt(wrapped, 0, y) != target {
			t.Fatalf("wrapped hyperlink missed row %d: %q", y, wrapped)
		}
	}
	if got := hyperlinkAt("\x1b]8;id=one;"+target+"\a#42\x1b]8;;\a", 1, 0); got != target {
		t.Fatal("BEL terminated link", got)
	}
}

func TestLinkClicksUseVisibleCellsAndRespectModals(t *testing.T) {
	for _, size := range [][2]int{{60, 10}, {80, 24}, {140, 40}} {
		m := update(newModel(false, false, "", nil), tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		r := row("ticket", true)
		r.Ticket, r.RemoteURL = "42", "https://github.com/owner/repo.git"
		m = update(m, snapshot{rows: []member.Row{row("first", true), r}})
		m = arrow(m, tea.KeyDown)
		target := "https://github.com/owner/repo/issues/42"
		calls := []string{}
		m.openURL = func(url string) error { calls = append(calls, url); return nil }
		x, y := -1, -1
		for row, line := range strings.Split(m.View(), "\n") {
			for col := 0; col <= m.listWidth(); col++ {
				if hyperlinkAt(line, col, 0) == target {
					x, y = col, row
					break
				}
			}
			if x >= 0 {
				break
			}
		}
		if x < 0 {
			t.Fatal("no visible ticket link", size)
		}
		for _, event := range []tea.MouseMsg{{X: x, Y: y, Action: tea.MouseActionMotion}, {X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease}} {
			_, cmd := m.Update(event)
			if cmd != nil {
				t.Fatal("non-click opened a link")
			}
		}
		next, cmd := m.Update(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		if cmd == nil {
			t.Fatal("click did not open link", size)
		}
		m = update(next.(Model), cmd())
		if len(calls) != 1 || calls[0] != target || m.selectedID() != "ticket" || !strings.Contains(m.message, "Opened") {
			t.Fatal("click changed target or did not report success")
		}
		m.openURL = func(string) error { return errors.New("browser unavailable") }
		_, cmd = m.openLink(target)
		m = update(m, cmd())
		if !strings.Contains(m.message, "browser unavailable") {
			t.Fatal("failure hidden")
		}
		m = update(m, key("e"))
		_, cmd = m.Update(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
		if cmd != nil {
			t.Fatal("background link activated behind editor")
		}
	}
}

func TestWrappedAndScrolledDetailLink(t *testing.T) {
	m := update(newModel(false, false, "", nil), tea.WindowSizeMsg{Width: 60, Height: 20})
	snap := crewSnapshot()
	snap.crews[1].Title = strings.Repeat("Banking ", 10)
	m = update(m, snap)
	m.panel = detailPanel
	target := snap.crews[1].URL
	m.openURL = func(url string) error {
		if url != target {
			t.Fatalf("wrong wrapped link %q", url)
		}
		return nil
	}
	found := false
	for i := 0; i < 40; i++ {
		for y, line := range strings.Split(m.View(), "\n") {
			if hyperlinkAt(m.View(), 1, y) != "" {
				t.Fatal("wrapped link leaked into the left panel")
			}
			for x := m.listWidth() + 3; x < m.width-1; x++ {
				if hyperlinkAt(line, x, 0) == target {
					_, cmd := m.Update(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
					if cmd == nil {
						t.Fatal("wrapped link did not dispatch")
					}
					cmd()
					found = true
					break
				}
			}
		}
		m = arrow(m, tea.KeyDown)
	}
	if !found {
		t.Fatal("wrapped link not reachable")
	}
}

func TestOpenWebURLUsesSingleArgument(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "args")
	name := "xdg-open"
	if runtime.GOOS == "darwin" {
		name = "open"
	}
	script := "#!/bin/sh\nprintf '%s\\n' \"$#\" \"$1\" > \"$LINK_TEST_LOG\"\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("LINK_TEST_LOG", log)
	target := "https://example.com/42?value=$(touch%20oops)&q='quoted'"
	if err := openWebURL(target); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(log)
	if err != nil || string(got) != "1\n"+target+"\n" {
		t.Fatalf("opener args %q %v", got, err)
	}
	for _, url := range []string{"file:///tmp/a", "javascript:alert(1)", "https://example.com/\x1b]52;bad", "--help", "https:"} {
		if err := openWebURL(url); err == nil {
			t.Fatal("unsafe opener target", url)
		}
	}
	if ansi.Strip(string(got)) != string(got) {
		t.Fatal("unexpected terminal control")
	}
}
