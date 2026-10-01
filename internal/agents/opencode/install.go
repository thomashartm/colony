package opencode

import (
	"os"
	"path/filepath"

	opencodeplugin "github.com/thomashartm/motley/integrations/opencode"
	"github.com/thomashartm/motley/internal/agents/hookfile"
)

func Install() (path, backup string, changed bool, err error) {
	root := os.Getenv("XDG_CONFIG_HOME")
	if root == "" {
		var home string
		home, err = os.UserHomeDir()
		if err != nil {
			return
		}
		root = filepath.Join(home, ".config")
	}
	path = filepath.Join(root, "opencode", "plugins", "motley.ts")
	backup, changed, err = hookfile.Write(path, opencodeplugin.Plugin)
	return
}
