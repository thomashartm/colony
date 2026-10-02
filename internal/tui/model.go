// Package tui implements motley's terminal overview.
package tui

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/thomashartm/motley/internal/config"
	"github.com/thomashartm/motley/internal/crew"
	"github.com/thomashartm/motley/internal/member"
	"github.com/thomashartm/motley/internal/state"
	"github.com/thomashartm/motley/internal/tmux"
)

type snapshot struct {
	crews   []crew.Crew
	rows    []member.Row
	clients []tmux.Client
	err     error
}
type tick struct{}
type actionDone struct {
	err  error
	quit bool
}

type Model struct {
	overview            bool
	opening             *agentPicker
	panel, actionCursor int
	actionScroll        int
	managerActions      bool
	managerAction       int
	allRows             []member.Row
	query               textinput.Model
	searching           bool
	pickMode, sendID    string
	spawn               *spawnForm
	spawnCfg            config.Config
	sendMsg             func(tea.Msg)
	focusID             string
	importing           *importDialog

	crews                             []crew.Crew
	group                             string
	crewCursor, tableCursor           int
	tableFocus, showHidden            bool
	expanded                          map[string]bool
	manager                           bool
	managerCursor                     int
	editor                            *identityEditor
	rows                              []member.Row
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
	fetchDetail                       func(member.Row, uint64, bool) tea.Cmd
	copyText                          func(client, text string) error
	openURL                           func(string) error
	copied                            string
	detailSeq                         uint64
	event                             state.Event
	gitDetail                         string
	alert, bell                       bool
	retiring                          *retireDialog
	terminating                       *terminateDialog
	busyText                          string
}

func newModel(monitor, inside bool, client string, poll tea.Cmd) Model {
	q := textinput.New()
	q.Prompt = "/"
	q.CharLimit = 128
	return Model{query: q, monitor: monitor, inside: inside, client: client, poll: poll, detail: viewport.New(1, 1)}
}

