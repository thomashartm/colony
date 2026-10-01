package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	claudehooks "github.com/thomashartm/motley/integrations/claude"
	"github.com/thomashartm/motley/internal/state"
)

const hookCommand = "motley report --agent claude"

func Install() (path, backup string, changed bool, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", false, err
	}
	path = filepath.Join(home, ".claude", "settings.json")
	original, err := os.ReadFile(path)
	exists := err == nil
	if err != nil && !os.IsNotExist(err) {
		return path, "", false, err
	}
	root := map[string]json.RawMessage{}
	if exists {
		if err = json.Unmarshal(original, &root); err != nil || root == nil {
			return path, "", false, fmt.Errorf("invalid Claude settings object: %s", path)
		}
	}
	hooks := map[string]json.RawMessage{}
	if raw, ok := root["hooks"]; ok {
		if err = json.Unmarshal(raw, &hooks); err != nil || hooks == nil {
			return path, "", false, fmt.Errorf("claude settings hooks must be an object")
		}
	}
	var template struct {
		Hooks map[string]json.RawMessage `json:"hooks"`
	}
	if err = json.Unmarshal(claudehooks.Settings, &template); err != nil {
		return path, "", false, err
	}
	for event, desired := range template.Hooks {
		var groups []map[string]json.RawMessage
		if raw, ok := hooks[event]; ok {
			if err = json.Unmarshal(raw, &groups); err != nil {
				return path, "", false, fmt.Errorf("claude %s hooks: %w", event, err)
			}
		}
		found := false
		for _, group := range groups {
			var matcher string
			if raw, ok := group["matcher"]; ok {
				if json.Unmarshal(raw, &matcher) != nil {
					continue
				}
			}
			if matcher != "" && matcher != "*" {
				continue
			}
			var handlers []struct {
				Type    string `json:"type"`
				Command string `json:"command"`
			}
			if json.Unmarshal(group["hooks"], &handlers) != nil {
				continue
			}
			for _, h := range handlers {
				if h.Type == "command" && h.Command == hookCommand {
					found = true
				}
			}
		}
		if found {
			continue
		}
		var addition []map[string]json.RawMessage
		if err = json.Unmarshal(desired, &addition); err != nil {
			return path, "", false, err
		}
		groups = append(groups, addition...)
		hooks[event], err = json.Marshal(groups)
		if err != nil {
			return path, "", false, err
		}
		changed = true
	}
	if !changed {
		return path, "", false, nil
	}
	root["hooks"], err = json.Marshal(hooks)
	if err != nil {
		return path, "", false, err
	}
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return path, "", false, err
	}
	data = append(data, '\n')
	if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return path, "", false, err
	}
	if exists {
		backup = path + ".motley-v1-backup-" + time.Now().UTC().Format("20060102T150405.000000000Z")
		f, e := os.OpenFile(backup, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if e != nil {
			return path, "", false, e
		}
		_, e = f.Write(original)
		if e == nil {
			e = f.Sync()
		}
		closeErr := f.Close()
		if e == nil {
			e = closeErr
		}
		if e != nil {
			return path, backup, false, e
		}
	}
	err = state.WriteAtomic(path, data)
	return path, backup, err == nil, err
}
