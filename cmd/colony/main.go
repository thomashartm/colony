package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/thomashartm/colony/internal/config"
)

// Set by GoReleaser using -ldflags.
var version = "dev"

func main() {
	if err := newRootCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "colony:", err)
		os.Exit(1)
	}
}

func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "colony",
		Short:         "A terminal tool for AI coding sessions",
		Long:          "colony — a terminal tool for AI coding sessions.\n\nRun without arguments to show the configured repository and worktree roots.\nConfiguration: ${XDG_CONFIG_HOME:-~/.config}/colony/config.toml.\nFile settings override defaults: ~/projects and ~/worktrees.",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "colony %s\nrepos_root: %s\nworktrees_root: %s\n", version, cfg.ReposRoot, cfg.WorktreesRoot)
			return err
		},
	}
	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print the colony version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "colony %s\n", version)
			return err
		},
	})
	root.AddCommand(spawnCommand(), listCommand(), connectCommand(true), connectCommand(false), execAgentCommand())
	return root
}
