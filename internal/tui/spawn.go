package tui

import (
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/thomashartm/motley/internal/blueprint"
	"github.com/thomashartm/motley/internal/config"
	"github.com/thomashartm/motley/internal/member"
)

const (
	repoStep = iota
	identityStep
	agentStep
	blueprintStep
	varsStep
	previewStep
	launchStep
)

type spawnForm struct {
	previewAction       int
	step, choice, field int
	repos               []string
	blueprints          []blueprint.Blueprint
	query               textinput.Model
	fields              []textinput.Model
	opts                member.SpawnOptions
	manualBranch        bool
	vars                []string
	plan                member.Prepared
	preview             viewport.Model
	err, progress       string
}
type spawnLoaded struct {
	cfg          config.Config
	repos        []string
	blueprints   []blueprint.Blueprint
	forBlueprint bool
	err          error
}
type spawnPrepared struct {
	plan member.Prepared
	err  error
}
type spawnProgress string
type spawnFinished struct {
	manifest member.Manifest
	err      error
}
type promptEdited struct {
	prompt string
	err    error
}
type progressWriter struct{ send func(tea.Msg) }

func (w progressWriter) Write(p []byte) (int, error) {
	if w.send != nil {
		w.send(spawnProgress(string(p)))
	}
	return len(p), nil
}

var _ io.Writer = progressWriter{}

