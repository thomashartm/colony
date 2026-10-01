package agents

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestClaudeModeArgs(t *testing.T) {
	for name, want := range map[string][]string{
		"manual":            {"--permission-mode", "manual"},
		"acceptEdits":       {"--permission-mode", "acceptEdits"},
		"plan":              {"--permission-mode", "plan"},
		"auto":              {"--permission-mode", "auto"},
		"dontAsk":           {"--permission-mode", "dontAsk"},
		"bypassPermissions": {"--permission-mode", "bypassPermissions"},
		"sandbox":           {"--permission-mode", "acceptEdits", "--settings", claudeSandbox},
	} {
		got, err := ModeArgs("claude", name)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: %q %v", name, got, err)
		}
		got[0] = "mutated"
		if again, _ := ModeArgs("claude", name); again[0] != "--permission-mode" {
			t.Fatal("caller mutated the preset table")
		}
	}
	if len(Modes("claude")) != 7 {
		t.Fatal("untested preset")
	}
}

func TestModeArgsRefusesUnknownSelections(t *testing.T) {
	// "default" is a hidden alias in Claude; Motley names the listed mode only.
	for _, name := range []string{"", "default", "acceptedits", "sandbox "} {
		if _, err := ModeArgs("claude", name); err == nil || !strings.Contains(err.Error(), "manual, acceptEdits") {
			t.Fatalf("%q: %v", name, err)
		}
	}
	for _, agent := range []string{"codex", "opencode"} {
		if _, err := ModeArgs(agent, "plan"); err == nil || !strings.Contains(err.Error(), "claude only") {
			t.Fatalf("%s: %v", agent, err)
		}
	}
}

func TestClaudeSandboxSettings(t *testing.T) {
	var settings struct {
		Sandbox map[string]any `json:"sandbox"`
	}
	if err := json.Unmarshal([]byte(claudeSandbox), &settings); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"enabled": true, "failIfUnavailable": true, "autoAllowBashIfSandboxed": true}
	if !reflect.DeepEqual(settings.Sandbox, want) {
		t.Fatalf("sandbox settings: %v", settings.Sandbox)
	}
}

func TestConflictingArg(t *testing.T) {
	sandbox, _ := ModeArgs("claude", "sandbox")
	plan, _ := ModeArgs("claude", "plan")
	for _, tc := range []struct {
		args, mode []string
		want       string
	}{
		{nil, plan, ""},
		{[]string{"--model", "opus"}, plan, ""},
		{[]string{"--permission-mode", "plan"}, plan, "--permission-mode"},
		{[]string{"--permission-mode=auto"}, plan, "--permission-mode"},
		{[]string{"--settings", "{}"}, plan, ""},
		{[]string{"--settings", "{}"}, sandbox, "--settings"},
		{[]string{"--dangerously-skip-permissions"}, plan, "--dangerously-skip-permissions"},
		// A preset value such as "acceptEdits" is not an option name.
		{[]string{"acceptEdits"}, sandbox, ""},
	} {
		if got := ConflictingArg(tc.args, tc.mode); got != tc.want {
			t.Fatalf("%q with %q: %q want %q", tc.args, tc.mode, got, tc.want)
		}
	}
}
