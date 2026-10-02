package member

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestManifestIssueRoundTripAndLegacy(t *testing.T) {
	dir := t.TempDir()
	m := Manifest{Schema: 1, ID: "412-fx", Name: "FX", Repo: "api", Branch: "feat/412-fx", Agent: "claude", CreatedAt: time.Unix(0, 0).UTC(),
		Issue: &IssueRef{Title: "Cache FX", URL: "https://github.com/o/r/issues/412"}}
	if err := saveManifest(dir, m); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "412-fx.toml"))
	if !strings.Contains(string(data), "[issue]") || strings.Index(string(data), "[issue]") < strings.Index(string(data), "agent =") {
		t.Fatalf("issue table must follow scalar keys:\n%s", data)
	}
	got, err := Load(dir, "412-fx")
	if err != nil || got.Issue == nil || got.Issue.Title != "Cache FX" || got.Issue.URL != "https://github.com/o/r/issues/412" {
		t.Fatalf("%+v %v", got.Issue, err)
	}
	legacy := "schema = 1\nid = 'old'\nname = 'Old'\nagent = 'claude'\nfuture = 1\n"
	if err := os.WriteFile(filepath.Join(dir, "old.toml"), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	if old, err := Load(dir, "old"); err != nil || old.Issue != nil {
		t.Fatalf("legacy manifest: %+v %v", old, err)
	}
}
