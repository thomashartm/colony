package opencode

import (
	"encoding/json"
	"os"
	"testing"
)

func TestPluginFixtures(t *testing.T) {
	// events.json holds synthetic edge cases; live.json recorded sessions.
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
			e, err := Parse(f.Payload)
			if err != nil || e.Event != f.Event || e.Status != f.Status || e.Summary != f.Summary || e.AgentSessionID != session || e.Agent != "opencode" {
				t.Fatalf("%s: %+v %v", f.Payload, e, err)
			}
		}
	}
	for _, bad := range []string{"invalid", "null", "{}", `{"type":"session.idle","properties":{}}`} {
		if _, err := Parse([]byte(bad)); err == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
	e, err := Parse([]byte(`{"type":"session.created","properties":{"info":{"id":"child","parentID":"parent"}}}`))
	if err != nil || e.Status != "" {
		t.Fatal(e, err)
	}
}