func inputs(values ...string) []textinput.Model {
	result := make([]textinput.Model, len(values))
	for i, v := range values {
		result[i] = textinput.New()
		result[i].Prompt = ""
		result[i].CharLimit = 4096
		result[i].SetValue(v)
	}
	if len(result) > 0 {
		result[0].Focus()
	}
	return result
}
func (f *spawnForm) matches() []string {
	var r []string
	for _, v := range f.repos {
		if match(f.query.Value(), v) {
			r = append(r, v)
		}
	}
	return r
}
func branchPreview(ticket, name string) string {
	slug := func(s string) string {
		var out strings.Builder
		dash := false
		for _, r := range strings.ToLower(s) {
			if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
				if dash && out.Len() > 0 {
					out.WriteByte('-')
				}
				out.WriteRune(r)
				dash = false
			} else {
				dash = true
			}
		}
		return out.String()
	}
	part := slug(name)
	if ticket != "" {
		part = slug(ticket) + "-" + part
	}
	return "feat/" + part
}
func (m Model) beginSpawn() (tea.Model, tea.Cmd) {
	m.spawn = &spawnForm{query: inputs("")[0]}
	m.busy = true
	m.busyText = "Finding repositories…"
	m.message = ""
	return m, func() tea.Msg {
		cfg, err := config.Load()
		if err != nil {
			return spawnLoaded{err: err}
		}
		entries, err := os.ReadDir(cfg.ReposRoot)
		if err != nil {
			return spawnLoaded{err: err}
		}
		var repos []string
		for _, e := range entries {
			if _, err := member.ResolveRepo(cfg.ReposRoot, e.Name()); err == nil {
				repos = append(repos, e.Name())
			}
		}
		return spawnLoaded{cfg: cfg, repos: repos}
	}
}
func (m Model) spawnMessage(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.spawn == nil {
		return m, nil
	}
	f := m.spawn
	switch msg := msg.(type) {
	case spawnLoaded:
		m.busy = false
		m.busyText = ""
		if msg.err != nil {
			f.err = msg.err.Error()
			return m, nil
		}
		if msg.forBlueprint {
			f.blueprints = msg.blueprints
			f.step = blueprintStep
			f.choice = 0
		} else {
			m.spawnCfg = msg.cfg
			f.repos = msg.repos
			if len(f.repos) == 0 {
				f.err = "No main repositories found under " + msg.cfg.ReposRoot
			}
		}
	case spawnPrepared:
		m.busy = false
		m.busyText = ""
		if msg.err != nil {
			f.err = msg.err.Error()
			return m, nil
		}
		f.plan = msg.plan
		f.step = previewStep
		f.preview = viewport.New(m.detailWidth(), max(1, m.height-9))
		f.preview.SetContent(ansi.Hardwrap(multiline(msg.plan.Prompt), m.detailWidth(), true))
		f.err = ""
	case promptEdited:
		if msg.err != nil {
			f.err = msg.err.Error()
		} else {
			f.plan.Prompt = msg.prompt
			f.plan.HasPrompt = true
			f.preview.SetContent(ansi.Hardwrap(multiline(msg.prompt), m.detailWidth(), true))
			f.err = ""
		}
	case spawnProgress:
		f.progress += string(msg)
		if len(f.progress) > 8192 {
			f.progress = f.progress[len(f.progress)-8192:]
		}
	case spawnFinished:
		m.busy = false
		m.busyText = ""
		if msg.err != nil {
			f.err = msg.err.Error()
			f.step = previewStep
			return m, nil
		}
		m.spawn = nil
		m.query.SetValue("")
		m.group = "attention"
		m.tableFocus = false
		m.panel = listPanel
		m.focusID = msg.manifest.ID
		rows := make([]member.Row, 0, len(m.allRows)+1)
		for _, r := range m.allRows {
			if r.ID != msg.manifest.ID {
				rows = append(rows, r)
			}
		}
		m.allRows = append(rows, member.Row{Manifest: msg.manifest, Alive: true, Status: "starting", Since: msg.manifest.CreatedAt.Unix()})
		m.applyFilter()
		for i, r := range m.rows {
			if r.ID == msg.manifest.ID {
				m.selected = i
			}
		}
		m.updateDetail()
		m.message = "Created " + msg.manifest.ID
		return m, m.requestDetail(true)
	}
	return m, nil
}
func (m Model) prepareSpawn() (tea.Model, tea.Cmd) {
	f := m.spawn
	opts := f.opts
	cfg := m.spawnCfg
	if f.step == varsStep {
		opts.Vars = nil
		for i, key := range f.vars {
			opts.Vars = append(opts.Vars, key+"="+f.fields[i].Value())
		}
	}
	m.busy = true
	m.busyText = "Rendering prompt…"
	f.err = ""
	return m, func() tea.Msg { p, err := member.Prepare(cfg, opts); return spawnPrepared{p, err} }
}
func (m Model) updateSpawn(msg tea.Msg) (tea.Model, tea.Cmd) {
	f := m.spawn
	if key, ok := msg.(tea.KeyMsg); ok {
		k := key.String()
		if k == "esc" {
			m.spawn = nil
			return m, nil
		}
		if f.step == previewStep {
			switch k {
			case "right", "tab":
				f.previewAction = (f.previewAction + 1) % 3
			case "left", "shift+tab":
				f.previewAction = (f.previewAction + 2) % 3
			case "enter":
				if f.previewAction == 1 {
					return m, m.editPrompt()
				}
				if f.previewAction == 2 {
					m.spawn = nil
					return m, nil
				}
				m.busy = true
				m.busyText = "Spawning…"
				f.step = launchStep
				plan, send := f.plan, m.sendMsg
				return m, func() tea.Msg {
					created, err := member.SpawnPrepared(plan, progressWriter{send})
					return spawnFinished{created, err}
				}
			case "e":
				return m, m.editPrompt()
			case "pgup", "pgdown", "up", "down", "j", "k":
				f.preview, _ = f.preview.Update(msg)
			}
			return m, nil
		}
		if f.step == repoStep {
			switch k {
			case "up":
				f.choice = max(0, f.choice-1)
			case "down":
				f.choice = min(f.choice+1, max(0, len(f.matches())-1))
			case "enter":
				matches := f.matches()
				if len(matches) == 0 {
					return m, nil
				}
				f.opts.Repo = matches[f.choice]
				f.fields = inputs("", "", "")
				f.field = 0
				f.step = identityStep
				f.err = ""
			default:
				f.query, _ = f.query.Update(msg)
				f.choice = 0
			}
			return m, nil
		}
		if f.step == agentStep || f.step == blueprintStep {
			size := 3
			if f.step == blueprintStep {
				size = len(f.blueprints) + 1
			}
			switch k {
			case "j", "down":
				f.choice = min(f.choice+1, size-1)
			case "k", "up":
				f.choice = max(0, f.choice-1)
			case "enter":
				if f.step == agentStep {
					f.opts.Agent = []string{"claude", "codex", "opencode"}[f.choice]
					cfg, repo, agent := m.spawnCfg, f.opts.Repo, f.opts.Agent
					m.busy = true
					m.busyText = "Loading blueprints…"
					return m, func() tea.Msg {
						path, err := member.ResolveRepo(cfg.ReposRoot, repo)
						if err != nil {
							return spawnLoaded{forBlueprint: true, err: err}
						}
						bs, err := blueprint.Discover(path, repo)
						var filtered []blueprint.Blueprint
						for _, b := range bs {
							if b.Agent == agent || len(b.Args) == 0 {
								filtered = append(filtered, b)
							}
						}
						return spawnLoaded{forBlueprint: true, blueprints: filtered, err: err}
					}
				}
				if f.choice > 0 {
					b := f.blueprints[f.choice-1]
					f.opts.Blueprint = b.Name
					f.vars = b.Vars
				}
				if len(f.vars) > 0 {
					f.fields = inputs(make([]string, len(f.vars))...)
					f.field = 0
					f.step = varsStep
					return m, nil
				}
				return m.prepareSpawn()
			}
			return m, nil
		}
		if k == "tab" || k == "shift+tab" || k == "up" || k == "down" {
			f.fields[f.field].Blur()
			delta := 1
			if k == "shift+tab" || k == "up" {
				delta = -1
			}
			f.field = (f.field + delta + len(f.fields)) % len(f.fields)
			return m, f.fields[f.field].Focus()
		}
		if k == "enter" || k == "ctrl+s" {
			if f.step == varsStep {
				return m.prepareSpawn()
			}
			ticket, name, branch := f.fields[0].Value(), strings.TrimSpace(f.fields[1].Value()), f.fields[2].Value()
			if name == "" || (!f.manualBranch && (branch == "feat/" || strings.HasSuffix(branch, "-"))) {
				f.err = "Enter a name containing letters or digits."
				return m, nil
			}
			if ticket != "" {
				if err := member.CheckID(ticket); err != nil {
					f.err = err.Error()
					return m, nil
				}
			}
			f.opts.Ticket, f.opts.Name, f.opts.Branch = ticket, name, branch
			f.step = agentStep
			f.choice = 0
			f.err = ""
			return m, nil
		}
	}
	if len(f.fields) > 0 {
		old := f.fields[f.field].Value()
		var cmd tea.Cmd
		f.fields[f.field], cmd = f.fields[f.field].Update(msg)
		if f.step == identityStep && old != f.fields[f.field].Value() {
			if f.field == 2 {
				f.manualBranch = true
			}
			if !f.manualBranch {
				f.fields[2].SetValue(branchPreview(f.fields[0].Value(), f.fields[1].Value()))
			}
		}
		return m, cmd
	}
	return m, nil
}
func (m Model) editPrompt() tea.Cmd {
	f := m.spawn
	file, err := os.CreateTemp("", "motley-prompt-*.md")
	if err != nil {
		return func() tea.Msg { return promptEdited{err: err} }
	}
	path := file.Name()
	_, err = file.WriteString(blueprint.PromptHeader + f.plan.Prompt)
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(path)
		return func() tea.Msg { return promptEdited{err: err} }
	}
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	// EDITOR is the user's shell command; the prompt and path are never shell code.
	cmd := exec.Command("/bin/sh", "-c", "exec "+editor+` "$1"`, "motley-editor", path)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		defer func() { _ = os.Remove(path) }()
		if err != nil {
			return promptEdited{err: err}
		}
		prompt, err := blueprint.ReadPrompt(path)
		return promptEdited{prompt, err}
	})
}
func (m Model) spawnView(height int) string {
	f := m.spawn
	width := m.detailWidth()
	titles := []string{"Repository — type to filter", "Ticket and name", "Agent", "Blueprint", "Variables", "Prompt preview", "Launching"}
	lines := []string{"Spawn · " + titles[f.step]}
	switch f.step {
	case repoStep:
		input := f.query
		input.Width = max(1, width-2)
		lines = append(lines, input.View())
		matches := f.matches()
		start := max(0, f.choice-max(1, height-4)+1)
		for i := start; i < len(matches) && len(lines) < height-1; i++ {
			prefix := "  "
			if i == f.choice {
				prefix = "> "
			}
			lines = append(lines, prefix+clean(matches[i]))
		}
	case identityStep, varsStep:
		labels := []string{"Ticket (optional)", "Name", "Branch"}
		if f.step == varsStep {
			labels = f.vars
		}
		start := max(0, f.field-(max(1, (height-2)/2)-1))
		for i := start; i < len(f.fields) && len(lines)+2 <= height-1; i++ {
			v := f.fields[i]
			v.Width = max(1, width-2)
			prefix := ""
			if i == f.field {
				prefix = "> "
			}
			lines = append(lines, prefix+clean(labels[i]), v.View())
		}
	case agentStep, blueprintStep:
		labels := []string{"claude", "codex", "opencode"}
		if f.step == blueprintStep {
			labels = []string{"none"}
			for _, b := range f.blueprints {
				labels = append(labels, b.Name+" — "+b.Description)
			}
		}
		start := max(0, f.choice-max(1, height-3)+1)
		for i := start; i < len(labels) && len(lines) < height-1; i++ {
			prefix := "  "
			if i == f.choice {
				prefix = "> "
			}
			lines = append(lines, prefix+clean(labels[i]))
		}
	case previewStep:
		lines = append(lines, f.plan.Manifest.Repo+" @ "+f.plan.Manifest.Branch+" · "+f.plan.Manifest.Agent)
		lines = append(lines, "> "+[]string{"Launch", "Edit prompt", "Cancel"}[f.previewAction]+"  ←/→")
		v := f.preview
		v.Width = width
		v.Height = max(1, height-4)
		lines = append(lines, strings.Split(v.View(), "\n")...)
	case launchStep:
		log := strings.Split(multiline(f.progress), "\n")
		start := max(0, len(log)-(height-2))
		lines = append(lines, log[start:]...)
	}
	if f.err != "" {
		lines = append(lines, clean(f.err))
	}
	for i := range lines {
		lines[i] = fit(lines[i], width)
	}
	return strings.Join(lines[:min(len(lines), height)], "\n")
}
func (m Model) spawnKeys() string {
	switch m.spawn.step {
	case repoStep:
		return " type filter  ↑↓ select  enter next  esc cancel"
	case identityStep, varsStep:
		return " ↑↓/tab field  enter next  esc cancel"
	case agentStep, blueprintStep:
		return " ↑↓/jk select  enter next  esc cancel"
	case previewStep:
		return " ←/→ action  enter choose  ↑↓ scroll  e edit  esc cancel"
	default:
		return " Spawning…"
	}
}
