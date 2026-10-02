package ghostty

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
)

// fakeGhostty answers the list and focus scripts. The live terminal's title is
// the last OSC 2 title written to the fake tty, as Ghostty would show it.
type fakeGhostty struct {
	terminals []Terminal
	live      string // id of the terminal attached to the tty; "" when none is
	tty       string
	retitle   int // lists, the first included, that report the agent's own title
	focused   []string
	fail      error
}

func (g *fakeGhostty) run(_ context.Context, script string, args ...string) (string, error) {
	if g.fail != nil {
		return "", g.fail
	}
	if script == focusScript {
		g.focused = append(g.focused, args...)
		return "", nil
	}
	if script != listScript {
		return "", errors.New("unexpected script")
	}
	var out strings.Builder
	for _, t := range g.terminals {
		if t.ID == g.live {
			if title, ok := lastTitle(g.tty); ok && g.retitle == 0 {
				t.Name = title
			}
		}
		out.WriteString(t.ID + fieldSep + t.Dir + fieldSep + t.Name + recordSep)
	}
	if g.retitle > 0 {
		g.retitle--
	}
	return out.String(), nil
}

func lastTitle(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return "", false
	}
	sequences := strings.Split(strings.TrimSuffix(string(data), "\a"), "\a")
	return strings.TrimPrefix(sequences[len(sequences)-1], "\x1b]2;"), true
}

// setup installs the fake and a ps reporting tty for every pid.
func setup(t *testing.T, g *fakeGhostty, tty string) {
	t.Helper()
	dev := t.TempDir()
	g.tty = filepath.Join(dev, "ttys007")
	if err := os.WriteFile(g.tty, nil, 0600); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "ps"), []byte("#!/bin/sh\ntest \"$1 $2 $3\" = '-o tty= -p' || exit 2\necho '"+tty+"'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	oldRun, oldDev, oldWithin := osascript, devDir, locateWithin
	osascript, devDir, locateWithin = g.run, dev, 300*time.Millisecond
	t.Cleanup(func() { osascript, devDir, locateWithin = oldRun, oldDev, oldWithin })
}

