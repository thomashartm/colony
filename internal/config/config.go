// Package config loads colony's configuration over its defaults.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Config contains the root paths used by colony.
type Config struct {
	Schema        int    `toml:"schema"`
	ReposRoot     string `toml:"repos_root"`
	WorktreesRoot string `toml:"worktrees_root"`
}

// Load reads the XDG config file, defaults missing settings, and expands ~/.
// A missing file is valid. Reading configuration never creates files.
func Load() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, fmt.Errorf("find home directory: %w", err)
	}
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		configHome = filepath.Join(home, ".config")
	}
	path := filepath.Join(configHome, "colony", "config.toml")
	cfg := Config{Schema: 1, ReposRoot: "~/projects", WorktreesRoot: "~/worktrees"}
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}
	if err == nil {
		if err := toml.Unmarshal(data, &cfg); err != nil {
			return Config{}, fmt.Errorf("parse config %s: %w", path, err)
		}
	}
	if cfg.Schema != 1 {
		return Config{}, fmt.Errorf("config %s: unsupported schema %d (supported: 1)", path, cfg.Schema)
	}
	if strings.TrimSpace(cfg.ReposRoot) == "" || strings.TrimSpace(cfg.WorktreesRoot) == "" {
		return Config{}, fmt.Errorf("config %s: repos_root and worktrees_root must not be empty", path)
	}
	cfg.ReposRoot = expandHome(cfg.ReposRoot, home)
	cfg.WorktreesRoot = expandHome(cfg.WorktreesRoot, home)
	return cfg, nil
}

func expandHome(path, home string) string {
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}
