package opencode

import (
	"encoding/json"
	"os"
	"testing"
)

func TestPluginFixtures(t *testing.T) {
	data, err := os.ReadFile("testdata/events.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures struct {
		Cases []struct {
			Payload                json.RawMessage
			Event, Status, Summary string
		}
	}
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, f := range fixtures.Cases {
		e, err := Parse(f.Payload)
		if err != nil || e.Event != f.Event || e.Status != f.Status || e.Summary != f.Summary || e.AgentSessionID != "session-1" || e.Agent != "opencode" {
			t.Fatalf("%s: %+v %v", f.Payload, e, err)
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
