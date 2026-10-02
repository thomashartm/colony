// Package config loads motley's configuration over its defaults.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Config contains the root paths used by motley.
type Config struct {
	Schema        int    `toml:"schema"`
	ReposRoot     string `toml:"repos_root"`
	WorktreesRoot string `toml:"worktrees_root"`
	MonitorBell   bool   `toml:"monitor_bell"`
}

// Load creates the config once, defaults missing settings, and expands ~/.
func Load() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, fmt.Errorf("find home directory: %w", err)
	}
	path, err := Ensure()
	if err != nil {
		return Config{}, err
	}
	cfg := Config{Schema: 1, ReposRoot: "~/projects", WorktreesRoot: "~/worktrees"}
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
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
