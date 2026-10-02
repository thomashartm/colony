package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/thomashartm/motley/internal/member"
)

func TestPanelSplit(t *testing.T) {
	for width, want := range map[int][2]int{60: {30, 25}, 80: {40, 35}, 120: {60, 55}, 200: {100, 95}, 240: {100, 135}} {
		m := update(newModel(false, true, "client", nil), tea.WindowSizeMsg{Width: width, Height: 24})
		m = update(m, snapshot{rows: []member.Row{row("a-member-with-a-long-name", true)}})
		if got := [2]int{m.listWidth(), m.detailWidth()}; got != want {
			t.Fatalf("width %d: list|detail %v want %v", width, got, want)
		}
		for i, line := range strings.Split(m.View(), "\n") {
			if w := ansi.StringWidth(line); w > width {
				t.Fatalf("width %d: line %d is %d wide: %q", width, i, w, line)
			}
		}
	}
}
