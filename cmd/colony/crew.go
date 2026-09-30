package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/thomashartm/colony/internal/crew"
	"github.com/thomashartm/colony/internal/minion"
)

func crewCommand() *cobra.Command {
	root := &cobra.Command{Use: "crew", Short: "Group minions by a named package of work"}
	var title, url, color string
	add := &cobra.Command{Use: "add --title <title>", Short: "Create a crew with an optional link and colour", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		c, err := minion.AddCrew(title, url, color)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "Created crew %s (%s, %s)\n", c.ID, c.Title, c.Color)
		return err
	}}
	add.Flags().StringVar(&title, "title", "", "Crew title (required)")
	_ = add.MarkFlagRequired("title")
	add.Flags().StringVar(&url, "url", "", "Optional http/https link (no network lookup)")
	add.Flags().StringVar(&color, "color", "", "Palette colour (default: next unused)")
	var asJSON bool
	list := &cobra.Command{Use: "list", Short: "List crews", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		crews, err := crew.Load()
		if err != nil {
			return err
		}
		sort.Slice(crews, func(i, j int) bool { return crews[i].ID < crews[j].ID })
		if asJSON {
			if crews == nil {
				crews = []crew.Crew{}
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(crews)
		}
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
		if _, err := fmt.Fprintln(w, "ID\tTITLE\tCOLOR\tKIND\tURL"); err != nil {
			return err
		}
		for _, c := range crews {
			if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", c.ID, c.Title, c.Color, c.Kind, c.URL); err != nil {
				return err
			}
		}
		return w.Flush()
	}}
	list.Flags().BoolVar(&asJSON, "json", false, "Print JSON")
	var editTitle, editURL, editColor string
	edit := &cobra.Command{Use: "edit <id>", Short: "Edit a crew and refresh its live sessions", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		e := minion.CrewEdit{}
		if cmd.Flags().Changed("title") {
			e.Title = &editTitle
		}
		if cmd.Flags().Changed("url") {
			e.URL = &editURL
		}
		if cmd.Flags().Changed("color") {
			e.Color = &editColor
		}
		return minion.EditCrew(args[0], e)
	}}
	edit.Flags().StringVar(&editTitle, "title", "", "New title")
	edit.Flags().StringVar(&editURL, "url", "", "New link (empty clears it)")
	edit.Flags().StringVar(&editColor, "color", "", "New palette colour")
	var force bool
	rm := &cobra.Command{Use: "rm <id>", Short: "Remove an unused crew; --force unassigns its minions", Args: cobra.ExactArgs(1), RunE: func(_ *cobra.Command, args []string) error { return minion.RemoveCrew(args[0], force) }}
	rm.Flags().BoolVar(&force, "force", false, "Unassign active and dead minions before removal")
	assign := &cobra.Command{Use: "assign <minion> <crew-id|none>", Short: "Assign or unassign a minion and refresh its colour", Args: cobra.ExactArgs(2), RunE: func(_ *cobra.Command, args []string) error {
		return minion.EditIdentity(args[0], minion.IdentityEdit{Crew: &args[1]})
	}}
	root.AddCommand(add, list, edit, rm, assign)
	return root
}
