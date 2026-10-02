// Package ghostty focuses Ghostty terminal surfaces through the AppleScript
// dictionary of Ghostty 1.3, which exposes each terminal's id, title and
// working directory but neither its tty nor its process.
package ghostty

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"
)

type Terminal struct {
	ID, Dir, Name string
}

// Unit and record separators cannot occur in titles or paths Ghostty reports.
const (
	fieldSep  = "\x1f"
	recordSep = "\x1e"
)

// Ghostty rejects `whose working directory is` filters, so scripts iterate.
// A tell block would launch Ghostty, so check that it runs first.
const listScript = `on run argv
 if application "Ghostty" is not running then error "Ghostty is not running."
 tell application "Ghostty"
  set out to ""
  repeat with term in terminals
   set d to working directory of term
   if d is missing value then set d to ""
   set n to name of term
   if n is missing value then set n to ""
   set out to out & (id of term) & (character id 31) & d & (character id 31) & n & (character id 30)
  end repeat
  return out
 end tell
end run`

const focusScript = `on run argv
 tell application "Ghostty"
  repeat with term in terminals
   if id of term is item 1 of argv then
    focus term
    return
   end if
  end repeat
  error "The Ghostty terminal closed."
 end tell
end run`

// Test seams.
var (
	devDir       = "/dev"
	locateWithin = 1500 * time.Millisecond
	pollEvery    = 50 * time.Millisecond
)

var osascript = func(ctx context.Context, script string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, "osascript", append([]string{"-e", script}, args...)...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("focus Ghostty: %s", strings.TrimSpace(string(out)))
	}
	return strings.TrimSuffix(string(out), "\n"), nil
}

func Terminals(ctx context.Context) ([]Terminal, error) {
	out, err := osascript(ctx, listScript)
	if err != nil {
		return nil, err
	}
	var terminals []Terminal
	for _, record := range strings.Split(out, recordSep) {
		if record == "" {
			continue
		}
		fields := strings.Split(record, fieldSep)
		if len(fields) != 3 || fields[0] == "" {
			return nil, fmt.Errorf("unexpected Ghostty terminal record %q", record)
		}
		terminals = append(terminals, Terminal{ID: fields[0], Dir: fields[1], Name: fields[2]})
	}
	return terminals, nil
}

func Focus(ctx context.Context, id string) error {
	_, err := osascript(ctx, focusScript, id)
	return err
}

// FocusProcess focuses the Ghostty terminal running pid. A single terminal in
// the process's directory is focused directly. Otherwise the process's own tty
// briefly carries a unique title, which identifies its terminal exactly; the
// previous title is restored before focusing.
func FocusProcess(ctx context.Context, pid int, inDir func(string) bool) error {
	terminals, err := Terminals(ctx)
	if err != nil {
		return err
	}
	var matches []Terminal
	for _, t := range terminals {
		if t.Dir != "" && inDir(t.Dir) {
			matches = append(matches, t)
		}
	}
	if len(matches) == 1 {
		return Focus(ctx, matches[0].ID)
	}
	tty, err := processTTY(ctx, pid)
	if err != nil {
		return err
	}
	t, err := locate(ctx, tty, terminals)
	if err != nil {
		return err
	}
	return Focus(ctx, t.ID)
}

var ttyName = regexp.MustCompile(`^ttys[0-9]+$`)

func processTTY(ctx context.Context, pid int) (string, error) {
	if pid <= 1 {
		return "", fmt.Errorf("invalid process id %d", pid)
	}
	out, err := exec.CommandContext(ctx, "ps", "-o", "tty=", "-p", strconv.Itoa(pid)).Output()
	tty := strings.TrimSpace(string(out))
	if err != nil || !ttyName.MatchString(tty) {
		return "", fmt.Errorf("cannot find the terminal of process %d; switch to its tab manually", pid)
	}
	return tty, nil
}

// locate marks tty with a unique title and finds the terminal showing it. The
// mark is rewritten on every poll because a working agent retitles its
// terminal continuously.
func locate(ctx context.Context, tty string, before []Terminal) (Terminal, error) {
	f, err := os.OpenFile(filepath.Join(devDir, tty), os.O_WRONLY|syscall.O_NOCTTY, 0)
	if err != nil {
		return Terminal{}, fmt.Errorf("open %s: %w", tty, err)
	}
	defer func() { _ = f.Close() }()
	nonce := make([]byte, 8)
	if _, err := rand.Read(nonce); err != nil {
		return Terminal{}, err
	}
	marker := "motley-" + hex.EncodeToString(nonce)
	deadline := time.Now().Add(locateWithin)
	for {
		if err := setTitle(f, marker); err != nil {
			return Terminal{}, err
		}
		terminals, err := Terminals(ctx)
		if err != nil {
			_ = setTitle(f, "")
			return Terminal{}, err
		}
		for _, t := range terminals {
			if t.Name == marker {
				previous := ""
				for _, b := range before {
					if b.ID == t.ID {
						previous = b.Name
					}
				}
				return t, setTitle(f, previous)
			}
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			// The mark reached a terminal Motley cannot see; an empty title
			// restores that terminal's default.
			_ = setTitle(f, "")
			return Terminal{}, fmt.Errorf("this Claude session (%s) is not in a Ghostty tab Motley can see, e.g. it runs in tmux, over ssh or in another terminal app; switch to it manually", tty)
		}
		time.Sleep(pollEvery)
	}
}

// setTitle writes OSC 2 with a single write call, so Motley never splits the
// sequence itself; control characters would end it early.
func setTitle(f *os.File, title string) error {
	title = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, title)
	_, err := f.WriteString("\x1b]2;" + title + "\a")
	return err
}
