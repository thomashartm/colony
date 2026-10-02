package config

import (
	"os"
	"strings"
	"testing"
)

func TestInitPreservesUserFiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path, err := Init()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "schema = 1") || !strings.Contains(string(data), "#{q:client_name}") {
		t.Fatalf("popup config %q %v", data, err)
	}
	configPath, err := Ensure()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{path, configPath} {
		if err := os.WriteFile(p, []byte("user settings"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Init(); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{path, configPath} {
		data, err := os.ReadFile(p)
		if err != nil || string(data) != "user settings" {
			t.Fatalf("init replaced %s", p)
		}
	}
}
