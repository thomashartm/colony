package member

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/thomashartm/motley/internal/tmux"
)

// Catalog caches parsed manifests between overview refreshes. Only the polling
// command accesses it; Bubble Tea receives immutable snapshots.
type Catalog struct{ files map[string]cachedManifest }
type cachedManifest struct {
	manifest Manifest
	modified time.Time
	size     int64
}

func (c *Catalog) Load(dir string) ([]Manifest, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		c.files = nil
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	next := make(map[string]cachedManifest)
	var manifests []Manifest
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".toml") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		cached, found := c.files[entry.Name()]
		if !found || cached.modified != info.ModTime() || cached.size != info.Size() {
			m, err := Load(dir, strings.TrimSuffix(filepath.Base(entry.Name()), ".toml"))
			if err != nil {
				return nil, err
			}
			cached = cachedManifest{m, info.ModTime(), info.Size()}
		}
		next[entry.Name()] = cached
		manifests = append(manifests, cached.manifest)
	}
	c.files = next
	return manifests, nil
}

func Join(manifests []Manifest, sessions []tmux.Session) []Row {
	live := make(map[string]tmux.Session, len(sessions))
	for _, s := range sessions {
		if s.Name != tmux.MonitorSession {
			live[s.Name] = s
		}
	}
	rows := make([]Row, 0, len(manifests))
	for _, m := range manifests {
		s := live[tmux.SessionName(m.ID)]
		rows = append(rows, Row{Manifest: m, Alive: s.MemberID == m.ID, Status: s.Status, Since: s.Since, Seen: s.Seen})
	}
	return rows
}
