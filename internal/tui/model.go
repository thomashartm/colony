// Package tui implements colony's terminal overview.
package tui

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/thomashartm/colony/internal/crew"
	"github.com/thomashartm/colony/internal/minion"
	"github.com/thomashartm/colony/internal/state"
	"github.com/thomashartm/colony/internal/tmux"
)

type snapshot struct {
	crews   []crew.Crew
	rows    []minion.Row
	clients []tmux.Client
	err     error
}
type tick struct{}
type actionDone struct {
	err  error
	quit bool
}

type Model struct {
	crews                             []crew.Crew
	group                             string
	crewCursor, tableCursor           int
	tableFocus, showHidden            bool
	expanded                          map[string]bool
	manager                           bool
	managerCursor                     int
	editor                            *identityEditor
	rows                              []minion.Row
	clients                           []tmux.Client
	selected, width, height           int
	detail                            viewport.Model
	monitor, inside, busy, loaded     bool
	client, pinned, message, attachID string
	pollError                         string
	picking                           bool
	choices                           []tmux.Client
	choice                            int
	poll                              tea.Cmd
	fetchDetail                       func(minion.Row, uint64, bool) tea.Cmd
	detailSeq                         uint64
	event                             state.Event
	gitDetail                         string
	alert, bell                       bool
	retiring                          *retireDialog
	busyText                          string
}

func newModel(monitor, inside bool, client string, poll tea.Cmd) Model {
	return Model{monitor: monitor, inside: inside, client: client, poll: poll, detail: viewport.New(1, 1)}
}

func (m Model) Init() tea.Cmd { return m.poll }
func nextPoll() tea.Cmd       { return tea.Tick(time.Second, func(time.Time) tea.Msg { return tick{} }) }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tick:
		return m, m.poll
	case snapshot:
		if msg.err != nil {
			m.pollError = msg.err.Error()
			return m, nextPoll()
		}
		m.pollError = ""
		id := m.selectedID()
		crewKey := m.currentEntry().key()
		old := make(map[string]minion.Row, len(m.rows))
		for _, r := range m.rows {
			old[r.ID] = r
		}
		ring := false
		if m.loaded && m.monitor {
			for _, r := range msg.rows {
				prev := old[r.ID]
				s := r.CurrentStatus()
				if state.Attention(s) && (!state.Attention(prev.CurrentStatus()) || (s == prev.CurrentStatus() && r.Since != prev.Since)) {
					m.alert = true
					ring = m.bell
				}
			}
		}
		m.rows, m.clients, m.loaded = msg.rows, msg.clients, true
		m.crews = msg.crews
		m.sortRows()
		for i, row := range m.rows {
			if row.ID == id {
				m.selected = i
				break
			}
		}
		m.selected = max(0, min(m.selected, len(m.rows)-1))
		if m.group == "crew" {
			m.restoreCrewSelection(crewKey, id)
		}
		m.managerCursor = max(0, min(m.managerCursor, len(m.crews)-1))
		m.updateDetail()
		cmd := m.requestDetail(id != m.selectedID())
		var bell tea.Cmd
		if ring {
			bell = func() tea.Msg { _, _ = fmt.Fprint(os.Stdout, "\a"); return nil }
		}
		return m, tea.Batch(nextPoll(), cmd, bell)
	case detailMsg:
		if msg.id == m.selectedID() && msg.seq == m.detailSeq {
			m.event, m.gitDetail = msg.event, msg.git
			if msg.err != nil {
				m.message = msg.err.Error()
			}
			m.updateDetail()
		}
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.detail.Width, m.detail.Height = m.detailWidth(), max(1, m.height-5)
		m.updateDetail()
	case identitySaved:
		crewKey, id := m.currentEntry().key(), m.selectedID()
		m.busy = false
		m.busyText = ""
		if msg.err != nil {
			if m.editor != nil {
				m.editor.err = msg.err.Error()
			}
			m.message = msg.err.Error()
		} else {
			m.editor = nil
			m.message = "Saved"
		}
		if msg.crews != nil || msg.err == nil {
			m.crews = msg.crews
		}
		m.managerCursor = max(0, min(m.managerCursor, len(m.crews)-1))
		m.restoreCrewSelection(crewKey, id)
		m.updateDetail()
	case retireChecked:
		m.busy = false
		m.busyText = ""
		if m.retiring != nil && m.retiring.id == msg.id {
			m.retiring.check = msg.check
			m.retiring.err = msg.err
			m.retiring.loaded = true
		}
	case lifecycleDone:
		m.busy = false
		m.busyText = ""
		m.retiring = nil
		if msg.err != nil {
			m.message = msg.err.Error()
		} else {
			m.message = msg.action + " " + msg.id
		}
	case actionDone:
		m.busy = false
		m.message = ""
		if msg.err != nil {
			m.message = msg.err.Error()
		} else if msg.quit {
			return m, tea.Quit
		}
	case tea.KeyMsg:
		previousID := m.selectedID()
		if m.busy {
			return m, nil
		}
		key := msg.String()
		if m.editor != nil {
			return m.updateEditor(msg)
		}
		if m.manager {
			return m.updateManager(key)
		}
		if m.retiring != nil {
			return m.updateRetire(key)
		}
		if m.picking {
			return m.updatePicker(key)
		}
		if next, cmd, handled := m.groupingKey(key); handled {
			return next, cmd
		}
		switch key {
		case "G":
			m.manager = true
			m.managerCursor = 0
			return m, nil
		case "e":
			return m.editMinion()
		case "q", "ctrl+c":
			if !m.monitor {
				return m, tea.Quit
			}
			// Detach the most recently active monitor client, keeping the overview alive.
			var own tmux.Client
			for _, c := range m.clients {
				if c.Session == tmux.MonitorSession && (own.Name == "" || c.Activity > own.Activity) {
					own = c
				}
			}
			if own.Name != "" {
				m.busy = true
				return m, func() tea.Msg { return actionDone{err: tmux.DetachClient(own.Name)} }
			}
		case "j", "down":
			m.selectRow(min(m.selected+1, len(m.rows)-1))
		case "k", "up":
			m.selectRow(max(0, m.selected-1))
		case "pgdown", "pgup":
			m.detail, _ = m.detail.Update(msg)
		case "x":
			return m.beginRetire()
		case "r":
			return m.beginRevive()
		case "T":
			if m.monitor {
				m.choices, m.choice, m.picking = workClients(m.clients), 0, true
				for i, c := range m.choices {
					if c.Name == m.pinned {
						m.choice = i + 1
					}
				}
			}
		case "enter":
			m.alert = false
			return m.jump()
		}
		if previousID != m.selectedID() {
			m.alert = false
			return m, m.requestDetail(true)
		}
	}
	if m.editor != nil && !m.busy {
		return m.updateEditor(msg)
	}
	return m, nil
}

