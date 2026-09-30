// Package crew stores user-managed packages of work without network access.
package crew

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/pelletier/go-toml/v2"
	"github.com/thomashartm/colony/internal/palette"
	"github.com/thomashartm/colony/internal/state"
)

type Crew struct {
	ID    string `toml:"id" json:"id"`
	Title string `toml:"title" json:"title"`
	URL   string `toml:"url,omitempty" json:"url,omitempty"`
	Kind  string `toml:"kind" json:"kind"`
	Color string `toml:"color" json:"color"`
}
type file struct {
	Schema int    `toml:"schema"`
	Crews  []Crew `toml:"crew"`
}

var slug = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func Kind(raw string) string {
	if raw == "" {
		return "text"
	}
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Hostname(), "github.com") {
		return "link"
	}
	p := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(p) == 4 {
		n, _ := strconv.Atoi(p[3])
		if n > 0 {
			if (p[0] == "orgs" || p[0] == "users") && p[2] == "projects" {
				return "project"
			}
			if p[2] == "issues" {
				return "issue"
			}
		}
	}
	return "link"
}
func Validate(c Crew) error {
	if !slug.MatchString(c.ID) || c.ID == "none" {
		return fmt.Errorf("invalid or reserved crew id %q", c.ID)
	}
	if strings.TrimSpace(c.Title) == "" || strings.IndexFunc(c.Title, unicode.IsControl) >= 0 {
		return fmt.Errorf("crew title must be non-empty and contain no control characters")
	}
	if _, ok := palette.Lookup(c.Color); !ok {
		return fmt.Errorf("unknown colour %q", c.Color)
	}
	if c.URL != "" {
		u, err := url.Parse(c.URL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || strings.IndexFunc(c.URL, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
			return fmt.Errorf("crew URL must be an http or https URL without whitespace or control characters")
		}
	}
	return nil
}
func Find(crews []Crew, id string) (Crew, bool) {
	for _, c := range crews {
		if c.ID == id {
			return c, true
		}
	}
	return Crew{}, false
}
func Load() ([]Crew, error) {
	dir, err := state.MinionsDir()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(dir), "crews.toml"))
	if os.IsNotExist(err) {
		return []Crew{}, nil
	}
	if err != nil {
		return nil, err
	}
	f := file{Schema: 1}
	if err := toml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse crews: %w", err)
	}
	if f.Schema != 1 {
		return nil, fmt.Errorf("unsupported crews schema %d", f.Schema)
	}
	seen := map[string]bool{}
	for i := range f.Crews {
		c := &f.Crews[i]
		if c.Color == "" {
			c.Color = palette.Resolve(c.ID, "", "").Name
		}
		c.Kind = Kind(c.URL)
		if err := Validate(*c); err != nil {
			return nil, err
		}
		if seen[c.ID] {
			return nil, fmt.Errorf("duplicate crew %q", c.ID)
		}
		seen[c.ID] = true
	}
	return f.Crews, nil
}

// Save is called by user actions while holding the shared lifecycle lock.
func Save(crews []Crew) error {
	for _, c := range crews {
		if err := Validate(c); err != nil {
			return err
		}
	}
	dir, err := state.MinionsDir()
	if err != nil {
		return err
	}
	root := filepath.Dir(dir)
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	data, err := toml.Marshal(file{Schema: 1, Crews: crews})
	if err != nil {
		return err
	}
	return state.WriteAtomic(filepath.Join(root, "crews.toml"), data)
}
