package tui

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/term"
	"github.com/thomashartm/motley/internal/crew"
	"github.com/thomashartm/motley/internal/gh"
	"github.com/thomashartm/motley/internal/member"
	"github.com/thomashartm/motley/internal/state"
	"github.com/thomashartm/motley/internal/tmux"
)

// Run restores the terminal before replacing this process with tmux attach.
func Run(monitor bool, client string, bell bool) error {
	if !term.IsTerminal(os.Stdin.Fd()) || !term.IsTerminal(os.Stdout.Fd()) {
		return fmt.Errorf("the overview requires a terminal; use motley ls for text output")
	}
	inside := os.Getenv("TMUX") != ""
	if monitor && !inside {
		return fmt.Errorf("use motley monitor to open the monitor session")
	}
	// An overview inside a managed agent would kill itself when retiring that
	// member. Always run that overview in the independent monitor session.
	if inside && client == "" {
		session, err := tmux.CurrentSession()
		if err != nil {
			return err
		}
		sessions, err := tmux.Sessions()
		if err != nil {
			return err
		}
		for _, s := range sessions {
			if s.Name == session && s.MemberID != "" && !s.Monitor {
				origin, err := tmux.CurrentClient()
				if err != nil {
					return err
				}
				if err := tmux.EnsureMonitor(); err != nil {
					return err
				}
				return tmux.SwitchClient(origin, tmux.MonitorSession)
			}
		}
	}
	if inside && !monitor && client == "" {
		var err error
		client, err = tmux.CurrentClient()
		if err != nil {
			return err
		}
	}
	dir, err := state.MembersDir()
	if err != nil {
		return err
	}
	catalog := &member.Catalog{}
	poll := func() tea.Msg {
		manifests, err := catalog.Load(dir)
		if err != nil {
			return snapshot{err: err}
		}
		sessions, err := tmux.Sessions()
		if err != nil {
			return snapshot{err: err}
		}
		clients, err := tmux.Clients()
		if err != nil {
			return snapshot{err: err}
		}
		crews, err := crew.Load()
		if err != nil {
			return snapshot{err: err}
		}
		rows, err := member.RefreshExternal(member.Join(manifests, sessions))
		return snapshot{rows: rows, clients: clients, crews: crews, err: err}
	}
	m := newModel(monitor, inside, client, poll)
	m.bell = bell
	m.github = gh.Default()
	cache := &detailCache{dir: dir}
	m.fetchDetail = cache.command
	m.copyText = copyToClipboard(inside, os.Stdout)
	var program *tea.Program
	m.sendMsg = func(msg tea.Msg) { program.Send(msg) }
	program = tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseAllMotion())
	result, err := program.Run()
	if err != nil {
		return err
	}
	m = result.(Model)
	if m.attachID != "" {
		return tmux.Attach(m.attachID)
	}
	return nil
}