func (m *Model) requestDetail(force bool) tea.Cmd {
	if force {
		m.event = state.Event{}
		m.gitDetail = ""
		m.updateDetail()
	}
	if m.fetchDetail == nil || m.selectedID() == "" {
		return nil
	}
	m.detailSeq++
	return m.fetchDetail(m.selectedRow(), m.detailSeq, force)
}

func (m *Model) selectRow(index int) {
	if index >= 0 && index != m.selected {
		m.selected = index
		m.message = ""
		m.detail.GotoTop()
		m.updateDetail()
	}
}
func (m Model) selectedID() string {
	if m.group == "crew" {
		e := m.currentEntry()
		if m.tableFocus {
			members := m.members(e.crew)
			if m.tableCursor >= 0 && m.tableCursor < len(members) {
				return members[m.tableCursor].ID
			}
			return ""
		}
		return e.id
	}
	if m.selected < 0 || m.selected >= len(m.rows) {
		return ""
	}
	return m.rows[m.selected].ID
}
func workClients(clients []tmux.Client) []tmux.Client {
	var work []tmux.Client
	for _, c := range clients {
		if c.Session != tmux.MonitorSession {
			work = append(work, c)
		}
	}
	sort.SliceStable(work, func(i, j int) bool {
		if work[i].Activity != work[j].Activity {
			return work[i].Activity > work[j].Activity
		}
		return work[i].Name < work[j].Name
	})
	return work
}

