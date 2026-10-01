package main

import (
	"github.com/spf13/cobra"
	"github.com/thomashartm/motley/internal/tmux"
)

func navigationCommand() *cobra.Command {
	var client, action, prefix string
	var row, column int
	cmd := &cobra.Command{Use: "navigation", Hidden: true, Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error { return tmux.Navigate(client, action, prefix, row, column) },
	}
	cmd.Flags().StringVar(&client, "client", "", "Originating tmux client")
	cmd.Flags().StringVar(&action, "action", "", "Navigation action")
	cmd.Flags().StringVar(&prefix, "prefix", "", "Displayed prefix")
	cmd.Flags().IntVar(&row, "row", 0, "Status row")
	cmd.Flags().IntVar(&column, "column", 0, "Status column")
	return cmd
}
