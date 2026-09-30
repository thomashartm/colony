package claude

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestRecordedPayloads(t *testing.T) {
	data, err := os.ReadFile("testdata/recorded.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Version string `json:"claude_version"`
		Cases   []struct {
			Name            string
			Payload         json.RawMessage
			Status, Summary string
			Detail          map[string]string
		}
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Version != "2.1.285" || len(fixture.Cases) != 12 {
		t.Fatal("missing recorded payloads")
	}
	for _, c := range fixture.Cases {
		t.Run(c.Name, func(t *testing.T) {
			got, err := Parse(c.Payload, "")
			if err != nil {
				t.Fatal(err)
			}
			var common struct {
				ID    string `json:"session_id"`
				Event string `json:"hook_event_name"`
			}
			if err := json.Unmarshal(c.Payload, &common); err != nil {
				t.Fatal(err)
			}
			if got.Status != c.Status || got.Summary != c.Summary || !reflect.DeepEqual(got.Detail, c.Detail) || got.AgentSessionID != common.ID || got.Event != common.Event || got.Agent != "claude" {
				t.Fatalf("mapping: %+v\nwant status=%s summary=%q detail=%v", got, c.Status, c.Summary, c.Detail)
			}
		})
	}
}

func TestNotificationFallbackAndStop(t *testing.T) {
	for _, tc := range []struct{ input, event, status, summary string }{
		{`{"hook_event_name":"Notification","message":"Approval needed"}`, "", "permission", "Approval needed"},
		{`{"hook_event_name":"Notification","message":"Waiting for input"}`, "", "idle", "Waiting for input"},
		{`{"hook_event_name":"Notification","notification_type":"auth_success","message":"permission granted"}`, "", "", "permission granted"},
		{`{"hook_event_name":"FutureHook"}`, "", "", ""},
		{`{"prompt":"first line\nsecond line"}`, "UserPromptSubmit", "working", "first line"},
		{`{"hook_event_name":"Stop","transcript_path":"testdata/transcript.jsonl"}`, "", "ready", "colony fixture complete"},
		{`{"hook_event_name":"Stop","transcript_path":"/missing","last_assistant_message":"Direct response"}`, "", "ready", "Direct response"},
	} {
		got, err := Parse([]byte(tc.input), tc.event)
		if err != nil || got.Status != tc.status || got.Summary != tc.summary {
			t.Fatalf("%s: %+v %v", tc.input, got, err)
		}
	}
	for _, input := range []string{`no json`, `{}`, `{"hook_event_name":"PreToolUse","tool_name":"AskUserQuestion","tool_input":42}`} {
		if _, err := Parse([]byte(input), ""); err == nil {
			t.Fatalf("accepted %s", input)
		}
	}
	got, err := Parse([]byte(`{"hook_event_name":"Stop","transcript_path":"/missing"}`), "")
	if err == nil || got.Status != "ready" {
		t.Fatal("a transcript failure must retain ready status")
	}
}
