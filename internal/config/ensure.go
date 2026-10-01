package config

import (
	"fmt"
	"os"
	"path/filepath"
)

const defaultConfig = "schema = 1\nrepos_root = \"~/projects\"\nworktrees_root = \"~/worktrees\"\nmonitor_bell = false\n"

// Ensure creates ~/.motley/config.toml once. An existing legacy configuration is
// copied byte-for-byte, including comments and settings unknown to this version.
func Ensure() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home directory: %w", err)
	}
	path := filepath.Join(home, ".motley", "config.toml")
	if _, err := os.Lstat(path); err == nil {
		return path, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	legacy, err := Dir()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(legacy, "config.toml"))
	if os.IsNotExist(err) {
		data = []byte(defaultConfig)
	} else if err != nil {
		return "", fmt.Errorf("read existing config: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("create config directory: %w", err)
	}
	// Publish a complete file without overwriting a concurrent first launch's file.
	f, err := os.CreateTemp(dir, ".config-*")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	if err := os.Link(f.Name(), path); err != nil && !os.IsExist(err) {
		return "", fmt.Errorf("create config %s: %w", path, err)
	}
	return path, nil
}
