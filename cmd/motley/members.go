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
	"github.com/thomashartm/motley/internal/crew"
	"github.com/thomashartm/motley/internal/member"
	"github.com/thomashartm/motley/internal/report"
	"github.com/thomashartm/motley/internal/shellx"
	"github.com/thomashartm/motley/internal/state"
	"github.com/thomashartm/motley/internal/tmux"
)

func spawnCommand() *cobra.Command {
	var opts member.SpawnOptions
	var detach bool
	cmd := &cobra.Command{
		Use:   "spawn --repo <name> --branch <new-branch>",
		Short: "Create a worktree and start a coding agent in tmux",
		Long:  "Create a new branch from origin/main (or origin/master), push it, copy local\nartifacts and start an agent. Switch inside tmux; attach outside.\nUse --detach to leave the session running in the background.\nA numeric --ticket looks up the GitHub issue with gh (title, body, URL) for\nblueprints and suggests a crew from its parent issue or milestone.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			p, err := member.Prepare(cfg, opts)
			if err != nil {
				return err
			}
			m, err := member.SpawnPrepared(p, cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Created %s\nWorktree: %s\nAttach: motley attach %s\n", m.ID, m.Worktree, m.ID); err != nil {
				return err
			}
			// Notes follow the spawn because the crew hint names the new member id.
			// A crews file that cannot be read only weakens the suggested id.
			crews, _ := crew.Load()
			for _, note := range spawnNotes(p, opts, crews, m.ID) {
				if _, err := fmt.Fprintln(cmd.ErrOrStderr(), note); err != nil {
					return err
				}
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
	cmd.Flags().StringVar(&opts.Mode, "mode", "", "Claude permission mode: "+agents.ModeNames("claude")+" (default: Claude's own setting)")
	cmd.Flags().StringVar(&opts.Blueprint, "blueprint", "", "Blueprint name")
	cmd.Flags().StringArrayVar(&opts.Vars, "var", nil, "Blueprint variable key=value (repeatable)")
	cmd.Flags().StringVar(&opts.Ticket, "ticket", "", "Ticket identifier")
	cmd.Flags().StringVar(&opts.Name, "name", "", "Display name (defaults to the branch's last component)")
	cmd.Flags().BoolVar(&detach, "detach", false, "Create without attaching or switching")
	cmd.Flags().BoolVar(&opts.NoGH, "no-gh", false, "Skip the GitHub issue lookup for a numeric --ticket")
	cmd.Flags().BoolVar(&opts.CreateCrew, "create-crew", false, "Create and assign the crew suggested by the issue's parent or milestone")
	_ = cmd.MarkFlagRequired("repo")
	_ = cmd.MarkFlagRequired("branch")
	return cmd
}

// spawnNotes reports lookup warnings, the issue found and what happened to its
// suggested crew. crews is the crew list after the spawn.
func spawnNotes(p member.Prepared, opts member.SpawnOptions, crews []crew.Crew, id string) []string {
	var notes []string
	for _, w := range p.Warnings {
		notes = append(notes, "warning: "+w)
	}
	if p.Issue == nil {
		return notes
	}
	notes = append(notes, fmt.Sprintf("Issue #%d: %s", p.Issue.Number, p.Issue.Title))
	s := p.Issue.Suggestion
	switch {
	case s == nil:
	case p.AutoCrew:
		notes = append(notes, fmt.Sprintf("Crew: %s (matches %s %s)", p.Manifest.Crew, s.Source, s.URL))
	case p.NewCrew != nil:
		notes = append(notes, fmt.Sprintf("Crew: created %q from %s %s", s.Title, s.Source, s.URL))
	case opts.Crew == "" && !opts.SkipSuggestion:
		notes = append(notes, fmt.Sprintf("Suggested crew from %s %q:", s.Source, s.Title),
			fmt.Sprintf("  motley crew add --url %s --title %s && motley crew assign %s %s", shellx.Quote(s.URL), shellx.Quote(s.Title), id, member.NewCrewID(crews, s.Title)),
			"  or spawn with --create-crew next time")
	}
	return notes
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
			rows, err := member.List()
			if err != nil {
				return err
			}
			for _, r := range rows {
				if r.ID == args[0] && r.External {
					if r.CodexSession == "" {
						return member.ExternalTerminal(r.ID)
					}
				}
			}
			if err := member.PrepareOpen(args[0]); err != nil {
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
			if m.CodexSession != "" {
				if err := member.ValidateCodex(m); err != nil {
					return err
				}
				return agents.ExecCodexClient(m.CodexSocket, m.CodexSession)
			}
			sessionID := ""
			if resume {
				sessionID, err = state.LatestSessionID(filepath.Join(dir, m.ID+".events.jsonl"), m.Agent)
				if err != nil {
					return err
				}
			}
			if resume && sessionID == "" {
				sessionID = m.ClaudeSession
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
	cmd.Flags().BoolVar(&resume, "resume", false, "Resume the agent's latest recorded session")
	return cmd
}

func agentExitedCommand() *cobra.Command {
	return &cobra.Command{
		Use: "agent-exited <id>", Short: "Record that the member's agent returned to the shell (silent)", Hidden: true,
		Args: cobra.ExactArgs(1),
		Run:  func(_ *cobra.Command, args []string) { report.Exited(args[0]) },
	}
}
