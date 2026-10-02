package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/thomashartm/motley/internal/member"
	"github.com/thomashartm/motley/internal/state"
)

func section(row member.Row) string {
	s := row.CurrentStatus()
	if state.Attention(s) {
		return "NEEDS YOU"
	}
	if s == "dead" || s == "ended" {
		return "ENDED / DEAD"
	}
	return "WORKING"
}
func sectionOrder(row member.Row) int {
	switch section(row) {
	case "NEEDS YOU":
		return 0
	case "WORKING":
		return 1
	default:
		return 2
	}
}
func statusIcon(status string) (string, lipgloss.Color) {
	switch status {
	case "permission":
		return "⚠", lipgloss.Color("1")
	case "question":
		return "?", lipgloss.Color("3")
	case "ready":
		return "✓", lipgloss.Color("2")
	case "idle":
		return "◌", lipgloss.Color("8")
	case "working", "starting":
		return "●", lipgloss.Color("4")
	case "ended":
		return "■", lipgloss.Color("8")
	case "dead":
		return "✗", lipgloss.Color("1")
	default:
		return "○", lipgloss.Color("8")
	}
}
func since(row member.Row) string {
	if row.Since == 0 || !row.Alive {
		return "—"
	}
	return elapsed(time.Since(time.Unix(row.Since, 0)))
}

// elapsed renders a duration as whole seconds, minutes or hours; clock skew
// never shows a negative age.
func elapsed(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh", int(d.Hours()))
}
func totals(rows []member.Row) string {
	counts := map[string]int{}
	for _, r := range rows {
		s := r.CurrentStatus()
		if s == "starting" {
			s = "working"
		}
		counts[s]++
	}
	var parts []string
	for _, s := range []string{"permission", "question", "ready", "idle", "working", "ended", "dead", "alive"} {
		if counts[s] == 0 {
			continue
		}
		icon, _ := statusIcon(s)
		parts = append(parts, fmt.Sprintf("%s%d", icon, counts[s]))
	}
	return strings.Join(parts, " ")
}
func multiline(s string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = clean(lines[i])
	}
	return strings.Join(lines, "\n")
}
