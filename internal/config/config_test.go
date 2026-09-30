package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		content string
		missing bool
		xdg     bool
		repos   string
		work    string
		wantErr string
	}{
		{name: "missing file", missing: true, repos: "~/projects", work: "~/worktrees"},
		{name: "file settings", content: "schema = 1\nrepos_root = '/srv/repos'\nworktrees_root = '/srv/trees'", repos: "/srv/repos", work: "/srv/trees"},
		{name: "partial and unknown settings", content: "repos_root = '~/code'\nfuture = true", repos: "~/code", work: "~/worktrees"},
		{name: "XDG location", xdg: true, content: "worktrees_root = '~/trees'", repos: "~/projects", work: "~/trees"},
		{name: "malformed TOML", content: "repos_root = [", wantErr: "parse config"},
		{name: "wrong type", content: "repos_root = 42", wantErr: "parse config"},
		{name: "unsupported schema", content: "schema = 2", wantErr: "unsupported schema 2"},
		{name: "empty root", content: "worktrees_root = ''", wantErr: "must not be empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", "")
			configHome := filepath.Join(home, ".config")
			if tt.xdg {
				configHome = t.TempDir()
				t.Setenv("XDG_CONFIG_HOME", configHome)
			}
			path := filepath.Join(configHome, "colony", "config.toml")
			if !tt.missing {
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			cfg, err := Load()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("want error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			wantRepos := strings.Replace(tt.repos, "~/", home+"/", 1)
			wantWork := strings.Replace(tt.work, "~/", home+"/", 1)
			if cfg.Schema != 1 || cfg.ReposRoot != wantRepos || cfg.WorktreesRoot != wantWork {
				t.Fatalf("got %+v; want schema 1, roots %q and %q", cfg, wantRepos, wantWork)
			}
			if tt.missing {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("loading defaults must not create a config file: %v", err)
				}
			}
		})
	}
}
