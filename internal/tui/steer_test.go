package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/thomashartm/motley/internal/agents"
	"github.com/thomashartm/motley/internal/blueprint"
	"github.com/thomashartm/motley/internal/member"
	"github.com/thomashartm/motley/internal/tmux"
)

func TestFilterPreservesSelectionAndRefresh(t *testing.T) {
	m := update(newModel(false, true, "client", nil), tea.WindowSizeMsg{Width: 120, Height: 30})
	a, b := row("alpha", true), row("fx-cache", true)
	b.Ticket = "412"
	b.Repo = "banking"
	m = update(m, snapshot{rows: []member.Row{a, b}})
	m = update(m, key("j"))
	m = update(m, key("/"))
	m = update(m, key("FXC"))
	if len(m.rows) != 1 || m.selectedID() != "fx-cache" {
		t.Fatal(m.rows, m.selectedID())
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyEnter})
	b.Status = "permission"
	m = update(m, snapshot{rows: []member.Row{a, b}})
	if m.selectedID() != "fx-cache" || len(m.rows) != 1 {
		t.Fatal("refresh lost filter")
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyEsc})
	if len(m.rows) != 2 || m.selectedID() != "fx-cache" {
		t.Fatal("clear lost selection")
	}
	for _, query := range []string{"412", "bank", "feat/fxc"} {
		m.query.SetValue(query)
		m.applyFilter()
		if len(m.rows) != 1 {
			t.Fatal(query)
		}
	}
	m.query.SetValue("no-such-member")
	m.applyFilter()
	if m.selectedID() != "" || !strings.Contains(m.View(), "No matches") {
		t.Fatal("empty filter")
	}
}
func TestReplyAndSendModel(t *testing.T) {
	m := update(newModel(true, true, "monitor", nil), snapshot{rows: []member.Row{row("a", true)}, clients: []tmux.Client{{Name: "monitor", TTY: "/dev/monitor", Session: "_motley"}, {Name: "work", TTY: "/dev/work", Session: "shell"}}})
	m = update(m, key("t"))
	if !m.picking || len(m.choices) != 1 || m.choices[0].Name != "work" {
		t.Fatal("unsafe tab picker")
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyEsc})
	m = update(m, key("i"))
	if m.editor == nil || m.editor.kind != "reply" || m.editor.id != "a" {
		t.Fatal("reply editor")
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyEsc})
	m.rows[0].Status = "permission"
	m = update(m, key("i"))
	if m.editor != nil || !strings.Contains(m.message, "Permission") {
		t.Fatal("permission allowed reply")
	}
	m.rows[0].Status = "ended"
	m = update(m, key("i"))
	if m.editor != nil {
		t.Fatal("ended allowed reply")
	}
}
func TestSpawnFormValidationAndPreview(t *testing.T) {
	m := update(newModel(false, true, "", nil), tea.WindowSizeMsg{Width: 100, Height: 24})
	m.spawn = &spawnForm{repos: []string{"api", "billing-service"}, query: inputs("")[0]}
	m = update(m, key("bsv"))
	if len(m.spawn.matches()) != 1 {
		t.Fatal("repo fuzzy filter")
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.spawn.opts.Repo != "billing-service" || m.spawn.step != identityStep {
		t.Fatal("repo selection")
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.spawn.step != identityStep || m.spawn.err == "" {
		t.Fatal("empty name allowed")
	}
	m = update(m, key("412"))
	m = update(m, tea.KeyMsg{Type: tea.KeyTab})
	m = update(m, key("FX cache"))
	if got := m.spawn.fields[2].Value(); got != "feat/412-fx-cache" {
		t.Fatal(got)
	}
	m = update(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.spawn.step != agentStep || m.spawn.opts.Name != "FX cache" {
		t.Fatal("identity step")
	}
	m = update(m, key("j"))
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil || m.spawn.opts.Agent != "codex" {
		t.Fatal("agent selection")
	}
	m = update(m, spawnLoaded{forBlueprint: true, blueprints: []blueprint.Blueprint{{Name: "plan", Agent: "codex", Vars: []string{"constraints"}}}})
	m = update(m, key("j"))
	m = update(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.spawn.step != varsStep || m.spawn.opts.Blueprint != "plan" {
		t.Fatal("blueprint vars")
	}
	m = update(m, key("Keep it small"))
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil || !m.busy {
		t.Fatal("prepare command")
	}
	p := member.Prepared{Manifest: member.Manifest{Repo: "api", Name: "FX cache", Branch: "feat/412-fx-cache", Agent: "codex"}, Prompt: "Plan first", HasPrompt: true}
	m = update(m, spawnPrepared{plan: p})
	if m.spawn.step != previewStep || !strings.Contains(m.View(), "Plan first") {
		t.Fatal("preview")
	}
	m = update(m, promptEdited{prompt: "Edited prompt"})
	if m.spawn.plan.Prompt != "Edited prompt" {
		t.Fatal("edited prompt discarded")
	}
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil || !m.busy || m.spawn.step != launchStep {
		t.Fatal("launch")
	}
	m = update(m, spawnProgress("Fetching origin/main…"))
	if !strings.Contains(m.View(), "Fetching") {
		t.Fatal("inline progress")
	}
	p.Manifest.ID = "412-fx-cache"
	m.allRows = append(m.allRows, member.Row{Manifest: p.Manifest, Alive: false})
	m = update(m, spawnFinished{manifest: p.Manifest})
	if m.spawn != nil || m.selectedID() != "412-fx-cache" || m.busy || len(m.rows) != 1 || !m.rows[0].Alive {
		t.Fatal("new member focus")
	}
}
func TestSpawnViewsFit(t *testing.T) {
	for _, size := range [][2]int{{60, 10}, {80, 24}, {140, 40}} {
		for step := repoStep; step <= launchStep; step++ {
			m := update(newModel(false, true, "", nil), tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			m.spawn = &spawnForm{step: step, opts: member.SpawnOptions{Agent: "claude"}, repos: []string{strings.Repeat("repo", 50)}, query: inputs("")[0], fields: inputs("412", strings.Repeat("界", 100), "feat/test"), vars: []string{"a", "b", "c"}, preview: viewport.New(30, 10), progress: strings.Repeat("long progress", 100), err: "an error\nsecond line"}
			m.spawn.preview.SetContent(strings.Repeat("prompt\n", 50))
			view := m.View()
			if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
				t.Fatalf("overflow %v step %d: %dx%d", size, step, lipgloss.Width(view), lipgloss.Height(view))
			}
		}
	}
}

func TestSpawnFormPermissionMode(t *testing.T) {
	atBlueprints := func(t *testing.T) Model {
		t.Helper()
		m := update(newModel(false, true, "", nil), tea.WindowSizeMsg{Width: 120, Height: 30})
		m.spawn = &spawnForm{step: agentStep, opts: member.SpawnOptions{Repo: "api", Name: "FX", Branch: "feat/fx"}}
		next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m = next.(Model)
		if cmd == nil || m.spawn.opts.Agent != "claude" {
			t.Fatal("agent selection")
		}
		return update(m, spawnLoaded{forBlueprint: true, blueprints: []blueprint.Blueprint{
			{Name: "plan-first", Agent: "claude", Args: []string{"--permission-mode=plan"}},
			{Name: "opus", Agent: "claude", Args: []string{"--model", "opus"}},
		}})
	}
	choose := func(t *testing.T, m Model, index int) (Model, tea.Cmd) {
		t.Helper()
		for i := 0; i < index; i++ {
			m = update(m, key("j"))
		}
		next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		return next.(Model), cmd
	}
	// Claude without a permission-setting blueprint offers the presets.
	m, _ := choose(t, atBlueprints(t), 2)
	if m.spawn.step != modeStep || m.spawn.opts.Blueprint != "opus" {
		t.Fatal("mode step not offered")
	}
	view := m.View()
	for _, want := range []string{"Permission mode", "default from claude settings", "sandbox — accept edits"} {
		if !strings.Contains(view, want) {
			t.Fatalf("mode view lacks %q:\n%s", want, view)
		}
	}
	m, cmd := choose(t, m, len(agents.Modes("claude"))+5) // clamps to the last preset
	if cmd == nil || !m.busy || m.spawn.opts.Mode != "sandbox" {
		t.Fatalf("sandbox selection: %+v", m.spawn.opts)
	}
	m = update(m, spawnPrepared{plan: member.Prepared{Manifest: member.Manifest{Repo: "api", Branch: "feat/fx", Agent: "claude", Mode: "sandbox"}}})
	if !strings.Contains(m.View(), "feat/fx · claude · sandbox") {
		t.Fatal("preview hides the mode")
	}
	// The first entry keeps Claude's own default and passes no mode.
	m, _ = choose(t, atBlueprints(t), 0)
	if m, cmd = choose(t, m, 0); cmd == nil || m.spawn.opts.Mode != "" {
		t.Fatalf("settings default: %+v", m.spawn.opts)
	}
	// A blueprint that already sets the permission mode skips the step.
	if m, cmd = choose(t, atBlueprints(t), 1); cmd == nil || m.spawn.step == modeStep || m.spawn.opts.Mode != "" {
		t.Fatal("blueprint permission mode was overridable")
	}
}
