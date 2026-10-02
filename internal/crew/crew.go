// Package crew stores groups of members and their gigs without network access.
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
	"github.com/thomashartm/motley/internal/palette"
	"github.com/thomashartm/motley/internal/state"
)

type Crew struct {
	ID    string `toml:"id" json:"id"`
	Title string `toml:"title" json:"title"`
	Gig   string `toml:"gig,omitempty" json:"gig,omitempty"`
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
	kind, _, _, _ := GitHubRef(raw)
	return kind
}

// GitHubRef classifies a crew URL and returns the GitHub coordinates it names:
// owner and number for a project, owner, repo and number for an issue.
func GitHubRef(raw string) (kind, owner, repo string, number int) {
	if raw == "" {
		return "text", "", "", 0
	}
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Hostname(), "github.com") {
		return "link", "", "", 0
	}
	p := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(p) == 4 {
		if n, _ := strconv.Atoi(p[3]); n > 0 {
			if (p[0] == "orgs" || p[0] == "users") && p[2] == "projects" {
				return "project", p[1], "", n
			}
			if p[2] == "issues" {
				return "issue", p[0], p[1], n
			}
		}
	}
	return "link", "", "", 0
}

// FindURL matches crews by URL, ignoring scheme and host case, query, fragment
// and a trailing slash.
func FindURL(crews []Crew, raw string) (Crew, bool) {
	want := normalizeURL(raw)
	if want == "" {
		return Crew{}, false
	}
	for _, c := range crews {
		if c.URL != "" && normalizeURL(c.URL) == want {
			return c, true
		}
	}
	return Crew{}, false
}

func normalizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	u.Scheme, u.Host = strings.ToLower(u.Scheme), strings.ToLower(u.Host)
	u.RawQuery, u.Fragment, u.RawFragment, u.RawPath = "", "", "", ""
	u.Path = strings.TrimSuffix(u.Path, "/")
	return u.String()
}
func Validate(c Crew) error {
	if !slug.MatchString(c.ID) || c.ID == "none" {
		return fmt.Errorf("invalid or reserved crew id %q", c.ID)
	}
	if strings.TrimSpace(c.Title) == "" || strings.IndexFunc(c.Title, unicode.IsControl) >= 0 {
		return fmt.Errorf("crew title must be non-empty and contain no control characters")
	}
	if strings.IndexFunc(c.Gig, unicode.IsControl) >= 0 {
		return fmt.Errorf("gig must contain no control characters")
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
	dir, err := state.MembersDir()
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
	dir, err := state.MembersDir()
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
