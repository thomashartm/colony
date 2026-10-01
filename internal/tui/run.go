package tui

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/term"
	"github.com/thomashartm/motley/internal/crew"
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
		return snapshot{rows: member.Join(manifests, sessions), clients: clients, crews: crews}
	}
	m := newModel(monitor, inside, client, poll)
	m.bell = bell
	cache := &detailCache{dir: dir}
	m.fetchDetail = cache.command
	var program *tea.Program
	m.sendMsg = func(msg tea.Msg) { program.Send(msg) }
	program = tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
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
