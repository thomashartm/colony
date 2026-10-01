package main

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/thomashartm/motley/internal/member"
	"github.com/thomashartm/motley/internal/tmux"
)

func tabsCommand() *cobra.Command {
	return &cobra.Command{Use: "tabs", Short: "List attached tmux clients", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		cs, err := tmux.Clients()
		if err != nil {
			return err
		}
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
		if _, err := fmt.Fprintln(w, "TTY\tSESSION\tACTIVITY"); err != nil {
			return err
		}
		for _, c := range cs {
			if _, err := fmt.Fprintf(w, "%s\t%s\t%d\n", c.TTY, c.Session, c.Activity); err != nil {
				return err
			}
		}
		return w.Flush()
	}}
}
func sendCommand() *cobra.Command {
	var client string
	cmd := &cobra.Command{Use: "send <id> --tab <client>", Short: "Switch another attached work tab to a member", Args: cobra.ExactArgs(1), RunE: func(_ *cobra.Command, args []string) error { return member.Send(args[0], client) }}
	cmd.Flags().StringVar(&client, "tab", "", "Client TTY from motley tabs")
	_ = cmd.MarkFlagRequired("tab")
	return cmd
}
