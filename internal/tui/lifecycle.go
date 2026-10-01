package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/thomashartm/motley/internal/member"
	"github.com/thomashartm/motley/internal/worktree"
)

type retireDialog struct {
	focus               int
	id                  string
	check               member.RetireCheck
	err                 error
	loaded, force, keep bool
}
type retireChecked struct {
	id    string
	check member.RetireCheck
	err   error
}
type lifecycleDone struct {
	id, action string
	err        error
}

func (m Model) beginRetire() (tea.Model, tea.Cmd) {
	id := m.selectedID()
	if id == "" {
		return m, nil
	}
	m.retiring = &retireDialog{id: id}
	m.busy = true
	m.busyText = "Checking retirement…"
	return m, func() tea.Msg { c, err := member.InspectRetire(id); return retireChecked{id: id, check: c, err: err} }
}
func (m Model) beginRevive() (tea.Model, tea.Cmd) {
	id := m.selectedID()
	if id == "" {
		return m, nil
	}
	if m.selectedRow().Alive {
		m.message = "Revive requires a dead member; this session is still alive."
		return m, nil
	}
	m.busy = true
	m.busyText = "Reviving…"
	return m, func() tea.Msg { return lifecycleDone{id: id, action: "Revived", err: member.Revive(id)} }
}
func (m Model) updateRetire(key string) (tea.Model, tea.Cmd) {
	// Copy the dialog so model snapshots remain values, as elsewhere in Bubble Tea.
	dialog := *m.retiring
	m.retiring = &dialog
	switch key {
	case "up", "shift+tab":
		dialog.focus = (dialog.focus + 3) % 4
		return m, nil
	case "down", "tab":
		dialog.focus = (dialog.focus + 1) % 4
		return m, nil
	case "enter":
		key = []string{"y", "f", "k", "esc"}[dialog.focus]
	}
	switch key {
	case "esc", "q":
		m.retiring = nil
		m.message = ""
	case "f":
		dialog.force = !dialog.force
		m.message = ""
	case "k":
		dialog.keep = !dialog.keep
		m.message = ""
	case "enter", "y":
		if !dialog.loaded || dialog.err != nil {
			return m, nil
		}
		if len(dialog.check.Risks()) > 0 && !dialog.force {
			m.message = "Work would be discarded. Select Force to enable it, or Esc to cancel."
			return m, nil
		}
		m.busy = true
		m.busyText = "Retiring…"
		m.message = ""
		return m, func() tea.Msg {
			return lifecycleDone{id: dialog.id, action: "Retired", err: member.Retire(dialog.id, dialog.force, dialog.keep)}
		}
	}
	return m, nil
}
func (m Model) retireView(height int) string {
	d := m.retiring
	lines := []string{"Retire " + clean(d.id) + "?"}
	if !d.loaded {
		lines = append(lines, "Checking worktree and commits…")
	} else if d.err != nil {
		lines = append(lines, "Cannot retire:", clean(d.err.Error()))
	} else {
		dirty := "no"
		if d.check.Dirty {
			dirty = "YES"
		}
		lines = append(lines, "Dirty/untracked files: "+dirty, fmt.Sprintf("Unpushed commits: %d", d.check.Ahead), fmt.Sprintf("Force: %t · keep branch: %t", d.force, d.keep))
		branch := d.check.Manifest.Branch
		keep := d.keep || worktree.Protected(branch)
		action := "delete"
		if keep {
			action = "keep"
		}
		if branch == "" {
			action = "none (detached)"
		}
		lines = append(lines, "Local branch: "+action, "", "Removes the tmux session and worktree:", clean(d.check.Manifest.Worktree), "Archives its manifest and history.", "Remote branches are kept.")
	}
	wrapped := strings.Split(ansi.Hardwrap(strings.Join(lines, "\n"), m.detailWidth(), true), "\n")
	wrapped = wrapped[:min(len(wrapped), max(1, height-4))]
	labels := []string{"Confirm retirement", fmt.Sprintf("Force: %t", d.force), fmt.Sprintf("Keep branch: %t", d.keep), "Cancel"}
	for i, label := range labels {
		wrapped = append(wrapped, fit(control(label, d.focus == i), m.detailWidth()))
	}
	return strings.Join(wrapped[:min(len(wrapped), height)], "\n")
}
