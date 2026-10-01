package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/thomashartm/motley/internal/agents/claude"
	"github.com/thomashartm/motley/internal/agents/codex"
	"github.com/thomashartm/motley/internal/agents/opencode"
	"github.com/thomashartm/motley/internal/report"
)

func reportCommand() *cobra.Command {
	return &cobra.Command{Use: "report --agent <claude|codex|opencode>", Short: "Receive an agent hook (silent, always exits successfully)", DisableFlagParsing: true, Run: func(cmd *cobra.Command, args []string) { report.Run(args, cmd.InOrStdin()) }}
}
func hooksCommand() *cobra.Command {
	root := &cobra.Command{Use: "hooks", Short: "Manage agent status hooks"}
	install := &cobra.Command{Use: "install <claude|codex|opencode>", Short: "Install agent status reporting, with a backup", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		var path, backup string
		var changed bool
		var err error
		switch args[0] {
		case "claude":
			path, backup, changed, err = claude.Install()
		case "codex":
			path, backup, changed, err = codex.Install()
		case "opencode":
			path, backup, changed, err = opencode.Install()
		default:
			return fmt.Errorf("choose claude, codex or opencode")
		}
		if err != nil {
			return err
		}
		if !changed {
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Motley hooks are already installed in %s\n", path)
			if err == nil && args[0] == "codex" {
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "In Codex, use /hooks to review and trust the Motley hooks before they can run.")
			}
			return err
		}
		if _, err = fmt.Fprintf(cmd.OutOrStdout(), "Installed motley hooks in %s\n", path); err != nil {
			return err
		}
		if backup != "" {
			if _, err = fmt.Fprintf(cmd.OutOrStdout(), "Backup: %s\n", backup); err != nil {
				return err
			}
		}
		if args[0] == "codex" {
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "Restart Codex, then use /hooks to review and trust the Motley hooks. Hook trust and approval policies are unchanged.")
		} else {
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Restart existing %s sessions to load the hooks. Sessions outside motley are ignored.\n", args[0])
		}
		return err
	}}
	root.AddCommand(install)
	return root
}
