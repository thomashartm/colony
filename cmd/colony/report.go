package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/thomashartm/colony/internal/agents/claude"
	"github.com/thomashartm/colony/internal/report"
)

func reportCommand() *cobra.Command {
	return &cobra.Command{Use: "report --agent claude", Short: "Receive an agent hook (silent, always exits successfully)", DisableFlagParsing: true, Run: func(cmd *cobra.Command, args []string) { report.Run(args, cmd.InOrStdin()) }}
}
func hooksCommand() *cobra.Command {
	root := &cobra.Command{Use: "hooks", Short: "Manage agent status hooks"}
	install := &cobra.Command{Use: "install claude", Short: "Merge Claude status hooks into settings, with a backup", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if args[0] != "claude" {
			return fmt.Errorf("hook installation currently supports claude")
		}
		path, backup, changed, err := claude.Install()
		if err != nil {
			return err
		}
		if !changed {
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Colony hooks are already installed in %s\n", path)
			return err
		}
		if _, err = fmt.Fprintf(cmd.OutOrStdout(), "Installed colony hooks in %s\n", path); err != nil {
			return err
		}
		if backup != "" {
			if _, err = fmt.Fprintf(cmd.OutOrStdout(), "Backup: %s\n", backup); err != nil {
				return err
			}
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), "Restart existing Claude sessions to load the hooks. Sessions outside colony are ignored.")
		return err
	}}
	root.AddCommand(install)
	return root
}
