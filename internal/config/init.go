package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/thomashartm/colony/internal/tmux"
)

func Dir() (string, error) {
	root := os.Getenv("XDG_CONFIG_HOME")
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(home, ".config")
	}
	return filepath.Abs(filepath.Join(root, "colony"))
}

// Init scaffolds only missing files; existing user settings are never replaced.
func Init() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	for _, file := range []struct{ name, body string }{
		{"config.toml", "schema = 1\nrepos_root = \"~/projects\"\nworktrees_root = \"~/worktrees\"\n"},
		{"colony.tmux.conf", "# schema = 1\n# run-shell expands the originating client before opening the popup.\nbind h run-shell 'tmux display-popup -c #{q:client_name} -E -w 90% -h 85% \"colony --client #{q:client_name}\"'\nset -g status-interval 2\nset -g status-left-length 50\nset -g status-left \"" + tmux.StatusLeft + "\"\n"},
	} {
		path := filepath.Join(dir, file.name)
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("create %s: %w", path, err)
		}
		_, writeErr := f.WriteString(file.body)
		closeErr := f.Close()
		if writeErr != nil {
			return "", writeErr
		}
		if closeErr != nil {
			return "", closeErr
		}
	}
	return filepath.Join(dir, "colony.tmux.conf"), nil
}
