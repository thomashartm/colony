package main

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/thomashartm/motley/internal/member"
)

func importCommand() *cobra.Command {
	var list bool
	var name, crew string
	cmd := &cobra.Command{Use: "import [claude-session-id]", Short: "Add a running Claude session from another terminal", Long: "Discover and add existing Claude sessions without restarting them.\nTheir directories and branches are always kept on retirement.\nUse --list to find a session ID, or Actions > Add existing Claude in the monitor.", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if list || len(args) == 0 {
			if len(args) > 0 {
				return fmt.Errorf("use --list without a session ID")
			}
			sessions, err := member.DiscoverClaude()
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
		m, err := member.ImportClaude(args[0], name, crew)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "Added %s. Claude keeps running in its original terminal.\nOpen the monitor: mtly monitor\n", m.ID)
		return err
	}}
	cmd.Flags().BoolVar(&list, "list", false, "List running sessions not already in Motley")
	cmd.Flags().StringVar(&name, "name", "", "Display name")
	cmd.Flags().StringVar(&crew, "crew", "", "Crew ID")
	return cmd
}
