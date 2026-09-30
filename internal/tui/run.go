package tui

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/term"
	"github.com/thomashartm/colony/internal/minion"
	"github.com/thomashartm/colony/internal/state"
	"github.com/thomashartm/colony/internal/tmux"
)

// Run restores the terminal before replacing this process with tmux attach.
func Run(monitor bool, client string) error {
	if !term.IsTerminal(os.Stdin.Fd()) || !term.IsTerminal(os.Stdout.Fd()) {
		return fmt.Errorf("the overview requires a terminal; use colony ls for text output")
	}
	inside := os.Getenv("TMUX") != ""
	if monitor && !inside {
		return fmt.Errorf("use colony monitor to open the monitor session")
	}
	if inside && !monitor && client == "" {
		var err error
		client, err = tmux.CurrentClient()
		if err != nil {
			return err
		}
	}
	dir, err := state.MinionsDir()
	if err != nil {
		return err
	}
	catalog := &minion.Catalog{}
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
		return snapshot{rows: minion.Join(manifests, sessions), clients: clients}
	}
	result, err := tea.NewProgram(newModel(monitor, inside, client, poll), tea.WithAltScreen()).Run()
	if err != nil {
		return err
	}
	m := result.(Model)
	if m.attachID != "" {
		return tmux.Attach(m.attachID)
	}
	return nil
}
