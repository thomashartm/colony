// Package agents starts the supported interactive coding agents.
package agents

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func Binary(name string) (string, error) {
	switch name {
	case "claude", "codex", "opencode":
	default:
		return "", fmt.Errorf("unknown agent %q; choose claude, codex or opencode", name)
	}
	bin, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("agent %s is not on PATH: %w", name, err)
	}
	return bin, nil
}

// StartArgv uses the interactive CLIs' native prompt arguments. The separator
// keeps a prompt beginning with a hyphen from being interpreted as an option.
func StartArgv(name string, args []string, prompt string) []string {
	argv := append([]string{name}, args...)
	if prompt != "" {
		if name == "opencode" {
			argv = append(argv, "--prompt="+prompt)
		} else {
			argv = append(argv, "--", prompt)
		}
	}
	return argv
}

// Exec preserves stored agent arguments when resuming, without replaying the
// original prompt. Codex/OpenCode resume support is a later slice.
func Exec(name string, args []string, prompt, sessionID string) error {
	bin, err := Binary(name)
	if err != nil {
		return err
	}
	argv := StartArgv(name, args, prompt)
	if name == "claude" && sessionID != "" {
		argv = append(append([]string{name}, args...), "--resume="+sessionID)
	}
	return syscall.Exec(bin, argv, os.Environ())
}
