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
