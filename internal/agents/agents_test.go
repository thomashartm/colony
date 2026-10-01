package agents

import (
	"reflect"
	"testing"
)

func TestStartArgv(t *testing.T) {
	prompt := "--literal\n$(touch nope) `pwd` \"quotes\""
	for _, name := range []string{"claude", "codex", "opencode"} {
		args := []string{"--model", "example"}
		before := append([]string(nil), args...)
		want := []string{name, "--model", "example", "--", prompt}
		if name == "codex" {
			want = []string{name, "--model", "example", "--no-daemon", "--", prompt}
		}
		if name == "opencode" {
			want = []string{name, "--model", "example", "--prompt=" + prompt}
		}
		if got := StartArgv(name, args, prompt); !reflect.DeepEqual(got, want) {
			t.Fatal(got, want)
		}
		if !reflect.DeepEqual(args, before) {
			t.Fatal("mutated arguments")
		}
		empty := []string{name}
		if name == "codex" {
			empty = append(empty, "--no-daemon")
		}
		if got := StartArgv(name, nil, ""); !reflect.DeepEqual(got, empty) {
			t.Fatal(got)
		}
	}
}

func TestResumeArgv(t *testing.T) {
	id := "--literal-id"
	args := []string{"--model", "fixture"}
	for name, want := range map[string][]string{
		"claude":   {"claude", "--model", "fixture", "--resume=" + id},
		"codex":    {"codex", "resume", "--model", "fixture", "--no-daemon", "--", id},
		"opencode": {"opencode", "--model", "fixture", "--session=" + id},
	} {
		if got := ResumeArgv(name, args, id); !reflect.DeepEqual(got, want) {
			t.Fatal(got, want)
		}
		fresh := []string{name, "--model", "fixture"}
		if name == "codex" {
			fresh = append(fresh, "--no-daemon")
		}
		if got := ResumeArgv(name, args, ""); !reflect.DeepEqual(got, fresh) {
			t.Fatal(got)
		}
	}
}
