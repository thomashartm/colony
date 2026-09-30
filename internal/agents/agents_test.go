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
		if name == "opencode" {
			want = []string{name, "--model", "example", "--prompt=" + prompt}
		}
		if got := StartArgv(name, args, prompt); !reflect.DeepEqual(got, want) {
			t.Fatal(got, want)
		}
		if !reflect.DeepEqual(args, before) {
			t.Fatal("mutated arguments")
		}
		if got := StartArgv(name, nil, ""); !reflect.DeepEqual(got, []string{name}) {
			t.Fatal(got)
		}
	}
}
