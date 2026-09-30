package blueprint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderGolden(t *testing.T) {
	src, err := os.ReadFile("testdata/render.md")
	if err != nil {
		t.Fatal(err)
	}
	b, err := Parse("testdata/render.md", src)
	if err != nil {
		t.Fatal(err)
	}
	got, err := b.Render(Data{Repo: "api", Name: "FX cache", Ticket: "412", Branch: "feat/412-cache", Base: "main", Worktree: "/work trees/api/feat-412-cache", Crew: Crew{Title: "FX & Banking", URL: "https://example.com/work", Kind: "link"}, Vars: map[string]string{"constraints": "first line\nsecond line, x=y"}})
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/render.golden")
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatalf("render:\n%s\nwant:\n%s", got, want)
	}
	empty, err := b.Render(Data{})
	if err != nil || strings.Contains(empty, "[<no value>]") || !strings.Contains(empty, "No ticket") {
		t.Fatal(empty, err)
	}
}
func TestParseAndRenderErrors(t *testing.T) {
	for _, src := range []string{"no front matter", "+++\nname='a'", "+++\nschema=2\n+++\nhi", "+++\nbroken=[\n+++\nhi", "+++\nagent='bad'\n+++\nhi", "+++\nname='../bad'\n+++\nhi", "+++\n+++\n{{unknown .Repo}}"} {
		if _, err := Parse("test.md", []byte(src)); err == nil {
			t.Fatalf("accepted %q", src)
		}
	}
	for _, body := range []string{"{{.UnknownField}}", "{{.Vars.large}}", "nul\x00"} {
		b, err := Parse("test.md", []byte("+++\n+++\n"+body))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := b.Render(Data{Vars: map[string]string{"large": strings.Repeat("x", MaxPromptBytes+1)}}); err == nil {
			t.Fatalf("render accepted %q", body)
		}
	}
	b, err := Parse("default.md", []byte("+++\r\nfuture=true\r\n+++\r\n{{.Vars.missing}}text\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := b.Render(Data{})
	if err != nil || b.Name != "default" || b.Agent != "claude" || got != "text\r\n" {
		t.Fatal(b, got, err)
	}
}
func write(t *testing.T, path, src string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(src), 0600); err != nil {
		t.Fatal(err)
	}
}
func TestDiscoveryAndVariables(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	repo := filepath.Join(root, "api")
	global := filepath.Join(root, "config/colony/blueprints")
	local := filepath.Join(repo, ".colony/blueprints")
	write(t, filepath.Join(global, "base.md"), "+++\nname='plan'\n+++\nglobal")
	write(t, filepath.Join(local, "override.md"), "+++\nname='plan'\nrepos=['api']\n+++\nlocal")
	write(t, filepath.Join(global, "restricted.md"), "+++\nrepos=['other']\n+++\nrestricted")
	bs, err := Discover(repo, "api")
	if err != nil || len(bs) != 1 {
		t.Fatal(bs, err)
	}
	b, err := Find(bs, "plan")
	if err != nil || b.Path != filepath.Join(local, "override.md") {
		t.Fatal(b, err)
	}
	got, err := b.Render(Data{})
	if err != nil || got != "local" {
		t.Fatal(got, err)
	}
	bs, err = Discover("", "")
	if err != nil || len(bs) != 2 {
		t.Fatal(bs, err)
	}
	// A restricted repository override masks the global definition as well.
	bs, err = Discover(repo, "other")
	if err != nil || len(bs) != 1 || bs[0].Name != "restricted" {
		t.Fatal(bs, err)
	}
	write(t, filepath.Join(local, "duplicate.md"), "+++\nname='plan'\n+++\nduplicate")
	if _, err := Discover(repo, "api"); err == nil {
		t.Fatal("duplicate name accepted")
	}
	vars, err := Variables([]string{"x=first", "x=second,a=b", "empty="})
	if err != nil || vars["x"] != "second,a=b" || vars["empty"] != "" {
		t.Fatal(vars, err)
	}
	for _, bad := range []string{"no-equals", "=value", "bad name=x"} {
		if _, err := Variables([]string{bad}); err == nil {
			t.Fatal("accepted", bad)
		}
	}
}
func TestPromptSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prompt.md")
	for _, tc := range []struct {
		data string
		bad  bool
	}{{PromptHeader + "literal\n", false}, {"<!-- schema = 2 -->\nhi", true}, {PromptHeader + "nul\x00", true}} {
		write(t, path, tc.data)
		got, err := ReadPrompt(path)
		if (err != nil) != tc.bad {
			t.Fatal(err)
		}
		if !tc.bad && got != "literal\n" {
			t.Fatal(got)
		}
	}
}
