package tui

import (
	"context"
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
	client := m.github
	return m, func() tea.Msg {
		c, err := member.InspectRetire(id)
		if err == nil && client != nil {
			ctx, cancel := context.WithTimeout(context.Background(), member.RetireLookupTimeout)
			c.OpenPR = member.OpenPR(ctx, client, c.Manifest)
			cancel()
		}
		return retireChecked{id: id, check: c, err: err}
	}
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
		dialog.focus = (dialog.focus + len(m.retireChoices()) - 1) % len(m.retireChoices())
		return m, nil
	case "down", "tab":
		dialog.focus = (dialog.focus + 1) % len(m.retireChoices())
		return m, nil
	case "enter":
		keys := []string{"y", "f", "k", "esc"}
		if dialog.check.Manifest.ClaudeSession != "" {
			keys = []string{"y", "esc"}
		}
		key = keys[dialog.focus]
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
	} else if d.check.Manifest.ClaudeSession != "" {
		lines = append(lines, "Stops the imported agent and archives its Motley entry.", "Keeps the checkout, files and all branches.", clean(d.check.Manifest.Worktree))
	} else {
		dirty := "no"
		if d.check.Dirty {
			dirty = "YES"
		}
		lines = append(lines, "Dirty/untracked files: "+dirty, fmt.Sprintf("Unpushed commits: %d", d.check.Ahead), fmt.Sprintf("Force: %t · keep branch: %t", d.force, d.keep))
		if pr := d.check.OpenPR; pr != nil {
			lines = append(lines, clean(fmt.Sprintf("Open PR #%d stays open on GitHub: %s", pr.Number, pr.URL)))
		}
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
	labels := m.retireChoices()
	wrapped = wrapped[:min(len(wrapped), max(1, height-len(labels)))]
	for i, label := range labels {
		wrapped = append(wrapped, fit(control(label, d.focus == i), m.detailWidth()))
	}
	return strings.Join(wrapped[:min(len(wrapped), height)], "\n")
}

func (m Model) retireChoices() []string {
	d := m.retiring
	if d.check.Manifest.ClaudeSession != "" {
		return []string{"Confirm retirement (keep files)", "Cancel"}
	}
	return []string{"Confirm retirement", fmt.Sprintf("Force: %t", d.force), fmt.Sprintf("Keep branch: %t", d.keep), "Cancel"}
}
