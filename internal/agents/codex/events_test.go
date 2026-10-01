package codex

import (
	"encoding/json"
	"os"
	"testing"
)

func TestNativeHookFixtures(t *testing.T) {
	data, err := os.ReadFile("testdata/events.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures struct {
		Cases []struct {
			Payload         json.RawMessage
			Status, Summary string
		}
	}
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, f := range fixtures.Cases {
		e, err := Parse(f.Payload, "")
		if err != nil || e.Status != f.Status || e.Summary != f.Summary || e.AgentSessionID != "session-1" || e.Agent != "codex" {
			t.Fatalf("%s: %+v %v", f.Payload, e, err)
		}
	}
	for _, bad := range []string{"invalid", "null", "{}"} {
		if _, err := Parse([]byte(bad), ""); err == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
}
