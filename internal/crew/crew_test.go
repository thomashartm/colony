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
	path := filepath.Join(root, "motley/crews.toml")
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

func TestFindURLNormalises(t *testing.T) {
	crews := []Crew{{ID: "fx", URL: "https://GitHub.com/o/r/issues/400/"}, {ID: "none-url"}}
	for _, raw := range []string{"https://github.com/o/r/issues/400", "https://github.com/o/r/issues/400?x=1#c", "HTTPS://github.com/o/r/issues/400/"} {
		if c, ok := FindURL(crews, raw); !ok || c.ID != "fx" {
			t.Fatalf("%s: %+v %v", raw, c, ok)
		}
	}
	if _, ok := FindURL(crews, "https://github.com/o/r/issues/40"); ok {
		t.Fatal("different issue matched")
	}
	if _, ok := FindURL(crews, ""); ok {
		t.Fatal("an empty URL must not match a crew without a URL")
	}
}

func TestGitHubRef(t *testing.T) {
	for _, tt := range []struct {
		raw, kind, owner, repo string
		n                      int
	}{
		{"https://github.com/o/r/issues/12", "issue", "o", "r", 12},
		{"https://github.com/orgs/acme/projects/7", "project", "acme", "", 7},
		{"https://github.com/users/thomas/projects/2", "project", "thomas", "", 2},
		{"https://github.com/o/r/milestone/3", "link", "", "", 0},
		{"https://example.com/o/r/issues/12", "link", "", "", 0},
		{"", "text", "", "", 0},
	} {
		kind, owner, repo, n := GitHubRef(tt.raw)
		if kind != tt.kind || owner != tt.owner || repo != tt.repo || n != tt.n || Kind(tt.raw) != tt.kind {
			t.Errorf("%s: %s %s %s %d", tt.raw, kind, owner, repo, n)
		}
	}
}
