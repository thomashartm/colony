package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// REQUIREMENTS §0: no AppleScript or Ghostty API. Motley controls terminals
// only through tmux, so no source may script another application.
func TestNoAppleScript(t *testing.T) {
	forbidden := regexp.MustCompile(`(?i)osascript|applescript|tell application|NSAppleScript`)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	self, err := filepath.Abs("policy_test.go")
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if name := d.Name(); path != root && (strings.HasPrefix(name, ".") || name == "node_modules" || name == "dist" || name == "tasks") {
				return filepath.SkipDir
			}
			return nil
		}
		switch filepath.Ext(path) {
		case ".go", ".sh", ".ts", ".mjs", ".js", ".scpt", ".applescript":
		default:
			return nil
		}
		if path == self {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		checked++
		if match := forbidden.Find(data); match != nil {
			rel, _ := filepath.Rel(root, path)
			t.Errorf("%s uses %q; Motley must not script other applications", rel, match)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked < 50 {
		t.Fatalf("checked only %d files; the walk is not covering the repository", checked)
	}
}
