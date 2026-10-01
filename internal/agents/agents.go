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
	if name == "codex" {
		// Keep hook subprocesses in this member's environment instead of a
		// shared daemon that may have been started by another terminal.
		argv = append(argv, "--no-daemon")
	}
	if prompt != "" {
		if name == "opencode" {
			argv = append(argv, "--prompt="+prompt)
		} else {
			argv = append(argv, "--", prompt)
		}
	}
	return argv
}

// ResumeArgv preserves stored options without replaying the initial prompt.
func ResumeArgv(name string, args []string, sessionID string) []string {
	if sessionID == "" {
		return StartArgv(name, args, "")
	}
	argv := append([]string{name}, args...)
	switch name {
	case "claude":
		return append(argv, "--resume="+sessionID)
	case "codex":
		return append(append([]string{name, "resume"}, args...), "--no-daemon", "--", sessionID)
	case "opencode":
		return append(argv, "--session="+sessionID)
	}
	return argv
}

// Exec replaces the pane process using native agent arguments.
func Exec(name string, args []string, prompt, sessionID string) error {
	bin, err := Binary(name)
	if err != nil {
		return err
	}
	argv := StartArgv(name, args, prompt)
	if sessionID != "" {
		argv = ResumeArgv(name, args, sessionID)
	}
	return syscall.Exec(bin, argv, os.Environ())
}
