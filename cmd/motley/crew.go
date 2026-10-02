package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/thomashartm/motley/internal/crew"
	"github.com/thomashartm/motley/internal/gh"
	"github.com/thomashartm/motley/internal/member"
)

func crewCommand() *cobra.Command {
	root := &cobra.Command{Use: "crew", Short: "Manage crews of members and their gigs"}
	var title, url, color, gig string
	add := &cobra.Command{Use: "add [--title <title>] [--url <url>]", Short: "Create a crew with an optional gig, link and colour", Long: "Create a crew. Without --title, the title of a GitHub issue or project --url\nis fetched once with gh; other links need --title.", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		ctx, cancel := context.WithTimeout(cmd.Context(), member.LookupTimeout)
		defer cancel()
		c, err := member.AddCrewWithLookup(ctx, gh.Default(), title, url, color, gig)
		if errors.Is(err, gh.ErrMissing) || errors.Is(err, gh.ErrAuth) {
			return fmt.Errorf("%s; or pass --title", gh.Hint(err))
		} else if err != nil {
			return err
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "Created crew %s (%s, %s)\n", c.ID, c.Title, c.Color)
		return err
	}}
	add.Flags().StringVar(&title, "title", "", "Crew title (fetched with gh for a GitHub issue or project --url when omitted)")
	add.Flags().StringVar(&url, "url", "", "Optional http/https link")
	add.Flags().StringVar(&color, "color", "", "Palette colour (default: next unused)")
	add.Flags().StringVar(&gig, "gig", "", "Gig: the crew's package of work")
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
		if _, err := fmt.Fprintln(w, "ID\tTITLE\tGIG\tCOLOR\tKIND\tURL"); err != nil {
			return err
		}
		for _, c := range crews {
			if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", c.ID, c.Title, c.Gig, c.Color, c.Kind, c.URL); err != nil {
				return err
			}
		}
		return w.Flush()
	}}
	list.Flags().BoolVar(&asJSON, "json", false, "Print JSON")
	var editTitle, editURL, editColor, editGig string
	edit := &cobra.Command{Use: "edit <id>", Short: "Edit a crew and refresh its live sessions", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		e := member.CrewEdit{}
		if cmd.Flags().Changed("title") {
			e.Title = &editTitle
		}
		if cmd.Flags().Changed("url") {
			e.URL = &editURL
		}
		if cmd.Flags().Changed("color") {
			e.Color = &editColor
		}
		if cmd.Flags().Changed("gig") {
			e.Gig = &editGig
		}
		return member.EditCrew(args[0], e)
	}}
	edit.Flags().StringVar(&editTitle, "title", "", "New title")
	edit.Flags().StringVar(&editURL, "url", "", "New link (empty clears it)")
	edit.Flags().StringVar(&editColor, "color", "", "New palette colour")
	edit.Flags().StringVar(&editGig, "gig", "", "New gig (empty clears it)")
	var force bool
	rm := &cobra.Command{Use: "rm <id>", Short: "Remove an unused crew; --force unassigns its members", Args: cobra.ExactArgs(1), RunE: func(_ *cobra.Command, args []string) error { return member.RemoveCrew(args[0], force) }}
	rm.Flags().BoolVar(&force, "force", false, "Unassign active and dead members before removal")
	assign := &cobra.Command{Use: "assign <member> <crew-id|none>", Short: "Assign or unassign a member and refresh its colour", Args: cobra.ExactArgs(2), RunE: func(_ *cobra.Command, args []string) error {
		return member.EditIdentity(args[0], member.IdentityEdit{Crew: &args[1]})
	}}
	root.AddCommand(add, list, edit, rm, assign)
	return root
}
