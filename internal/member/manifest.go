// Package member manages coding sessions and their manifests.
package member

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
	"github.com/thomashartm/motley/internal/state"
	"github.com/thomashartm/motley/internal/tmux"
)

type Manifest struct {
	// Imported checkouts are borrowed: retirement never removes files or branches.
	ClaudeSession string `toml:"claude_session,omitempty"`

	Prompt    bool       `toml:"prompt,omitempty"`
	Blueprint string     `toml:"blueprint,omitempty"`
	AgentArgs []string   `toml:"agent_args,omitempty"`
	Schema    int        `toml:"schema"`
	ID        string     `toml:"id"`
	Name      string     `toml:"name"`
	Repo      string     `toml:"repo"`
	RepoPath  string     `toml:"repo_path"`
	Worktree  string     `toml:"worktree"`
	Branch    string     `toml:"branch"`
	Base      string     `toml:"base"`
	RemoteURL string     `toml:"remote_url"`
	Ticket    string     `toml:"ticket,omitempty"`
	Crew      string     `toml:"crew,omitempty"`
	Color     string     `toml:"color,omitempty"`
	Agent     string     `toml:"agent"`
	CreatedAt time.Time  `toml:"created_at"`
	RetiredAt *time.Time `toml:"retired_at,omitempty"`
}

var validID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

func CheckID(id string) error {
	if !validID.MatchString(id) {
		return fmt.Errorf("invalid member id %q; use letters, digits, dots, underscores or hyphens, starting with a letter or digit", id)
	}
	return nil
}

func Load(dir, id string) (Manifest, error) {
	if err := CheckID(id); err != nil {
		return Manifest{}, err
	}
	path := filepath.Join(dir, id+".toml")
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read member %s: %w", id, err)
	}
	m := Manifest{Schema: 1}
	if err := toml.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if m.Schema != 1 || m.ID != id {
		return Manifest{}, fmt.Errorf("invalid manifest %s: expected schema 1 and id %q", path, id)
	}
	if m.ClaudeSession != "" && (m.Agent != "claude" || CheckID(m.ClaudeSession) != nil) {
		return Manifest{}, fmt.Errorf("invalid imported Claude session in %s", path)
	}
	return m, nil
}

func loadAll(dir string) ([]Manifest, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var manifests []Manifest
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".toml") {
			continue
		}
		m, err := Load(dir, strings.TrimSuffix(entry.Name(), ".toml"))
		if err != nil {
			return nil, err
		}
		manifests = append(manifests, m)
	}
	return manifests, nil
}

type Row struct {
	External bool
	Manifest
	Alive  bool
	Status string
	Since  int64
	Seen   int64
}

func (r Row) CurrentStatus() string {
	if !r.Alive {
		return "dead"
	}
	if !state.ValidStatus(r.Status) {
		return "alive"
	}
	return r.Status
}

func List() ([]Row, error) {
	dir, err := state.MembersDir()
	if err != nil {
		return nil, err
	}
	manifests, err := loadAll(dir)
	if err != nil {
		return nil, err
	}
	if len(manifests) == 0 {
		return nil, nil
	}
	sessions, err := tmux.Sessions()
	if err != nil {
		return nil, err
	}
	return RefreshExternal(Join(manifests, sessions))
}

func RequireLive(id string) error {
	if err := CheckID(id); err != nil {
		return err
	}
	rows, err := List()
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row.ID == id {
			if !row.Alive {
				return fmt.Errorf("member %s is dead: its tmux session is not running", id)
			}
			return nil
		}
	}
	return fmt.Errorf("member %q not found", id)
}