func (m Model) Init() tea.Cmd { return m.poll }
func nextPoll() tea.Cmd       { return tea.Tick(time.Second, func(time.Time) tea.Msg { return tick{} }) }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.MouseMsg:
		return m.mouse(msg)
	case importLoaded, importDone:
		return m.importMessage(msg)
	case spawnLoaded, spawnPrepared, spawnProgress, spawnFinished, promptEdited:
		return m.spawnMessage(msg)
	case tick:
		return m, m.poll
	case linkOpened:
		if msg.err != nil {
			m.message = "Could not open link: " + msg.err.Error()
		} else {
			m.message = "Opened " + msg.url
		}
		return m, nil
	case copiedMsg:
		m.copied = ""
		if msg.err != nil {
			m.message = "Copy failed: " + msg.err.Error()
		} else {
			m.copied = msg.text
		}
		return m, nil
	case snapshot:
		if msg.err != nil {
			m.pollError = msg.err.Error()
			return m, nextPoll()
		}
		m.pollError = ""
		id := m.selectedID()
		crewKey := m.currentEntry().key()
		old := make(map[string]member.Row, len(m.rows))
		for _, r := range m.allRows {
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
		m.allRows, m.clients, m.loaded = msg.rows, msg.clients, true
		m.rows = nil
		for _, r := range msg.rows {
			if match(m.query.Value(), strings.Join([]string{r.ID, r.Name, r.Ticket, r.Repo, r.Branch}, " ")) {
				m.rows = append(m.rows, r)
			}
		}
		m.crews = msg.crews
		m.sortRows()
		if len(m.rows) == 0 {
			m.overview = true
		}
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
		m.actionCursor = max(0, min(m.actionCursor, len(m.actions())-1))
		m.managerCursor = max(0, min(m.managerCursor, len(m.crews)-1))
		if m.focusID != "" {
			for i, r := range m.rows {
				if r.ID == m.focusID {
					m.selected = i
					m.overview = false
					m.focusID = ""
					break
				}
			}
		}
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
		m.detail.Width, m.detail.Height = m.detailWidth(), m.contentHeight()
		if m.spawn != nil && m.spawn.step == previewStep {
			m.spawn.preview.Width = m.detailWidth()
			m.spawn.preview.Height = max(1, m.contentHeight()-4)
			m.spawn.preview.SetContent(ansi.Hardwrap(multiline(m.spawn.plan.Prompt), m.detailWidth(), true))
		}
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
			m.message = "Saved"
			if m.editor != nil && m.editor.kind == "reply" {
				m.message = "Reply sent"
			}
			m.editor = nil
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
		m.terminating = nil
		m.busy = false
		m.busyText = ""
		m.retiring = nil
		if msg.err != nil {
			m.message = msg.err.Error()
		} else {
			m.message = msg.action + " " + msg.id
		}
		return m, m.poll
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
		if m.opening != nil {
			return m.updateAgentPicker(key)
		}
		if m.spawn != nil {
			return m.updateSpawn(msg)
		}
		if m.importing != nil {
			return m.updateImport(key)
		}
		if m.searching {
			return m.updateFilter(msg)
		}
		if m.editor != nil {
			return m.updateEditor(msg)
		}
		if m.manager {
			return m.updateManager(key)
		}
		if m.terminating != nil {
			return m.updateTerminate(key)
		}
		if m.retiring != nil {
			return m.updateRetire(key)
		}
		if m.picking {
			if m.pickMode == "send" {
				return m.updateSendPicker(key)
			}
			return m.updatePicker(key)
		}
		if next, cmd, handled := m.navigationKey(msg); handled {
			return next, cmd
		}
		if key == "esc" && m.query.Value() != "" && (m.group != "crew" || !m.tableFocus) {
			m.query.SetValue("")
			m.applyFilter()
			return m, m.requestDetail(true)
		}
		if next, cmd, handled := m.groupingKey(key); handled {
			return next, cmd
		}
		switch key {
		case "a":
			return m.beginImport()
		case "c":
			return m.copyMessage()
		case "s":
			return m.beginSpawn()
		case "i":
			return m.beginReply()
		case "t":
			return m.beginSend()
		case "/":
			m.searching = true
			return m, m.query.Focus()
		case "esc":
			m.query.SetValue("")
			m.applyFilter()
			return m, m.requestDetail(true)
		case "m":
			m.manager = true
			m.managerCursor = 0
			return m, nil
		case "e":
			return m.editMember()
		case "q", "ctrl+c":
			if !m.monitor {
				return m, tea.Quit
			}
			// Detach the most recently active monitor client, keeping the overview alive.
			if own := m.activeMonitorClient(); own != "" {
				m.busy = true
				return m, func() tea.Msg { return actionDone{err: tmux.DetachClient(own)} }
			}
		case "j", "down":
			if m.overview {
				m.selectRow(0)
			} else {
				m.selectRow(min(m.selected+1, len(m.rows)-1))
			}
		case "k", "up":
			if m.overview || m.selected == 0 {
				m.selectOverview()
			} else {
				m.selectRow(m.selected - 1)
			}
		case "pgdown", "pgup":
			m.detail, _ = m.detail.Update(msg)
		case "d":
			return m.beginTerminate()
		case "x":
			return m.beginRetire()
		case "r":
			return m.beginRevive()
		case "p":
			if m.monitor {
				m.pickMode = "pin"
				m.choices, m.choice, m.picking = workClients(m.clients), 0, true
				for i, c := range m.choices {
					if c.Name == m.pinned {
						m.choice = i + 1
					}
				}
			}
		case "enter", "o":
			if m.overview && key == "enter" {
				m.panel, m.actionCursor, m.actionScroll = actionsPanel, 0, 0
				return m, nil
			}
			m.alert = false
			return m.jump()
		}
		if previousID != m.selectedID() {
			m.alert = false
			return m, m.requestDetail(true)
		}
	}
	if m.spawn != nil && !m.busy {
		return m.updateSpawn(msg)
	}
	if m.searching {
		return m.updateFilter(msg)
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
	if index >= 0 && index < len(m.rows) && (index != m.selected || m.overview) {
		m.overview = false
		m.selected = index
		m.message = ""
		m.detail.GotoTop()
		m.updateDetail()
	}
}
func (m Model) selectedID() string {
	if m.overview {
		return ""
	}
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
		return tmux.Client{}, fmt.Errorf("open another tab and run motley attach <id> to use it as the work tab")
	}
	return work[0], nil
}

// A separate work tab stays the preferred destination. With a single monitor
// tab, opening a member uses that tab and keeps the monitor session alive.
func openClient(clients []tmux.Client, pinned string) (tmux.Client, error) {
	if pinned != "" || len(workClients(clients)) > 0 {
		return WorkClient(clients, pinned)
	}
	var monitor tmux.Client
	for _, client := range clients {
		if client.Session != tmux.MonitorSession {
			continue
		}
		if monitor.Name != "" {
			return tmux.Client{}, fmt.Errorf("multiple monitor tabs are open; choose a work tab with t")
		}
		monitor = client
	}
	if monitor.Name == "" {
		return tmux.Client{}, fmt.Errorf("monitor tab is no longer attached")
	}
	return monitor, nil
}

