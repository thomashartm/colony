package main

import (
	"github.com/spf13/cobra"
	"github.com/thomashartm/motley/internal/update"
)

func updateCommand() *cobra.Command {
	var check bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Check GitHub for the latest release and install it",
		Long:  "Download the latest published release using gh, verify its SHA-256 checksum,\nand replace motley and mtly beside the running executable. Settings and\nworktrees are preserved. Requires gh and a writable installation directory.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return update.Run(version, check, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "Check for a release without installing it")
	return cmd
}
