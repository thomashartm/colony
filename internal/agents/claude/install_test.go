package claude

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallPreservesSettingsAndIsIdempotent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, ".claude/settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	original := []byte(`{"permissions":{"allow":["Read"]},"custom":{"keep":true},"hooks":{"Stop":[{"matcher":"","hooks":[{"type":"command","command":"echo existing"}]}],"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"motley report --agent claude"}]}]}}`)
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	got, backup, changed, err := Install()
	if err != nil || !changed || got != path || backup == "" {
		t.Fatalf("install: %s %s %v %v", got, backup, changed, err)
	}
	saved, err := os.ReadFile(backup)
	if err != nil || !bytes.Equal(saved, original) {
		t.Fatal("backup must match original bytes", err)
	}
	installed, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(installed, &root); err != nil {
		t.Fatal(err)
	}
	if string(root["permissions"]) == "" || !bytes.Contains(root["custom"], []byte("true")) {
		t.Fatal("unrelated settings lost")
	}
	var hooks map[string][]struct {
		Matcher string
		Hooks   []struct {
			Type, Command string
			Timeout       int
		}
	}
	if err := json.Unmarshal(root["hooks"], &hooks); err != nil {
		t.Fatal(err)
	}
	if len(hooks) != 7 || len(hooks["Stop"]) != 2 || hooks["Stop"][0].Hooks[0].Command != "echo existing" || len(hooks["PreToolUse"]) != 2 {
		t.Fatalf("hooks not merged: %s", installed)
	}
	for event, groups := range hooks {
		count := 0
		for _, g := range groups {
			if g.Matcher == "" {
				for _, h := range g.Hooks {
					if h.Command == hookCommand && h.Type == "command" && h.Timeout == 1 {
						count++
					}
				}
			}
		}
		if count != 1 {
			t.Fatalf("%s has %d motley hooks", event, count)
		}
	}
	_, backup, changed, err = Install()
	again, readErr := os.ReadFile(path)
	if err != nil || readErr != nil || changed || backup != "" || !bytes.Equal(installed, again) {
		t.Fatal("second install changed settings", err, readErr)
	}
}
func TestInstallNewAndInvalidSettings(t *testing.T) {
	for _, input := range []string{"", "not json", "null", `{"hooks":42}`} {
		t.Run(input, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			if input == "" {
				_, backup, changed, err := Install()
				if err != nil || backup != "" || !changed {
					t.Fatal("new installation", err)
				}
				return
			}
			path := filepath.Join(os.Getenv("HOME"), ".claude/settings.json")
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(input), 0600); err != nil {
				t.Fatal(err)
			}
			if _, _, _, err := Install(); err == nil {
				t.Fatal("accepted invalid settings")
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != input {
				t.Fatal("invalid settings overwritten")
			}
		})
	}
}
