package main

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/thomashartm/colony/internal/blueprint"
	"github.com/thomashartm/colony/internal/config"
	"github.com/thomashartm/colony/internal/minion"
)

func blueprintCommand() *cobra.Command {
	root := &cobra.Command{Use: "blueprint", Short: "Discover and check reusable initial prompts"}
	var repo string
	root.PersistentFlags().StringVar(&repo, "repo", "", "Include repository blueprints and filter by repo")
	load := func() ([]blueprint.Blueprint, error) {
		path := ""
		if repo != "" {
			cfg, err := config.Load()
			if err != nil {
				return nil, err
			}
			path, err = minion.ResolveRepo(cfg.ReposRoot, repo)
			if err != nil {
				return nil, err
			}
		}
		return blueprint.Discover(path, repo)
	}
	root.AddCommand(&cobra.Command{Use: "list", Short: "List global blueprints, or the effective set for --repo", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		bs, err := load()
		if err != nil {
			return err
		}
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
		if _, err := fmt.Fprintln(w, "NAME\tAGENT\tREPOS\tDESCRIPTION"); err != nil {
			return err
		}
		for _, b := range bs {
			repos := strings.Join(b.Repos, ",")
			if repos == "" {
				repos = "*"
			}
			if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", b.Name, b.Agent, repos, b.Description); err != nil {
				return err
			}
		}
		return w.Flush()
	}})
	root.AddCommand(&cobra.Command{Use: "show <name>", Short: "Print a blueprint's original Markdown, including front matter", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		bs, err := load()
		if err != nil {
			return err
		}
		b, err := blueprint.Find(bs, args[0])
		if err != nil {
			return err
		}
		_, err = fmt.Fprint(cmd.OutOrStdout(), b.Source)
		return err
	}})
	root.AddCommand(&cobra.Command{Use: "validate [name]", Short: "Parse and render blueprints with empty template data", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		bs, err := load()
		if err != nil {
			return err
		}
		if len(args) > 0 {
			b, err := blueprint.Find(bs, args[0])
			if err != nil {
				return err
			}
			bs = []blueprint.Blueprint{b}
		}
		for _, b := range bs {
			if _, err := b.Render(blueprint.Data{Repo: repo, Vars: map[string]string{}}); err != nil {
				return err
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Valid %s (%s)\n", b.Name, b.Path); err != nil {
				return err
			}
		}
		if len(bs) == 0 {
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "No blueprints found.")
		}
		return err
	}})
	return root
}
