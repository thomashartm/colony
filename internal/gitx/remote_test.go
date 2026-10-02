package gitx

import "testing"

func TestWebURL(t *testing.T) {
	for _, tt := range []struct {
		remote, web string
		ok          bool
	}{
		{"git@github.com:AderisERP/aderis-api.git", "https://github.com/AderisERP/aderis-api", true},
		{"git@github.com:o/r", "https://github.com/o/r", true},
		{"https://github.com/o/r.git", "https://github.com/o/r", true},
		{"https://GitHub.com/o/r/", "https://github.com/o/r", true},
		{"ssh://git@github.com/o/r.git", "https://github.com/o/r", true},
		{"ssh://git@github.com:22/o/r.git", "https://github.com/o/r", true},
		{"https://gitlab.com/o/r.git", "", false},
		{"/tmp/origin.git", "", false},
		{"file:///tmp/origin.git", "", false},
		{"https://github.com/o", "", false},
		{"https://github.com/o/r/extra", "", false},
		{"git@github.com:o/r;rm.git", "", false},
		{"", "", false},
	} {
		got, ok := WebURL(tt.remote)
		if ok != tt.ok || got.Web != tt.web {
			t.Errorf("WebURL(%q) = %+v, %v; want %q, %v", tt.remote, got, ok, tt.web, tt.ok)
		}
	}
	r, _ := WebURL("git@github.com:o/r.git")
	if r.Owner != "o" || r.Name != "r" {
		t.Fatalf("owner/name: %+v", r)
	}
}

func TestBranchAndCompareURLs(t *testing.T) {
	r := Remote{Web: "https://github.com/o/r"}
	if got := r.BranchURL("feat/412-fx cache#1"); got != "https://github.com/o/r/tree/feat/412-fx%20cache%231" {
		t.Fatal(got)
	}
	if got := r.CompareURL("main", "feat/x"); got != "https://github.com/o/r/compare/main...feat/x" {
		t.Fatal(got)
	}
}

func TestWeb(t *testing.T) {
	for _, tt := range []struct{ remote, want string }{
		{"git@github.com:o/r.git", "https://github.com/o/r"},
		{"ssh://git@github.com:22/o/r.git", "https://github.com/o/r"},
		{"https://gitlab.com/group/sub/repo.git", "https://gitlab.com/group/sub/repo"},
		{"https://unknown.example/owner/repo/", "https://unknown.example/owner/repo"},
		{"http://GitHub.com/o/r", "https://github.com/o/r"},
		{"https://secret@github.com/o/r", ""},
		{"https://github.com/o", ""},
		{"/tmp/origin.git", ""},
		{"file:///tmp/origin.git", ""},
		{"git@github.com:o/r\x1b]52", ""},
		{"", ""},
	} {
		got := ""
		if u, ok := Web(tt.remote); ok {
			got = u.String()
		}
		if got != tt.want {
			t.Errorf("Web(%q) = %q; want %q", tt.remote, got, tt.want)
		}
	}
	if _, ok := WebURL("https://secret@github.com/o/r"); ok {
		t.Fatal("credentials in a remote must not yield GitHub links")
	}
}
