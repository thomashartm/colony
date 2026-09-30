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

// Exec starts an interactive agent without a prompt or permission overrides.
func Exec(name string) error {
	bin, err := Binary(name)
	if err != nil {
		return err
	}
	return syscall.Exec(bin, []string{name}, os.Environ())
}