// WorkClient never selects a client displaying a monitor, even when pinned.
func WorkClient(clients []tmux.Client, pinned string) (tmux.Client, error) {
	work := workClients(clients)
	if pinned != "" {
		for _, c := range work {
			if c.Name == pinned {
				return c, nil
			}
		}
		return tmux.Client{}, fmt.Errorf("pinned work tab is unavailable; press T to choose a work tab")
	}
	if len(work) == 0 {
		return tmux.Client{}, fmt.Errorf("open another tab and run colony attach <id> to use it as the work tab")
	}
	return work[0], nil
}
func (m Model) jump() (tea.Model, tea.Cmd) {
	id := m.selectedID()
	if id == "" {
		return m, nil
	}
	if !m.selectedRow().Alive {
		m.message = "This minion is dead; its tmux session is not running."
		return m, nil
	}
	if !m.inside && !m.monitor {
		m.attachID = id
		return m, tea.Quit
	}
	client := m.client
	if m.monitor {
		target, err := WorkClient(m.clients, m.pinned)
		if err != nil {
			m.message = err.Error()
			return m, nil
		}
		client = target.Name
	}
	m.busy = true
	monitor, pinned := m.monitor, m.pinned
	return m, func() tea.Msg {
		// Recheck the destination immediately before switching: clients may have
		// detached or moved into the monitor since the last one-second snapshot.
		clients, err := tmux.Clients()
		if err != nil {
			return actionDone{err: err}
		}
		if monitor {
			target, err := WorkClient(clients, pinned)
			if err != nil {
				return actionDone{err: err}
			}
			client = target.Name
		} else {
			found := false
			for _, c := range clients {
				if c.Name == client {
					found = true
				}
			}
			if !found {
				return actionDone{err: fmt.Errorf("the originating tmux client is no longer attached")}
			}
		}
		return actionDone{err: tmux.SwitchClient(client, id), quit: !monitor}
	}
}
func (m Model) updatePicker(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "q":
		m.picking = false
	case "down", "j":
		m.choice = min(m.choice+1, len(m.choices))
	case "up", "k":
		m.choice = max(0, m.choice-1)
	case "enter":
		m.pinned = ""
		if m.choice > 0 {
			m.pinned = m.choices[m.choice-1].Name
		}
		m.picking, m.message = false, ""
	}
	return m, nil
}

func (m Model) listWidth() int   { return max(20, min(42, m.width/3)) }
func (m Model) detailWidth() int { return max(1, m.width-m.listWidth()-5) }
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, ansi.Strip(s))
}
func fit(s string, width int) string { return ansi.Truncate(s, max(0, width), "…") }
func field(label, value string) string {
	if value == "" {
		value = "—"
	}
	return label + "  " + clean(value)
}
func (m *Model) updateDetail() {
	if m.selectedID() == "" {
		m.detail.SetContent("Select a minion to see its details.")
		return
	}
	r := m.selectedRow()
	status := r.CurrentStatus()
	var clients []string
	for _, c := range m.clients {
		if c.Session == tmux.SessionName(r.ID) {
			clients = append(clients, c.Name)
		}
	}
	body := strings.Join([]string{
		colored("▌ "+clean(r.Name), minion.Color(r.Manifest, m.crews)), "", field("ID", r.ID), field("Status", status+" · "+since(r)), field("Ticket", r.Ticket),
		field("Repo", r.Repo), field("Branch", r.Branch), field("Base", r.Base), field("Agent", r.Agent) + " · " + coloredBadge(r.Agent),
		"Crew  " + m.crewLabel(r.Crew),
		"", field("Worktree", r.Worktree), field("Main repo", r.RepoPath), field("Remote", r.RemoteURL),
		field("Created", r.CreatedAt.Local().Format("2006-01-02 15:04 MST")), field("Tabs", strings.Join(clients, ", ")),
	}, "\n")
	if m.event.Status == status {
		text := m.event.Summary
		switch status {
		case "question":
			if q := m.event.Detail["question"]; q != "" {
				text = q
			}
		case "permission":
			if tool := m.event.Detail["tool"]; tool != "" {
				text = tool + ": " + m.event.Detail["input"] + "\n\n" + text
			}
		}
		if text != "" {
			body = multiline(text) + "\n\n────────────────────\n" + body
		}
	}
	if status == "ready" && m.gitDetail != "" {
		body += "\n\n" + multiline(m.gitDetail)
	}
	m.detail.SetContent(ansi.Hardwrap(body, m.detail.Width, true))
}

