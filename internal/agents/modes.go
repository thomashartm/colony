package agents

import (
	"fmt"
	"strings"
)

// Mode is a hard-coded launch preset resolved to native agent arguments.
type Mode struct {
	Name, Summary string
	Args          []string
}

// claudeSandbox enables Claude Code's OS-level Bash sandbox for one session
// through --settings, so nothing is written into the worktree. Sandboxed
// commands may write to the working directory and temp only; in a linked
// worktree Claude also permits the shared .git except its hooks and config.
// failIfUnavailable refuses to start rather than silently run unsandboxed.
const claudeSandbox = `{"sandbox":{"enabled":true,"failIfUnavailable":true,"autoAllowBashIfSandboxed":true}}`

// Verified against Claude Code 2.1.286, whose --permission-mode choices are
// acceptEdits, auto, bypassPermissions, manual, dontAsk and plan.
var claudeModes = []Mode{
	{"manual", "ask before edits and commands", []string{"--permission-mode", "manual"}},
	{"acceptEdits", "accept file edits, ask before commands", []string{"--permission-mode", "acceptEdits"}},
	{"plan", "read-only planning", []string{"--permission-mode", "plan"}},
	{"auto", "approve actions after background safety checks", []string{"--permission-mode", "auto"}},
	{"dontAsk", "deny anything not pre-approved", []string{"--permission-mode", "dontAsk"}},
	{"bypassPermissions", "skip all permission prompts", []string{"--permission-mode", "bypassPermissions"}},
	{"sandbox", "accept edits; Bash confined to the worktree", []string{"--permission-mode", "acceptEdits", "--settings", claudeSandbox}},
}

// Modes lists an agent's presets; agents without presets return nil.
func Modes(agent string) []Mode {
	if agent == "claude" {
		return claudeModes
	}
	return nil
}

func ModeNames(agent string) string {
	var names []string
	for _, m := range Modes(agent) {
		names = append(names, m.Name)
	}
	return strings.Join(names, ", ")
}

// ModeArgs resolves a preset name; unknown names never fall back to another mode.
func ModeArgs(agent, name string) ([]string, error) {
	modes := Modes(agent)
	if len(modes) == 0 {
		return nil, fmt.Errorf("modes are available for claude only, not %s", agent)
	}
	for _, m := range modes {
		if m.Name == name {
			return append([]string(nil), m.Args...), nil
		}
	}
	return nil, fmt.Errorf("unknown %s mode %q; choose %s", agent, name, ModeNames(agent))
}

// ConflictingArg returns the first option in args that mode would also set, so
// a blueprint and a mode never both decide how permissions work.
func ConflictingArg(args, mode []string) string {
	for _, arg := range args {
		option, _, _ := strings.Cut(arg, "=")
		if option == "--dangerously-skip-permissions" {
			return option
		}
		for _, m := range mode {
			if strings.HasPrefix(m, "--") && option == m {
				return option
			}
		}
	}
	return ""
}
