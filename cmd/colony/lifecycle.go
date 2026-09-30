package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/thomashartm/colony/internal/minion"
	"github.com/thomashartm/colony/internal/tmux"
)

func retireCommand() *cobra.Command {
	var force, keep bool
	cmd := &cobra.Command{Use: "retire <id>", Short: "Remove a minion's session/worktree and archive its history", Long: "Refuse uncommitted or unpushed work unless --force is supplied.\nRemove the linked worktree and local branch; keep main/master/develop and all\nremote branches. Run from the monitor, another session, or outside tmux.", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if err := minion.Retire(args[0], force, keep); err != nil {
			return err
		}
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "Retired %s; history archived. Remote branches were kept.\n", args[0])
		return err
	}}
	cmd.Flags().BoolVar(&force, "force", false, "Discard uncommitted and unpushed work")
	cmd.Flags().BoolVar(&keep, "keep-branch", false, "Keep the local branch")
	return cmd
}
func reviveCommand() *cobra.Command {
	return &cobra.Command{Use: "revive <id>", Short: "Restart a dead minion in its existing worktree", Long: "Recreate a missing tmux session. Claude resumes its latest recorded session;\nwithout a recorded session, or for other agents, start fresh without a prompt.\nArchived minions are retired and cannot be revived.", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if err := minion.Revive(args[0]); err != nil {
			return err
		}
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "Revived %s. Attach: colony attach %s\n", args[0], args[0])
		return err
	}}
}
func adoptCommand() *cobra.Command {
	var opts minion.AdoptOptions
	cmd := &cobra.Command{Use: "adopt", Short: "Register this tmux session and linked git worktree as a minion", Long: "Run inside tmux from a linked worktree. Colony records its Git state and\nrenames the session to the minion id. Existing processes keep their environment;\nexport COLONY_MINION in the current shell and restart the agent for reporting.", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		m, err := minion.Adopt(opts)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "Adopted %s. Session: %s\nFor reporting in the current shell, run:\nexport COLONY_MINION=%s\nThen restart %s. New panes inherit the minion id.\n", m.ID, tmux.SessionName(m.ID), m.ID, m.Agent)
		return err
	}}
	cmd.Flags().StringVar(&opts.Crew, "crew", "", "Crew id")
	cmd.Flags().StringVar(&opts.Color, "color", "", "Colour override (otherwise inherit crew colour)")
	cmd.Flags().StringVar(&opts.Agent, "agent", "claude", "Agent: claude, codex or opencode")
	cmd.Flags().StringVar(&opts.Ticket, "ticket", "", "Ticket identifier")
	cmd.Flags().StringVar(&opts.Name, "name", "", "Display name (defaults to branch name)")
	return cmd
}
