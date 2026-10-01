package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/thomashartm/motley/internal/config"
	"github.com/thomashartm/motley/internal/report"
	"github.com/thomashartm/motley/internal/tmux"
	"github.com/thomashartm/motley/internal/tui"
)

// Set by GoReleaser using -ldflags.
var version = "dev"

func main() {
	// Hooks must bypass Cobra's flag errors, help output and nonzero exits.
	if len(os.Args) > 1 && os.Args[1] == "report" {
		report.Run(os.Args[2:], os.Stdin)
		return
	}
	if err := newRootCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "motley:", err)
		os.Exit(1)
	}
}

func newRootCommand() *cobra.Command {
	var monitor bool
	var client string
	root := &cobra.Command{
		Use:           "motley",
		Aliases:       []string{"mtly"},
		Short:         "A terminal tool for AI coding sessions",
		Long:          "motley (mtly) — a terminal tool for AI coding sessions.\n\nRun without arguments to open the session overview.\nUse motley monitor for a persistent overview with a separate work tab.\nConfiguration: ~/.motley/config.toml (created on first launch).",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			return tui.Run(monitor, client, cfg.MonitorBell)
		},
	}
	root.Flags().BoolVar(&monitor, "monitor", false, "Run the monitor overview (internal)")
	root.Flags().StringVar(&client, "client", "", "Originating tmux client (internal)")
	_ = root.Flags().MarkHidden("monitor")
	_ = root.Flags().MarkHidden("client")
	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print the motley version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "motley %s\n", version)
			return err
		},
	})
	root.AddCommand(importCommand(), navigationCommand(), updateCommand(), tabsCommand(), sendCommand(), blueprintCommand(), crewCommand(), retireCommand(), reviveCommand(), adoptCommand(), spawnCommand(), listCommand(), connectCommand(true), connectCommand(false), execAgentCommand(), reportCommand(), hooksCommand())
	root.AddCommand(&cobra.Command{
		Use: "config", Short: "Show the configured repository and worktree roots", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "motley %s\nrepos_root: %s\nworktrees_root: %s\n", version, cfg.ReposRoot, cfg.WorktreesRoot)
			return err
		},
	})
	root.AddCommand(&cobra.Command{
		Use: "init", Short: "Create missing config files and the tmux popup binding", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := config.Init()
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Existing files are preserved. Add this line to ~/.tmux.conf and reload tmux configuration:\nsource-file %q\n\nEnable agent reporting: motley hooks install <claude|codex|opencode>\n", path)
			return err
		},
	})
	root.AddCommand(&cobra.Command{
		Use: "monitor", Short: "Open a persistent overview that jumps in another work tab", Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if _, err := config.Load(); err != nil {
				return err
			}
			if err := tmux.EnsureMonitor(); err != nil {
				return err
			}
			return jump(tmux.MonitorSession)
		},
	})
	return root
}
