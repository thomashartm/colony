package main

import (
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/thomashartm/motley/internal/agents"
	"github.com/thomashartm/motley/internal/blueprint"
	"github.com/thomashartm/motley/internal/config"
	"github.com/thomashartm/motley/internal/member"
	"github.com/thomashartm/motley/internal/state"
	"github.com/thomashartm/motley/internal/tmux"
)

func spawnCommand() *cobra.Command {
	var opts member.SpawnOptions
	var detach bool
	cmd := &cobra.Command{
		Use:   "spawn --repo <name> --branch <new-branch>",
		Short: "Create a worktree and start a coding agent in tmux",
		Long:  "Create a new branch from origin/main (or origin/master), push it, copy local\nartifacts and start an agent. Switch inside tmux; attach outside.\nUse --detach to leave the session running in the background.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			m, err := member.Spawn(cfg, opts, cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Created %s\nWorktree: %s\nAttach: motley attach %s\n", m.ID, m.Worktree, m.ID); err != nil {
				return err
			}
			if detach {
				return nil
			}
			return jump(m.ID)
		},
	}
	cmd.Flags().StringVar(&opts.Repo, "repo", "", "Main repository directory name under repos_root (required)")
	cmd.Flags().StringVar(&opts.Branch, "branch", "", "New branch name (required)")
	cmd.Flags().StringVar(&opts.Crew, "crew", "", "Crew id")
	cmd.Flags().StringVar(&opts.Color, "color", "", "Colour override (otherwise inherit crew colour)")
	cmd.Flags().StringVar(&opts.Agent, "agent", "", "Agent: claude, codex or opencode (default: blueprint agent, then claude)")
	cmd.Flags().StringVar(&opts.Blueprint, "blueprint", "", "Blueprint name")
	cmd.Flags().StringArrayVar(&opts.Vars, "var", nil, "Blueprint variable key=value (repeatable)")
	cmd.Flags().StringVar(&opts.Ticket, "ticket", "", "Ticket identifier")
	cmd.Flags().StringVar(&opts.Name, "name", "", "Display name (defaults to the branch's last component)")
	cmd.Flags().BoolVar(&detach, "detach", false, "Create without attaching or switching")
	_ = cmd.MarkFlagRequired("repo")
	_ = cmd.MarkFlagRequired("branch")
	return cmd
}

func listCommand() *cobra.Command {
	return &cobra.Command{
		Use: "ls", Short: "List members, agent status and tmux session state", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			rows, err := member.List()
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			if _, err := fmt.Fprintln(w, "ID\tTICKET\tREPO\tBRANCH\tAGENT\tSTATUS\tSTATE"); err != nil {
				return err
			}
			for _, row := range rows {
				status := "dead"
				if row.Alive {
					status = "alive"
				}
				ticket := row.Ticket
				if ticket == "" {
					ticket = "—"
				}
				if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", row.ID, ticket, row.Repo, row.Branch, row.Agent, row.CurrentStatus(), status); err != nil {
					return err
				}
			}
			return w.Flush()
		},
	}
}

func connectCommand(attach bool) *cobra.Command {
	use, short := "switch <id>", "Switch this tmux client to a member, or attach outside tmux"
	if attach {
		use, short = "attach <id>", "Attach this terminal to a member's tmux session"
	}
	return &cobra.Command{
		Use: use, Short: short, Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if err := member.RequireLive(args[0]); err != nil {
				return err
			}
			if attach {
				return tmux.Attach(args[0])
			}
			return jump(args[0])
		},
	}
}

func jump(id string) error {
	if os.Getenv("TMUX") != "" {
		return tmux.Switch(id)
	}
	return tmux.Attach(id)
}

func execAgentCommand() *cobra.Command {
	var resume bool
	cmd := &cobra.Command{
		Use: "exec-agent <id>", Short: "Start the agent recorded in a member manifest", Hidden: true,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			dir, err := state.MembersDir()
			if err != nil {
				return err
			}
			m, err := member.Load(dir, args[0])
			if err != nil {
				return err
			}
			if err := os.Chdir(m.Worktree); err != nil {
				return err
			}
			if err := os.Setenv("MOTLEY_MEMBER", m.ID); err != nil {
				return err
			}
			sessionID := ""
			if resume && m.Agent == "claude" {
				sessionID, err = state.LatestSessionID(filepath.Join(dir, m.ID+".events.jsonl"), m.Agent)
				if err != nil {
					return err
				}
			}
			prompt := ""
			if !resume && (m.Prompt || m.Blueprint != "") {
				prompt, err = blueprint.ReadPrompt(filepath.Join(dir, m.ID+".prompt.md"))
				if err != nil {
					return err
				}
			}
			return agents.Exec(m.Agent, m.AgentArgs, prompt, sessionID)
		},
	}
	cmd.Flags().BoolVar(&resume, "resume", false, "Resume the latest recorded Claude session")
	return cmd
}
