// Package report implements the bounded, fail-open hook entry point.
package report

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/thomashartm/motley/internal/agents/claude"
	"github.com/thomashartm/motley/internal/agents/codex"
	"github.com/thomashartm/motley/internal/agents/opencode"
	"github.com/thomashartm/motley/internal/member"
	"github.com/thomashartm/motley/internal/state"
	"github.com/thomashartm/motley/internal/tmux"
)

const timeout = 250 * time.Millisecond

// Run never prints or returns an error to an agent. It also bounds blocked stdin
// and tmux calls, so a broken hook cannot hold up the agent indefinitely.
func Run(args []string, stdin io.Reader) {
	id := os.Getenv("MOTLEY_MEMBER")
	if id == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		defer func() {
			if p := recover(); p != nil {
				done <- fmt.Errorf("report panic: %v", p)
			}
		}()
		done <- handle(ctx, id, args, stdin)
	}()
	select {
	case err := <-done:
		if err != nil {
			logError(err)
		}
	case <-ctx.Done():
		logError(ctx.Err())
	}
}

func handle(ctx context.Context, id string, args []string, stdin io.Reader) error {
	if err := member.CheckID(id); err != nil {
		return err
	}
	flags := flag.NewFlagSet("report", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	agent := flags.String("agent", "", "agent")
	eventName := flags.String("event", "", "event")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if (*agent != "claude" && *agent != "codex" && *agent != "opencode") || flags.NArg() != 0 {
		return fmt.Errorf("report requires --agent claude|codex|opencode and JSON stdin")
	}
	data, readErr := io.ReadAll(io.LimitReader(stdin, 1024*1024+1))
	if err := ctx.Err(); err != nil {
		return err
	}
	var event state.Event
	var parseErr error
	switch *agent {
	case "claude":
		event, parseErr = claude.Parse(data, *eventName)
	case "codex":
		event, parseErr = codex.Parse(data, *eventName)
	case "opencode":
		event, parseErr = opencode.Parse(data)
	}
	if len(data) > 1024*1024 {
		event = state.Event{}
		parseErr = fmt.Errorf("hook payload exceeds 1 MiB")
	}
	if readErr != nil {
		parseErr = readErr
	}
	dir, err := state.MembersDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, id+".events.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY|syscall.O_NONBLOCK, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("event log is not a regular file")
	}
	// A bounded lock serializes status reads and append decisions for concurrent
	// tool hooks. It is released automatically even when the process times out.
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Millisecond):
		}
	}
	defer func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }()
	previous, err := tmux.ReportStatus(ctx, id)
	if err != nil {
		return err
	}
	event.TS = time.Now().UTC()
	var prior state.Event
	_ = json.Unmarshal([]byte(previous.Context), &prior)
	if *agent == "claude" && event.Event == "Notification" && event.Status == "permission" && prior.AgentSessionID == event.AgentSessionID {
		if previous.Status == "question" {
			event.Status = "question"
		}
		for k, v := range prior.Detail {
			if _, exists := event.Detail[k]; !exists {
				event.Detail[k] = v
			}
		}
	}
	changed := event.Status != "" && event.Status != previous.Status
	contextText := ""
	if event.Status != "" {
		data, err := state.EncodeEvent(event)
		if err != nil {
			return err
		}
		contextText = strings.TrimSuffix(string(data), "\n")
	}
	if err := tmux.ReportUpdate(ctx, id, event.Status, contextText, changed, event.TS.Unix()); err != nil {
		return err
	}
	if event.Status == "" {
		event.Status = previous.Status
	}
	retained := event.Event == "SessionStart" || event.Event == "Stop" || event.Event == "Notification" || event.Event == "SessionEnd"
	if changed || retained {
		line, err := state.EncodeEvent(event)
		if err != nil {
			return err
		}
		if _, err := f.Write(line); err != nil {
			return err
		}
	}
	return parseErr
}

func logError(err error) {
	// An unavailable state filesystem must not make diagnostic logging block
	// the agent after the report itself has already timed out.
	done := make(chan struct{})
	go func() { writeError(err); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Millisecond):
	}
}

func writeError(err error) {
	dir, e := state.MembersDir()
	if e != nil {
		return
	}
	root := filepath.Dir(dir)
	if os.MkdirAll(root, 0o700) != nil {
		return
	}
	// JSONL gives the diagnostic log a schema without changing Claude settings.
	line, e := state.EncodeEvent(state.Event{Event: "ReportError", TS: time.Now().UTC(), Summary: err.Error()})
	if e != nil {
		return
	}
	f, e := os.OpenFile(filepath.Join(root, "report.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND|syscall.O_NONBLOCK, 0o600)
	if e != nil {
		return
	}
	defer func() { _ = f.Close() }()
	if st, e := f.Stat(); e == nil && st.Mode().IsRegular() {
		_, _ = f.Write(line)
	}
}
