package main

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/thomashartm/motley/internal/member"
)

func importCommand() *cobra.Command {
	var list bool
	var name, crew, agent string
	cmd := &cobra.Command{Use: "import [session-id]", Short: "Add an existing Claude or Codex session", Long: "Discover and register existing sessions without restarting them.\nUse --agent claude (default) or --agent codex, and --list to choose a session.\nImported directories and branches are always kept on retirement.\nCodex requires a running local shared app-server; standalone --no-daemon sessions are not discoverable.", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if list || len(args) == 0 {
			if len(args) > 0 {
				return fmt.Errorf("use --list without a session ID")
			}
			sessions, err := member.DiscoverImports(agent)
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			if _, err := fmt.Fprintln(w, "SESSION ID\tNAME\tSTATUS\tDIRECTORY"); err != nil {
				return err
			}
			for _, s := range sessions {
				if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", s.SessionID, s.Name, s.Status, s.Cwd); err != nil {
					return err
				}
			}
			return w.Flush()
		}
		m, err := member.Import(agent, args[0], name, crew)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "Added %s. The existing %s session keeps running.\nOpen the monitor: mtly monitor\n", m.ID, agent)
		return err
	}}
	cmd.Flags().StringVar(&agent, "agent", "claude", "Session provider: claude or codex")
	cmd.Flags().BoolVar(&list, "list", false, "List running sessions not already in Motley")
	cmd.Flags().StringVar(&name, "name", "", "Display name")
	cmd.Flags().StringVar(&crew, "crew", "", "Crew ID")
	return cmd
}