func written(t *testing.T, g *fakeGhostty) string {
	t.Helper()
	data, err := os.ReadFile(g.tty)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func inBackend(dir string) bool { return dir == "/w/backend" }

var markSequence = regexp.MustCompile(`^\x1b\]2;motley-[0-9a-f]{16}\a$`)

func TestFocusProcessUniqueDirectoryNeedsNoTTY(t *testing.T) {
	g := &fakeGhostty{terminals: []Terminal{{"a", "/w/other", "x"}, {"b", "/w/backend", "✳ Work"}}}
	setup(t, g, "not a tty") // ps must not even be consulted
	if err := FocusProcess(context.Background(), 4242, inBackend); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(g.focused, []string{"b"}) || written(t, g) != "" {
		t.Fatalf("focused %v, wrote %q", g.focused, written(t, g))
	}
}

func TestFocusProcessResolvesSharedDirectoryByTTY(t *testing.T) {
	g := &fakeGhostty{
		terminals: []Terminal{{"a", "/w/backend", "zsh"}, {"b", "/w/backend", "✳ Sales search"}, {"c", "/w/backend", "◐ Other"}},
		live:      "b",
	}
	setup(t, g, "ttys007")
	if err := FocusProcess(context.Background(), 4242, inBackend); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(g.focused, []string{"b"}) {
		t.Fatalf("focused %v", g.focused)
	}
	out := written(t, g)
	mark, restore, ok := strings.Cut(out, "\a")
	if !ok || !markSequence.MatchString(mark+"\a") || restore != "\x1b]2;✳ Sales search\a" {
		t.Fatalf("tty writes %q", out)
	}
}

// A working agent retitles its terminal; the mark is rewritten until seen.
func TestFocusProcessRewritesMarkOverAgentTitles(t *testing.T) {
	g := &fakeGhostty{
		terminals: []Terminal{{"a", "/w/backend", "zsh"}, {"b", "/w/backend", "◐ Busy"}},
		live:      "b", retitle: 3, // the first list, then two polls miss the mark
	}
	setup(t, g, "ttys007")
	if err := FocusProcess(context.Background(), 4242, inBackend); err != nil {
		t.Fatal(err)
	}
	sequences := strings.SplitAfter(written(t, g), "\a")
	sequences = sequences[:len(sequences)-1]
	if len(sequences) != 4 || sequences[3] != "\x1b]2;◐ Busy\a" || !reflect.DeepEqual(g.focused, []string{"b"}) {
		t.Fatalf("writes %q focused %v", sequences, g.focused)
	}
	for _, s := range sequences[:3] {
		if s != sequences[0] || !markSequence.MatchString(s) {
			t.Fatalf("marks %q", sequences)
		}
	}
}

// Without shell integration Ghostty reports no directory; the tty still knows.
func TestFocusProcessFindsTerminalWithoutDirectory(t *testing.T) {
	g := &fakeGhostty{terminals: []Terminal{{"a", "", "zsh"}, {"b", "", "claude"}}, live: "b"}
	setup(t, g, "ttys007")
	if err := FocusProcess(context.Background(), 4242, inBackend); err != nil || !reflect.DeepEqual(g.focused, []string{"b"}) {
		t.Fatalf("focused %v: %v", g.focused, err)
	}
}

func TestFocusProcessRefusesInvisibleSession(t *testing.T) {
	g := &fakeGhostty{terminals: []Terminal{{"a", "/w/backend", "zsh"}, {"b", "/w/backend", "zsh"}}}
	setup(t, g, "ttys007")
	err := FocusProcess(context.Background(), 4242, inBackend)
	if err == nil || !strings.Contains(err.Error(), "switch to it manually") || len(g.focused) != 0 {
		t.Fatalf("focused %v: %v", g.focused, err)
	}
	if !strings.HasSuffix(written(t, g), "\a\x1b]2;\a") {
		t.Fatalf("mark left behind: %q", written(t, g))
	}
}

func TestFocusProcessRefusesUnknownTTY(t *testing.T) {
	for _, tty := range []string{"??", "", "ttys00/../disk0", "console"} {
		g := &fakeGhostty{terminals: []Terminal{{"a", "/w/backend", "zsh"}, {"b", "/w/backend", "zsh"}}, live: "b"}
		setup(t, g, tty)
		err := FocusProcess(context.Background(), 4242, inBackend)
		if err == nil || !strings.Contains(err.Error(), "cannot find the terminal") || written(t, g) != "" || len(g.focused) != 0 {
			t.Fatalf("%q: focused %v wrote %q: %v", tty, g.focused, written(t, g), err)
		}
	}
	g := &fakeGhostty{terminals: []Terminal{{"a", "/w/backend", "zsh"}, {"b", "/w/backend", "zsh"}}}
	setup(t, g, "ttys007")
	if err := FocusProcess(context.Background(), 1, inBackend); err == nil || written(t, g) != "" {
		t.Fatalf("pid 1 accepted: %v", err)
	}
}

func TestRestoredTitleCannotEndTheSequence(t *testing.T) {
	g := &fakeGhostty{terminals: []Terminal{{"a", "/w/backend", "zsh"}, {"b", "/w/backend", "evil\a\x1b]2;x\u009c"}}, live: "b"}
	setup(t, g, "ttys007")
	if err := FocusProcess(context.Background(), 4242, inBackend); err != nil {
		t.Fatal(err)
	}
	if out := written(t, g); !strings.HasSuffix(out, "\a\x1b]2;evil]2;x\a") {
		t.Fatalf("tty writes %q", out)
	}
}

func TestTerminalsParsing(t *testing.T) {
	g := &fakeGhostty{}
	setup(t, g, "ttys007")
	osascript = func(context.Context, string, ...string) (string, error) {
		return "id1" + fieldSep + "/w/a b" + fieldSep + "tab\twith tab" + recordSep + "id2" + fieldSep + fieldSep + recordSep, nil
	}
	got, err := Terminals(context.Background())
	want := []Terminal{{"id1", "/w/a b", "tab\twith tab"}, {"id2", "", ""}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("%q %v", got, err)
	}
	for _, bad := range []string{"id" + fieldSep + "dir" + recordSep, fieldSep + "dir" + fieldSep + "name" + recordSep} {
		osascript = func(context.Context, string, ...string) (string, error) { return bad, nil }
		if _, err := Terminals(context.Background()); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
	osascript = func(context.Context, string, ...string) (string, error) { return "", nil }
	if got, err := Terminals(context.Background()); err != nil || len(got) != 0 {
		t.Fatalf("no terminals: %v %v", got, err)
	}
}

func TestScriptingFailureIsReported(t *testing.T) {
	g := &fakeGhostty{fail: errors.New("focus Ghostty: Ghostty is not running.")}
	setup(t, g, "ttys007")
	if err := FocusProcess(context.Background(), 4242, inBackend); err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatal(err)
	}
}
