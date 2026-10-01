package member

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSteerChecksAndLiteralArguments(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_STATE_HOME", root)
	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("MOTLEY_TEST_STATUS", "working")
	t.Setenv("MOTLEY_TEST_RECEIPT", filepath.Join(root, "receipt"))
	dir := filepath.Join(root, "motley/members")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.toml"), []byte("schema=1\nid='a'\nagent='claude'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
case "$*" in
*list-sessions*) printf 'a\ta\t\t%s\t0\t0\n' "$MOTLEY_TEST_STATUS";;
*list-clients*) printf '/dev/work\t/dev/work\tshell\t1\n/dev/monitor\t/dev/monitor\t_motley\t2\n';;
*) for arg do printf '%s\000' "$arg"; done > "$MOTLEY_TEST_RECEIPT";;
esac
`
	if err := os.WriteFile(filepath.Join(root, "tmux"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	receipt := func() []string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(root, "receipt"))
		if err != nil {
			t.Fatal(err)
		}
		return strings.Split(strings.TrimSuffix(string(data), "\x00"), "\x00")
	}
	text := "-literal $(touch nope) ;"
	if err := Reply("a", text); err != nil {
		t.Fatal(err)
	}
	want := []string{"send-keys", "-t", "=a:", "-l", "--", text[:len(text)-1] + `\;`, ";", "send-keys", "-t", "=a:", "Enter"}
	if got := receipt(); !reflect.DeepEqual(got, want) {
		t.Fatalf("reply args: %q", got)
	}
	if err := Send("a", "/dev/work"); err != nil {
		t.Fatal(err)
	}
	if got := receipt(); !reflect.DeepEqual(got, []string{"switch-client", "-c", "/dev/work", "-t", "=a"}) {
		t.Fatal(got)
	}
	if err := os.Remove(filepath.Join(root, "receipt")); err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"permission", "ended"} {
		t.Setenv("MOTLEY_TEST_STATUS", status)
		if err := Reply("a", "hello"); err == nil {
			t.Fatal("reply allowed", status)
		}
	}
	for _, text := range []string{"", "line\nline", "escape\x1b"} {
		if err := Reply("a", text); err == nil {
			t.Fatal("invalid reply accepted")
		}
	}
	for _, client := range []string{"/dev/monitor", "missing", ""} {
		if err := Send("a", client); err == nil {
			t.Fatal("invalid tab accepted", client)
		}
	}
	if err := Reply("missing", "hello"); err == nil {
		t.Fatal("missing member accepted")
	}
	if _, err := os.Stat(filepath.Join(root, "receipt")); !os.IsNotExist(err) {
		t.Fatal("refused action invoked send/switch")
	}
}