func (m Model) View() string {
	if m.width == 0 {
		return "Loading colony…"
	}
	if m.width < 60 || m.height < 10 {
		return "colony needs a terminal of at least 60 × 10.\nResize the terminal; q to quit."
	}
	alive := 0
	for _, row := range m.rows {
		if row.Alive {
			alive++
		}
	}
	header := fmt.Sprintf(" colony  %d alive · %d dead", alive, len(m.rows)-alive)
	header += "  " + totals(m.rows)
	if m.monitor {
		header = fmt.Sprintf(" colony monitor  %d alive · %d dead", alive, len(m.rows)-alive)
		header += "  " + totals(m.rows)
		if m.alert {
			header += "  ! NEW ATTENTION"
		}
		if target, err := WorkClient(m.clients, m.pinned); err == nil {
			header += "  work: " + clean(target.Name)
			if m.pinned != "" {
				header += " (pinned)"
			}
		} else {
			header += "  no work tab"
		}
	}
	height, width := max(1, m.height-5), m.listWidth()
	list := m.listView(height, width)
	right := m.detail.View()
	if m.group == "crew" && m.currentEntry().id == "" {
		right = m.crewTable(height, m.detailWidth())
	}
	if m.picking {
		right = m.pickerView(height)
	}
	if m.retiring != nil {
		right = m.retireView(height)
	}
	if m.manager {
		right = m.managerView(height)
	}
	if m.editor != nil {
		right = m.editorView(height)
	}
	border := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("8"))
	left := border.Width(width).Height(height).Render(list)
	detail := border.Width(m.detailWidth()).Height(height).Render(right)
	message := m.message
	if m.pollError != "" {
		message = m.pollError
	}
	if m.busy {
		message = "Switching…"
		if m.busyText != "" {
			message = m.busyText
		}
	}
	if !m.loaded && message == "" {
		message = "Loading…"
	}
	keys := " ↑↓/jk select  enter jump  x retire  r revive  e edit  g group  G crews  pgup/pgdn detail  q quit"
	if m.monitor {
		keys = " ↑↓/jk select  enter jump  x retire  r revive  e edit  g group  G crews  T pin  q detach"
	}
	if m.group == "crew" {
		keys = " tab members  →/space expand  ← collapse  H hidden  g group  G crews  q quit"
		if m.tableFocus {
			keys = " ↑↓/jk member  enter jump  x retire  r revive  e edit  esc list"
		}
	}
	if m.picking {
		keys = " ↑↓/jk select work tab  enter pin  esc cancel"
	}
	if m.retiring != nil {
		keys = " y/enter confirm  f force  k keep branch  esc cancel"
	}
	if m.manager {
		keys = " a add  e edit  c colour  x delete  esc back"
	}
	if m.editor != nil {
		keys = " tab field  ctrl+s save  esc cancel"
		if m.editor.kind == "delete" {
			keys = " y delete  f force unassign  esc cancel"
		}
	}
	header += "  [" + m.groupName() + "]"
	return fit(header, m.width) + "\n" + lipgloss.JoinHorizontal(lipgloss.Top, left, detail) + "\n" + fit(clean(message), m.width) + "\n" + fit(keys, m.width)
}
func (m Model) listView(height, width int) string {
	if m.group == "crew" {
		return m.crewList(height, width)
	}
	if len(m.rows) == 0 {
		return "No minions yet.\n\ncolony spawn --help"
	}
	var lines []string
	selectedLine := 0
	last := ""
	for i, r := range m.rows {
		group := section(r)
		if m.group == "repo" {
			group = clean(r.Repo)
		}
		if group != last {
			lines = append(lines, group)
			last = group
		}
		line := m.minionLine(r, width)
		style := lipgloss.NewStyle()
		if i == m.selected {
			style = style.Reverse(true)
			selectedLine = len(lines)
		}
		lines = append(lines, style.Render(line))
	}
	start := max(0, selectedLine-height+1)
	return strings.Join(lines[start:min(len(lines), start+height)], "\n")
}
func (m Model) pickerView(height int) string {
	labels := []string{"Automatic — most recently active work tab"}
	for _, c := range m.choices {
		labels = append(labels, clean(c.Name)+" · "+clean(c.Session))
	}
	lines := []string{"Pin work tab", ""}
	start := max(0, m.choice-max(1, height-2)+1)
	for i := start; i < len(labels) && len(lines) < height; i++ {
		label := "  " + labels[i]
		if i == m.choice {
			label = "> " + labels[i]
		}
		lines = append(lines, fit(label, m.detailWidth()))
	}
	return strings.Join(lines, "\n")
}
