package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

const issueFixture = `{"data":{"repository":{"issue":{"title":"Cache FX rates","body":"Body text","url":"https://github.com/acme/api/issues/412","parent":{"title":"Epic: FX","url":"https://github.com/acme/api/issues/400"},"milestone":null}}}}`

// motleyCombined returns stdout and stderr; spawn notes and warnings go to stderr.
func (f *memberFixture) motleyCombined(args ...string) string {
	f.t.Helper()
	return commandOutput(f.t, f.bin, args...)
}

// useGitHubRemote names origin like GitHub while a fake ssh serves the local
// bare repository, so spawn's fetch and push work and links see GitHub.
func (f *memberFixture) useGitHubRemote() {
	f.t.Helper()
	f.git(f.repo, "remote", "set-url", "origin", "git@github.com:acme/api.git")
	script := filepath.Join(f.home, "fake agents", "fake-ssh")
	writeFixture(f.t, script, "#!/bin/sh\nfor last; do :; done\nexec /bin/sh -c \"$(printf '%s' \"$last\" | /usr/bin/sed \"s#'acme/api.git'#'"+f.remote+"'#\")\"\n", 0o755)
	f.t.Setenv("GIT_SSH_COMMAND", quoteShell(script))
	f.t.Setenv("GIT_SSH_VARIANT", "simple")
}

// fakeGH installs a gh that logs each call to $HOME/gh-calls.txt and serves
// canned JSON from $HOME/gh/<name>.
func (f *memberFixture) fakeGH(files map[string]string) {
	f.t.Helper()
	for name, body := range files {
		writeFixture(f.t, filepath.Join(f.home, "gh", name), body, 0o644)
	}
	writeFixture(f.t, filepath.Join(f.home, "fake agents", "gh"), `#!/bin/sh
printf '%s\n' "$*" >> "$HOME/gh-calls.txt"
case "$*" in
  "api graphql"*) cat "$HOME/gh/issue.json" ;;
  "project view"*) cat "$HOME/gh/project.json" ;;
  "pr list"*"--state open"*) cat "$HOME/gh/open.json" 2>/dev/null || echo '[]' ;;
  "pr list"*) cat "$HOME/gh/prs.json" 2>/dev/null || echo '[]' ;;
  "pr create"*) echo "https://github.com/acme/api/pull/7" ;;
  "pr ready"*) : ;;
  *) echo "unexpected gh $*" >&2; exit 1 ;;
esac
`, 0o755)
}

func (f *memberFixture) ghCalls() int {
	f.t.Helper()
	data, err := os.ReadFile(filepath.Join(f.home, "gh-calls.txt"))
	if os.IsNotExist(err) {
		return 0
	} else if err != nil {
		f.t.Fatal(err)
	}
	return strings.Count(string(data), "\n")
}

// withoutGH replaces PATH with the fake agents plus a tools directory linking
// git, tmux and every system command except gh, which CI images often ship in
// /usr/bin.
func (f *memberFixture) withoutGH() {
	f.t.Helper()
	tools := filepath.Join(f.home, "tools")
	if err := os.MkdirAll(tools, 0o755); err != nil {
		f.t.Fatal(err)
	}
	link := func(name, target string) {
		if name == "gh" {
			return
		}
		if _, err := os.Lstat(filepath.Join(tools, name)); err == nil {
			return
		}
		if err := os.Symlink(target, filepath.Join(tools, name)); err != nil {
			f.t.Fatal(err)
		}
	}
	for _, name := range []string{"git", "tmux"} {
		path, err := exec.LookPath(name)
		if err != nil {
			f.t.Fatal(err)
		}
		link(name, path)
	}
	for _, dir := range []string{"/usr/bin", "/bin"} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			f.t.Fatal(err)
		}
		for _, e := range entries {
			link(e.Name(), filepath.Join(dir, e.Name()))
		}
	}
	_ = os.Remove(filepath.Join(f.home, "fake agents", "gh"))
	f.t.Setenv("PATH", filepath.Join(f.home, "fake agents")+string(os.PathListSeparator)+tools)
	if path, err := exec.LookPath("gh"); err == nil {
		f.t.Fatalf("gh still resolvable at %s; cannot test its absence", path)
	}
}