func (m Model) jump() (tea.Model, tea.Cmd) {
	id := m.selectedID()
	if id == "" {
		return m.beginAgentPicker()
	}
	if !m.selectedRow().Alive {
		m.message = "This member is dead; its tmux session is not running."
		return m, nil
	}
	if m.selectedRow().External {
		m.busy = true
		return m, func() tea.Msg { return actionDone{err: member.ExternalTerminal(id)} }
	}
	if !m.inside && !m.monitor {
		m.attachID = id
		return m, tea.Quit
	}
	client := m.client
	if m.monitor {
		target, err := openClient(m.clients, m.pinned)
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
			target, err := openClient(clients, pinned)
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

// The list and details split evenly; the list stops at 100 columns, beyond
// which member rows gain nothing and details need the room.
func (m Model) listWidth() int   { return max(20, min(100, m.width/2)) }
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
func (m *Model) updateDetail() {
	if m.selectedID() == "" {
		m.detail.SetContent("Overview\n\nSpawn or add agents, open a running agent, and manage crews.\n\nPress Enter or 3 for main actions.\nSelect a member for its details and actions.")
		return
	}
	m.detail.SetContent(m.memberDetails())
}

func (m Model) View() string {
	if m.width == 0 {
		return "Loading motley…"
	}
	if m.width < 60 || m.height < 10 {
		return "motley needs a terminal of at least 60 × 10.\nResize the terminal; q to quit."
	}
	alive := 0
	for _, row := range m.rows {
		if row.Alive {
			alive++
		}
	}
	var crewMark string
	for _, color := range []lipgloss.Color{"6", "5", "3"} {
		crewMark += lipgloss.NewStyle().Foreground(color).Render("▌")
	}
	header := " " + crewMark + " " + lipgloss.NewStyle().Bold(true).Render("MOTLEY")
	header += " " + lipgloss.NewStyle().Foreground(lipgloss.Color("3")).Render(`\m/_`)
	if m.monitor {
		header += " monitor"
	}
	header += fmt.Sprintf("  %d alive · %d dead  %s", alive, len(m.rows)-alive, totals(m.rows))
	if m.monitor {
		if m.alert {
			header += "  ! NEW ATTENTION"
		}
		if target, err := WorkClient(m.clients, m.pinned); err == nil {
			header += "  work: " + clean(target.Name)
			if m.pinned != "" {
				header += " (pinned)"
			}
		} else {
			header += "  Open agent: this tab"
		}
	}
	height, width := m.contentHeight(), m.listWidth()
	list := m.listView(height, width)
	right := m.detail.View()
	if !m.overview && m.group == "crew" && m.currentEntry().id == "" {
		right = m.crewTable(height, m.detailWidth())
	}
	if m.panel == actionsPanel {
		right = m.actionsView(height)
	}
	if m.opening != nil {
		right = m.agentPickerView(height)
	}
	if m.importing != nil {
		right = m.importView(height)
	}
	if m.picking {
		right = m.pickerView(height)
	}
	if m.terminating != nil {
		right = m.terminateView(height)
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
	if m.spawn != nil {
		right = m.spawnView(height)
	}
	border := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("8"))
	leftBorder, rightBorder := border, border
	if m.panel == listPanel && m.editor == nil && !m.manager && m.spawn == nil && m.retiring == nil && m.terminating == nil && m.importing == nil && m.opening == nil && !m.picking {
		leftBorder = leftBorder.BorderForeground(lipgloss.Color("6"))
	} else {
		rightBorder = rightBorder.BorderForeground(lipgloss.Color("6"))
	}
	left := leftBorder.Width(width).Height(m.panelHeight()).Render(m.panelHeading(list, width))
	detail := rightBorder.Width(m.detailWidth()).Height(m.panelHeight()).Render(m.panelHeading(right, m.detailWidth()))
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
	if m.query.Value() != "" {
		header += "  /" + m.query.Value()
	}
	if m.searching {
		message = m.query.View()
	}
	header += "  [" + m.groupName() + "]"
	return fit(header, m.width) + "\n" + lipgloss.JoinHorizontal(lipgloss.Top, left, detail) + "\n" + m.messageLine(message) + "\n" + m.footer()
}
func (m Model) pickerView(height int) string {
	labels := []string{"Automatic — most recently active work tab"}
	if m.pickMode == "send" {
		labels = nil
	}
	for _, c := range m.choices {
		labels = append(labels, clean(c.Name)+" · "+clean(c.Session))
	}
	title := "Pin work tab"
	if m.pickMode == "send" {
		title = "Send " + m.sendID + " to tab"
	}
	lines := []string{fit(title, m.detailWidth()), ""}
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
