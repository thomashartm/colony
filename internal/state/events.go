package state

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"
)

const MaxEventBytes = 4096

type Event struct {
	Schema         int               `json:"schema"`
	TS             time.Time         `json:"ts"`
	Agent          string            `json:"agent"`
	Event          string            `json:"event"`
	Status         string            `json:"status"`
	Summary        string            `json:"summary,omitempty"`
	AgentSessionID string            `json:"agent_session_id,omitempty"`
	Detail         map[string]string `json:"detail,omitempty"`
}

func Clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:n]
	for !utf8.ValidString(s) && len(s) > 0 {
		s = s[:len(s)-1]
	}
	return s
}

// EncodeEvent bounds the entire JSON line, including escaped text, to 4 KiB.
func EncodeEvent(e Event) ([]byte, error) {
	e.Schema = 1
	e.Agent = Clip(e.Agent, 32)
	e.Event = Clip(e.Event, 64)
	e.Status = Clip(e.Status, 32)
	e.AgentSessionID = Clip(e.AgentSessionID, 128)
	e.Summary = Clip(e.Summary, 1536)
	keys := make([]string, 0, len(e.Detail))
	for k := range e.Detail {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	detail := make(map[string]string)
	for _, k := range keys[:min(len(keys), 8)] {
		detail[Clip(k, 32)] = Clip(e.Detail[k], 2048)
	}
	e.Detail = detail
	for {
		data, err := json.Marshal(e)
		if err != nil {
			return nil, err
		}
		if len(data) < MaxEventBytes {
			return append(data, '\n'), nil
		}
		e.Summary = Clip(e.Summary, len(e.Summary)/2)
		for k, v := range e.Detail {
			e.Detail[k] = Clip(v, len(v)/2)
		}
	}
}

// Tail reads regular files only and never scans an entire growing event log or
// transcript. O_NONBLOCK avoids hanging on an accidentally supplied FIFO.
func Tail(path string, limit int64) ([]byte, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	offset := max(int64(0), st.Size()-limit)
	if _, err = f.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(f, limit))
	if err != nil {
		return nil, err
	}
	if offset > 0 {
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			data = data[i+1:]
		} else {
			return nil, nil
		}
	}
	return data, nil
}

func LatestEvent(path string) (Event, error) {
	data, err := Tail(path, 64*1024)
	if os.IsNotExist(err) {
		return Event{}, nil
	}
	if err != nil {
		return Event{}, err
	}
	lines := bytes.Split(data, []byte{'\n'})
	for i := len(lines) - 1; i >= 0; i-- {
		var e Event
		if json.Unmarshal(lines[i], &e) == nil && e.Event != "" {
			return e, nil
		}
	}
	return Event{}, nil
}

func Attention(status string) bool {
	return status == "permission" || status == "question" || status == "ready" || status == "idle"
}
func ValidStatus(status string) bool {
	switch status {
	case "starting", "working", "permission", "question", "ready", "idle", "ended":
		return true
	}
	return false
}

func TranscriptMessage(path string) (string, error) {
	data, err := Tail(path, 64*1024)
	if err != nil {
		return "", err
	}
	lines := bytes.Split(data, []byte{'\n'})
	for i := len(lines) - 1; i >= 0; i-- {
		var row struct {
			Type    string `json:"type"`
			Message struct {
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(lines[i], &row) != nil || row.Type != "assistant" {
			continue
		}
		var text []string
		for _, c := range row.Message.Content {
			if c.Type == "text" {
				text = append(text, c.Text)
			}
		}
		if len(text) > 0 {
			return strings.Join(text, "\n"), nil
		}
	}
	return "", nil
}

// LatestSessionID scans on explicit revive only; hooks and TUI reads remain bounded.
func LatestSessionID(path, agent string) (string, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("event log is not a regular file")
	}
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 4096), 64*1024)
	id := ""
	for scan.Scan() {
		var e Event
		if json.Unmarshal(scan.Bytes(), &e) == nil && e.Agent == agent && e.AgentSessionID != "" {
			id = e.AgentSessionID
		}
	}
	return id, scan.Err()
}