func TestSpawnIssueLookupAndCrews(t *testing.T) {
	f := newMemberFixture(t, buildLifecycleBinary(t), "main")
	f.useGitHubRemote()
	f.fakeGH(map[string]string{"issue.json": issueFixture})
	writeFixture(t, filepath.Join(f.repo, ".motley/blueprints/issue.md"), "+++\n+++\n{{.Issue.Title}} | {{.Issue.URL}} | {{.Crew.Title}}\n{{.Issue.Body}}", 0o644)

	out := f.motleyCombined("spawn", "--repo", "api", "--branch", "feat/412-fx", "--ticket", "412", "--name", "FX", "--blueprint", "issue", "--detach")
	for _, want := range []string{"Created 412-fx", "Issue #412: Cache FX rates", `Suggested crew from parent issue "Epic: FX":`, "motley crew add --url 'https://github.com/acme/api/issues/400' --title 'Epic: FX' && motley crew assign 412-fx epic-fx", "or spawn with --create-crew"} {
		if !strings.Contains(out, want) {
			t.Fatalf("spawn output lacks %q:\n%s", want, out)
		}
	}
	m := f.manifest("412-fx")
	if m.Issue == nil || m.Issue.Title != "Cache FX rates" || m.Issue.URL != "https://github.com/acme/api/issues/412" || m.Crew != "" {
		t.Fatalf("manifest: %+v crew=%q", m.Issue, m.Crew)
	}
	prompt, err := os.ReadFile(filepath.Join(f.state, "motley/members/412-fx.prompt.md"))
	if err != nil || !strings.Contains(string(prompt), "Cache FX rates | https://github.com/acme/api/issues/412 | \nBody text") {
		t.Fatalf("prompt: %v\n%s", err, prompt)
	}

	out = f.motleyCombined("spawn", "--repo", "api", "--branch", "feat/412-fx-2", "--ticket", "412", "--name", "FX two", "--create-crew", "--detach")
	if !strings.Contains(out, `Crew: created "Epic: FX" from parent issue`) || f.manifest("412-fx-two").Crew != "epic-fx" {
		t.Fatalf("create crew:\n%s", out)
	}
	out = f.motleyCombined("spawn", "--repo", "api", "--branch", "feat/412-fx-3", "--ticket", "412", "--name", "FX three", "--create-crew", "--detach")
	if !strings.Contains(out, "Crew: epic-fx (matches parent issue https://github.com/acme/api/issues/400)") || f.manifest("412-fx-three").Crew != "epic-fx" {
		t.Fatalf("reuse crew:\n%s", out)
	}
	if crews := f.motley("crew", "list"); strings.Count(crews, "epic-fx") != 1 {
		t.Fatalf("crew created twice:\n%s", crews)
	}

	before := f.ghCalls()
	out = f.motleyCombined("spawn", "--repo", "api", "--branch", "feat/412-nogh", "--ticket", "412", "--name", "No gh", "--no-gh", "--detach")
	if f.ghCalls() != before || f.manifest("412-no-gh").Issue != nil || strings.Contains(out, "Issue #") {
		t.Fatalf("--no-gh must not call gh:\n%s", out)
	}
	before = f.ghCalls()
	f.motley("spawn", "--repo", "api", "--branch", "feat/plain", "--detach")
	if f.ghCalls() != before {
		t.Fatal("a spawn without a numeric ticket must not call gh")
	}
}

func TestSpawnWithoutGHStillWorks(t *testing.T) {
	f := newMemberFixture(t, buildLifecycleBinary(t), "main")
	f.useGitHubRemote()
	f.withoutGH()
	out := f.motleyCombined("spawn", "--repo", "api", "--branch", "feat/7-x", "--ticket", "7", "--name", "x", "--detach")
	if !strings.Contains(out, "warning: issue 7 lookup skipped: GitHub CLI (gh) not found; install gh for issue and PR data") || !strings.Contains(out, "Created 7-x") {
		t.Fatalf("%s", out)
	}
	if strings.Count(out, "warning:") != 1 {
		t.Fatalf("one warning only:\n%s", out)
	}
}

