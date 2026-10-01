package opencode

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPluginInstall(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path, backup, changed, err := Install()
	if err != nil || !changed || backup != "" || path != filepath.Join(dir, "opencode/plugins/motley.ts") {
		t.Fatal(path, backup, changed, err)
	}
	_, backup, changed, err = Install()
	if err != nil || changed || backup != "" {
		t.Fatal("not idempotent", err)
	}
	if err := os.WriteFile(path, []byte("// previous plugin"), 0600); err != nil {
		t.Fatal(err)
	}
	_, backup, changed, err = Install()
	if err != nil || !changed || backup == "" {
		t.Fatal("no backup", err)
	}
	old, _ := os.ReadFile(backup)
	if string(old) != "// previous plugin" {
		t.Fatal(string(old))
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "// schema = 1") {
		t.Fatal("missing schema")
	}
}
