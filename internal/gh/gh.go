// Package gh runs the GitHub CLI on explicit user action only (REQUIREMENTS §0).
package gh

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Runner executes gh. dir is the working directory; empty means the current one.
type Runner interface {
	Output(ctx context.Context, dir string, args ...string) ([]byte, error)
}

// RunnerFunc adapts a function to Runner.
type RunnerFunc func(ctx context.Context, dir string, args ...string) ([]byte, error)

// Output calls f.
func (f RunnerFunc) Output(ctx context.Context, dir string, args ...string) ([]byte, error) {
	return f(ctx, dir, args...)
}

var (
	// ErrMissing reports that gh is not installed or not on PATH.
	ErrMissing = errors.New("GitHub CLI (gh) not found; install gh for issue and PR data")
	// ErrAuth reports that gh needs gh auth login.
	ErrAuth  = errors.New("GitHub CLI is not authenticated; run gh auth login")
	errScope = errors.New("gh needs the read:project scope; run gh auth refresh -s read:project")
)

// Exec runs the gh executable found on PATH, never prompting.
type Exec struct{}

// Output runs gh with args in dir and returns its stdout.
func (Exec) Output(ctx context.Context, dir string, args ...string) ([]byte, error) {
	bin, err := exec.LookPath("gh")
	if err != nil {
		return nil, ErrMissing
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GH_PROMPT_DISABLED=1", "GH_NO_UPDATE_NOTIFIER=1", "NO_COLOR=1")
	// Do not wait on pipes held open by children after a timeout kills gh.
	cmd.WaitDelay = time.Second
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, classify(ctx, err, stderr.String())
	}
	return out, nil
}

func classify(ctx context.Context, err error, stderr string) error {
	if ctx.Err() != nil {
		return fmt.Errorf("gh timed out: %w", ctx.Err())
	}
	lower := strings.ToLower(stderr)
	if strings.Contains(lower, "read:project") {
		return errScope
	}
	var exit *exec.ExitError
	if (errors.As(err, &exit) && exit.ExitCode() == 4) || strings.Contains(lower, "gh auth login") || strings.Contains(lower, "not logged in") {
		return ErrAuth
	}
	if line := firstLine(stderr); line != "" {
		return fmt.Errorf("gh: %s", line)
	}
	return fmt.Errorf("gh: %w", err)
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(line)
}

// Hint is the one-line message shown to the user for a gh failure.
func Hint(err error) string {
	if err == nil {
		return ""
	}
	return firstLine(err.Error())
}

// Client issues the gh calls motley needs.
type Client struct{ run Runner }

// New returns a client that runs gh through r.
func New(r Runner) *Client { return &Client{run: r} }

// Default returns a client that runs the gh executable.
func Default() *Client { return New(Exec{}) }
