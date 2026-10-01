package codex

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallPreservesSettingsAndBacksUp(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CODEX_HOME", dir)
	t.Setenv("PATH", dir)
	if err := os.WriteFile(filepath.Join(dir, "codex"), []byte("#!/bin/sh\necho 'hooks stable true'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	original := []byte(`{"description":"My hooks","custom":{"keep":true},"hooks":{"Stop":[{"hooks":[{"type":"command","command":"existing"}]}]}}`)
	path := filepath.Join(dir, "hooks.json")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	config := []byte("# leave config alone\nnotify=['existing']\n")
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), config, 0600); err != nil {
		t.Fatal(err)
	}
	got, backup, changed, err := Install()
	if err != nil || !changed || got != path || backup == "" {
		t.Fatal(got, backup, changed, err)
	}
	b, err := os.ReadFile(backup)
	if err != nil || !bytes.Equal(b, original) {
		t.Fatal("backup", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Description string
		Custom      map[string]bool
		Hooks       map[string][]json.RawMessage
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if !result.Custom["keep"] || len(result.Hooks["Stop"]) != 2 || len(result.Hooks["SessionStart"]) != 1 {
		t.Fatal(string(data))
	}
	_, backup, changed, err = Install()
	if err != nil || changed || backup != "" {
		t.Fatal("not idempotent", backup, err)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(data, after) {
		t.Fatal("second install changed file")
	}
	after, _ = os.ReadFile(filepath.Join(dir, "config.toml"))
	if !bytes.Equal(config, after) {
		t.Fatal("config overwritten")
	}
	if err := os.WriteFile(filepath.Join(dir, "codex"), []byte("#!/bin/sh\necho 'hooks stable false'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := Install(); err == nil {
		t.Fatal("enabled explicitly disabled hooks")
	}
	after, _ = os.ReadFile(path)
	if !bytes.Equal(data, after) {
		t.Fatal("disabled hook install changed file")
	}
	for _, bad := range []string{"invalid", "null", `{"hooks":[]}`, `{"hooks":{"Stop":[42]}}`} {
		if _, err := merge([]byte(bad)); err == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
}
