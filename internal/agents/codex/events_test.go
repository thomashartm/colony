package codex

import (
	"encoding/json"
	"os"
	"testing"
)

func TestNativeHookFixtures(t *testing.T) {
	// events.json holds synthetic edge cases; live.json a recorded session.
	for _, name := range []string{"testdata/events.json", "testdata/live.json"} {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		var fixtures struct {
			Cases []struct {
				Payload                         json.RawMessage
				Event, Status, Summary, Session string
			}
		}
		if err := json.Unmarshal(data, &fixtures); err != nil {
			t.Fatal(err)
		}
		for _, f := range fixtures.Cases {
			session := f.Session
			if session == "" {
				session = "session-1"
			}
			e, err := Parse(f.Payload, "")
			if err != nil || (f.Event != "" && e.Event != f.Event) || e.Status != f.Status || e.Summary != f.Summary || e.AgentSessionID != session || e.Agent != "codex" {
				t.Fatalf("%s: %+v %v", f.Payload, e, err)
			}
		}
	}
	for _, bad := range []string{"invalid", "null", "{}"} {
		if _, err := Parse([]byte(bad), ""); err == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
}