func TestCrewAddFetchesGitHubTitle(t *testing.T) {
	f := newMemberFixture(t, buildLifecycleBinary(t), "main")
	f.fakeGH(map[string]string{"issue.json": issueFixture, "project.json": `{"title":"Q4 platform"}`})
	if out := f.motley("crew", "add", "--url", "https://github.com/acme/api/issues/412"); !strings.Contains(out, "Created crew cache-fx-rates (Cache FX rates,") {
		t.Fatal(out)
	}
	if out := f.motley("crew", "add", "--url", "https://github.com/orgs/acme/projects/7"); !strings.Contains(out, "Created crew q4-platform (Q4 platform,") {
		t.Fatal(out)
	}
	before := f.ghCalls()
	if out := f.motley("crew", "add", "--title", "Manual", "--url", "https://github.com/acme/api/issues/1"); !strings.Contains(out, "(Manual,") || f.ghCalls() != before {
		t.Fatalf("an explicit title must not call gh: %s", out)
	}
	for _, args := range [][]string{{"crew", "add", "--url", "https://example.com/x"}, {"crew", "add"}} {
		out, err := exec.Command(f.bin, args...).CombinedOutput()
		if err == nil || !strings.Contains(string(out), "required") {
			t.Fatalf("%v: %v %s", args, err, out)
		}
	}
}

func TestCrewAddWithoutGH(t *testing.T) {
	f := newMemberFixture(t, buildLifecycleBinary(t), "main")
	f.withoutGH()
	out, err := exec.Command(f.bin, "crew", "add", "--url", "https://github.com/acme/api/issues/412").CombinedOutput()
	if err == nil || !strings.Contains(string(out), "GitHub CLI (gh) not found") || !strings.Contains(string(out), "--title") {
		t.Fatalf("%v %s", err, out)
	}
}

func TestSpawnFormIssueLookupTerminal(t *testing.T) {
	bin := buildLifecycleBinary(t)
	f := newMemberFixture(t, bin, "main")
	f.useGitHubRemote()
	f.fakeGH(map[string]string{"issue.json": issueFixture})
	terminal := startTerminal(t, exec.Command(bin))
	defer func() {
		if t.Failed() {
			t.Log(ansi.Strip(terminal.text()))
		}
	}()
	send := func(keys, want string) {
		t.Helper()
		offset := len(terminal.text())
		terminal.send(t, keys)
		eventually(t, func() bool { return strings.Contains(ansi.Strip(terminal.text()[offset:]), want) })
	}
	eventually(t, func() bool { return strings.Contains(terminal.text(), "No members yet") })
	send("s", "> api")
	send("api\r", "Ticket and name")
	send("412\tFX\r", `> Create crew "Epic: FX" and assign`)
	send("\r", "Agent")
	send("\r", "Blueprint")
	send("\r", "Permission mode")
	send("\r", "#412 Cache FX rates · new crew Epic: FX")
	send("\r", "Created 412-fx")
	m := f.manifest("412-fx")
	if m.Crew != "epic-fx" || m.Issue == nil || m.Issue.Title != "Cache FX rates" {
		t.Fatalf("crew=%q issue=%+v", m.Crew, m.Issue)
	}
	if calls := f.ghCalls(); calls != 1 {
		t.Fatalf("gh called %d times; the TUI lookup must not repeat in Prepare", calls)
	}
	terminal.send(t, "q")
	eventually(t, func() bool {
		select {
		case err := <-terminal.done:
			return err == nil
		default:
			return false
		}
	})
}

func TestRetireWarnsAboutOpenPR(t *testing.T) {
	f := newMemberFixture(t, buildLifecycleBinary(t), "main")
	f.useGitHubRemote()
	f.fakeGH(map[string]string{"open.json": `[{"number":7,"url":"https://github.com/acme/api/pull/7","state":"OPEN","headRefName":"feat/pr","statusCheckRollup":[]}]`})
	f.motley("spawn", "--repo", "api", "--branch", "feat/pr", "--detach", "--no-gh")
	out := f.motleyCombined("retire", "feat-pr")
	if !strings.Contains(out, "warning: PR #7 is still open: https://github.com/acme/api/pull/7") || !strings.Contains(out, "Retired feat-pr") {
		t.Fatal(out)
	}
}

func TestRetireWithoutGH(t *testing.T) {
	f := newMemberFixture(t, buildLifecycleBinary(t), "main")
	f.useGitHubRemote()
	f.motley("spawn", "--repo", "api", "--branch", "feat/nogh", "--detach", "--no-gh")
	f.withoutGH()
	if out := f.motleyCombined("retire", "feat-nogh"); strings.Contains(out, "warning") || !strings.Contains(out, "Retired feat-nogh") {
		t.Fatal(out)
	}
}
