package state

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"unicode/utf8"
)

func TestEventBoundAndTail(t *testing.T) {
	detail := map[string]string{}
	for i := 0; i < 1000; i++ {
		detail[fmt.Sprintf("key-%d", i)] = strings.Repeat("界\x00\"", 2000)
	}
	data, err := EncodeEvent(Event{Agent: "claude", Event: "Stop", Status: "ready", Summary: strings.Repeat("界\x00", 10000), Detail: detail})
	if err != nil || len(data) > MaxEventBytes || !utf8.Valid(data) || data[len(data)-1] != '\n' {
		t.Fatalf("invalid bounded event: %d %v", len(data), err)
	}
	var event Event
	if err := json.Unmarshal(data, &event); err != nil || event.Schema != 1 || event.Status != "ready" {
		t.Fatal("lost event identity", err)
	}
	path := filepath.Join(t.TempDir(), "events.jsonl")
	log := append(bytes.Repeat([]byte("old line\n"), 20000), data...)
	log = append(log, []byte(`{"incomplete":`)...)
	if err := os.WriteFile(path, log, 0600); err != nil {
		t.Fatal(err)
	}
	got, err := LatestEvent(path)
	if err != nil || got.Summary != event.Summary || got.Event != "Stop" {
		t.Fatal("tail did not find last complete event", err)
	}
	empty, err := LatestEvent(path + "missing")
	if err != nil || empty.Event != "" {
		t.Fatal("missing log", err)
	}
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), 70000), 0600); err != nil {
		t.Fatal(err)
	}
	got, err = LatestEvent(path)
	if err != nil || got.Event != "" {
		t.Fatal("partial line treated as event", err)
	}
}
func TestTailRefusesFIFO(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pipe")
	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Tail(path, 4096); err == nil {
		t.Fatal("FIFO accepted")
	}
}
