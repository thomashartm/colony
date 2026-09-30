package crew

import (
	"os"
	"path/filepath"
	"testing"
)

func TestKindAndValidation(t *testing.T) {
	for url, want := range map[string]string{"": "text", "https://github.com/acme/api/issues/42": "issue", "https://github.com/orgs/acme/projects/7": "project", "https://github.com/users/ada/projects/2/": "project", "https://example.com/tasks/42": "link", "https://github.com/acme/api/pull/42": "link", "https://github.com/orgs/acme/projects/not-a-number": "link", "https://github.com.evil.test/acme/api/issues/42": "link"} {
		if got := Kind(url); got != want {
			t.Errorf("%q: %s want %s", url, got, want)
		}
	}
	c := Crew{ID: "fx", Title: "FX & Banking", URL: "https://example.com", Color: "blue"}
	if err := Validate(c); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []Crew{{ID: "none", Title: "None", Color: "blue"}, {ID: "fx", Title: "", Color: "blue"}, {ID: "fx", Title: "Title", Color: "pink"}, {ID: "fx", Title: "Title", Color: "blue", URL: "javascript:alert(1)"}, {ID: "fx", Title: "bad\x1btitle", Color: "blue"}} {
		if err := Validate(invalid); err == nil {
			t.Fatal("invalid crew accepted", invalid)
		}
	}
}
func TestLoadDefaultsAndInvalidFiles(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_STATE_HOME", root)
	if crews, err := Load(); err != nil || len(crews) != 0 {
		t.Fatal(crews, err)
	}
	path := filepath.Join(root, "colony/crews.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		data string
		bad  bool
	}{
		{"[[crew]]\nid='fx'\ntitle='FX'\nfuture=true\n", false},
		{"broken = [", true}, {"schema=2", true},
		{"[[crew]]\nid='fx'\ntitle='FX'\ncolor='blue'\n[[crew]]\nid='fx'\ntitle='Other'\n", true},
	} {
		if err := os.WriteFile(path, []byte(tc.data), 0600); err != nil {
			t.Fatal(err)
		}
		crews, err := Load()
		if (err != nil) != tc.bad {
			t.Fatalf("load %q: %v", tc.data, err)
		}
		if !tc.bad && (len(crews) != 1 || crews[0].Kind != "text" || crews[0].Color == "") {
			t.Fatal("defaults missing", crews)
		}
	}
}
