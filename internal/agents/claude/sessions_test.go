package claude

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSessionDiscoveryContract(t *testing.T) {
	bin := t.TempDir()
	script := filepath.Join(bin, "claude")
	write := func(body string) {
		t.Helper()
		if err := os.WriteFile(script, []byte("#!/bin/sh\n"+body), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	// The contract, not the bound: a loaded machine can take seconds to start a fresh script.
	old := discoveryTimeout
	discoveryTimeout = time.Minute
	t.Cleanup(func() { discoveryTimeout = old })
	write(`test "$1 $2 $3" = 'agents --json ' || exit 2
printf '%s' '[{"sessionId":"session","cwd":"/repo","kind":"interactive","pid":123,"status":"waiting","waitingFor":"permission prompt","startedAt":1},{"sessionId":"incomplete","cwd":"relative","kind":"interactive"}]'
`)
	sessions, err := Sessions()
	if err != nil || len(sessions) != 1 || sessions[0].MotleyStatus() != "permission" {
		t.Fatal(sessions, err)
	}
	for _, test := range []struct{ status, waiting, want string }{{"busy", "", "working"}, {"idle", "", "idle"}, {"waiting", "input needed", "question"}, {"waiting", "sandbox request", "permission"}, {"unknown", "", "alive"}} {
		if got := (Session{Status: test.status, WaitingFor: test.waiting}).MotleyStatus(); got != test.want {
			t.Fatalf("%+v: %s", test, got)
		}
	}
	write("echo invalid\n")
	if _, err := Sessions(); err == nil {
		t.Fatal("malformed discovery accepted")
	}
	write("exit 2\n")
	if _, err := Sessions(); err == nil {
		t.Fatal("unsupported CLI treated as no sessions")
	}
}
