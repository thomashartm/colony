package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	codexhooks "github.com/thomashartm/motley/integrations/codex"
	"github.com/thomashartm/motley/internal/agents/hookfile"
)

func Install() (path, backup string, changed bool, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "codex", "features", "list").Output()
	if err != nil {
		return "", "", false, fmt.Errorf("check Codex native hooks: %w", err)
	}
	enabled := false
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) == 3 && f[0] == "hooks" && f[2] == "true" {
			enabled = true
		}
	}
	if !enabled {
		return "", "", false, fmt.Errorf("codex native hooks must be enabled; use a current Codex with features.hooks enabled (verified with 0.159.2)")
	}
	home := os.Getenv("CODEX_HOME")
	if home == "" {
		var h string
		h, err = os.UserHomeDir()
		if err != nil {
			return "", "", false, err
		}
		home = filepath.Join(h, ".codex")
	}
	path = filepath.Join(home, "hooks.json")
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return path, "", false, err
	}
	merged, err := merge(data)
	if err != nil {
		return path, "", false, err
	}
	backup, changed, err = hookfile.Write(path, merged)
	return
}

func merge(data []byte) ([]byte, error) {
	root := map[string]json.RawMessage{}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &root); err != nil || root == nil {
			return nil, fmt.Errorf("invalid Codex hooks object")
		}
	}
	hooks := map[string][]json.RawMessage{}
	if raw, ok := root["hooks"]; ok {
		if err := json.Unmarshal(raw, &hooks); err != nil || hooks == nil {
			return nil, fmt.Errorf("invalid Codex hooks table")
		}
	}
	var template struct {
		Hooks map[string][]json.RawMessage `json:"hooks"`
	}
	if err := json.Unmarshal(codexhooks.Settings, &template); err != nil {
		return nil, err
	}
	changed := false
	for event, desired := range template.Hooks {
		found := false
		for _, raw := range hooks[event] {
			var group struct {
				Matcher string `json:"matcher"`
				Hooks   []struct {
					Type    string `json:"type"`
					Command string `json:"command"`
				} `json:"hooks"`
			}
			if err := json.Unmarshal(raw, &group); err != nil {
				return nil, fmt.Errorf("invalid Codex %s hook group: %w", event, err)
			}
			if group.Matcher != "" && group.Matcher != "*" {
				continue
			}
			for _, h := range group.Hooks {
				if h.Type == "command" && h.Command == "motley report --agent codex" {
					found = true
				}
			}
		}
		if !found {
			hooks[event] = append(hooks[event], desired...)
			changed = true
		}
	}
	if !changed {
		return data, nil
	}
	root["hooks"], _ = json.Marshal(hooks)
	var description string
	if raw, ok := root["description"]; ok {
		if err := json.Unmarshal(raw, &description); err != nil {
			return nil, err
		}
	}
	if !strings.Contains(description, "motley schema = 1") {
		description = strings.TrimSpace(description + "\nAdded motley schema = 1 status hooks.")
	}
	root["description"], _ = json.Marshal(description)
	result, err := json.MarshalIndent(root, "", "  ")
	return append(result, '\n'), err
}
