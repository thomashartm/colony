package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/thomashartm/colony/internal/gitx"
	"github.com/thomashartm/colony/internal/minion"
	"github.com/thomashartm/colony/internal/state"
)

type detailMsg struct {
	id    string
	seq   uint64
	event state.Event
	git   string
	err   error
}
type detailCache struct {
	mu       sync.Mutex
	dir, id  string
	modified time.Time
	size     int64
	event    state.Event
	git      string
	readyKey string
}

func (c *detailCache) command(row minion.Row, seq uint64, force bool) tea.Cmd {
	return func() tea.Msg {
		c.mu.Lock()
		defer c.mu.Unlock()
		result := detailMsg{id: row.ID, seq: seq}
		path := filepath.Join(c.dir, row.ID+".events.jsonl")
		info, err := os.Stat(path)
		if err != nil && !os.IsNotExist(err) {
			result.err = err
			return result
		}
		changed := c.id != row.ID || force || (info == nil && c.size != 0) || (info != nil && (info.ModTime() != c.modified || info.Size() != c.size))
		if changed {
			c.id = row.ID
			c.modified = time.Time{}
			c.size = 0
			c.event = state.Event{}
			if info != nil {
				c.event, err = state.LatestEvent(path)
				if err != nil {
					result.err = err
					return result
				}
				c.modified = info.ModTime()
				c.size = info.Size()
			}
		}
		result.event = c.event
		key := fmt.Sprintf("%s:%d", row.ID, row.Since)
		if row.CurrentStatus() == "ready" {
			if changed || c.readyKey != key {
				c.git = ""
				c.readyKey = key
				head, e := gitx.Output(row.Worktree, "rev-parse", "--short=8", "HEAD")
				if e != nil {
					c.git = e.Error()
				} else {
					diff, e := gitx.Output(row.Worktree, "diff", "--no-ext-diff", "--stat", row.Base+"...HEAD", "--")
					if e != nil {
						diff = e.Error()
					}
					if diff == "" {
						diff = "No committed changes from " + row.Base
					}
					c.git = "HEAD  " + head + "\n" + state.Clip(diff, 4096)
				}
			}
			result.git = c.git
		} else {
			c.readyKey = ""
		}
		return result
	}
}
