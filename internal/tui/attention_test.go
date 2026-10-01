package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/thomashartm/motley/internal/member"
	"github.com/thomashartm/motley/internal/state"
)

func statusRow(id, status string, since int64) member.Row {
	r := row(id, status != "dead")
	r.Status = status
	r.Since = since
	return r
}
func TestAttentionOrderAndAlerts(t *testing.T) {
	rows := []member.Row{statusRow("ready", "ready", 30), statusRow("work", "working", 1), statusRow("question", "question", 20), statusRow("permission", "permission", 10), statusRow("idle", "idle", 40), statusRow("dead", "dead", 1), statusRow("ended", "ended", 1)}
	m := update(newModel(true, true, "monitor", nil), tea.WindowSizeMsg{Width: 120, Height: 30})
	m = update(m, snapshot{rows: rows})
	var ids []string
	for _, r := range m.rows {
		ids = append(ids, r.ID)
	}
	if strings.Join(ids, ",") != "permission,question,ready,idle,work,dead,ended" {
		t.Fatalf("order: %v", ids)
	}
	if m.alert {
		t.Fatal("initial snapshot should not alert for an existing backlog")
	}
	for _, s := range []string{"NEEDS YOU", "WORKING", "ENDED / DEAD"} {
		if !strings.Contains(m.View(), s) {
			t.Fatal("missing section", s)
		}
	}
	next := append([]member.Row(nil), rows...)
	for i := range next {
		if next[i].ID == "work" {
			next[i].Status = "ready"
			next[i].Since = 50
		}
	}
	m = update(m, snapshot{rows: next})
	if !m.alert || !strings.Contains(m.View(), "NEW ATTENTION") {
		t.Fatal("new attention not marked")
	}
	m = update(m, key("j"))
	if m.alert {
		t.Fatal("selection should acknowledge alert")
	}
	m = update(m, snapshot{rows: next})
	if m.alert {
		t.Fatal("unchanged snapshot alerted again")
	}
	ordinary := update(newModel(false, true, "client", nil), snapshot{rows: rows})
	ordinary = update(ordinary, snapshot{rows: next})
	if ordinary.alert {
		t.Fatal("ordinary view raised monitor alert")
	}
}
func TestStatusDetailAndLateSelectionResponse(t *testing.T) {
	m := update(newModel(false, true, "client", nil), tea.WindowSizeMsg{Width: 120, Height: 50})
	m = update(m, snapshot{rows: []member.Row{statusRow("a", "question", 1), statusRow("b", "permission", 2)}})
	m.detailSeq = 1
	m = update(m, detailMsg{id: "a", seq: 1, event: state.Event{Status: "question", Summary: "generic", Detail: map[string]string{"question": "Which option?\n1) Alpha\n2) Beta"}}})
	if !strings.Contains(m.detail.View(), "Beta") {
		t.Fatal("question/options missing")
	}
	m = update(m, key("j"))
	m.detailSeq = 2
	m = update(m, detailMsg{id: "a", seq: 1, event: state.Event{Status: "permission", Summary: "wrong response"}})
	if strings.Contains(m.detail.View(), "wrong response") {
		t.Fatal("late result replaced selected detail")
	}
	m = update(m, detailMsg{id: "b", seq: 2, event: state.Event{Status: "permission", Summary: "Permission needed", Detail: map[string]string{"tool": "Bash", "input": "pnpm test"}}})
	if !strings.Contains(m.detail.View(), "Bash: pnpm test") {
		t.Fatal("permission command missing")
	}
	m.rows[m.selected].Status = "ready"
	m = update(m, detailMsg{id: "b", seq: 2, event: state.Event{Status: "ready", Summary: "Finished"}, git: "HEAD  abcdef12\nchange.txt | 1 +"})
	if !strings.Contains(m.detail.View(), "Finished") || !strings.Contains(m.detail.View(), "change.txt") {
		t.Fatal("ready response/diff missing")
	}
}
func TestSelectedEventRefresh(t *testing.T) {
	dir := t.TempDir()
	cache := detailCache{dir: dir}
	r := statusRow("a", "question", 1)
	path := filepath.Join(dir, "a.events.jsonl")
	write := func(summary string) {
		t.Helper()
		data, err := state.EncodeEvent(state.Event{Event: "Notification", Status: "question", Summary: summary})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("first question")
	first := cache.command(r, 1, true)().(detailMsg)
	if first.err != nil || first.event.Summary != "first question" {
		t.Fatal("first load", first)
	}
	write("replacement question while still selected")
	second := cache.command(r, 2, false)().(detailMsg)
	if second.err != nil || second.event.Summary == first.event.Summary {
		t.Fatal("selection did not refresh", second)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if result := cache.command(r, 3, false)().(detailMsg); result.event.Event != "" {
		t.Fatal("removed log remained cached")
	}
}
