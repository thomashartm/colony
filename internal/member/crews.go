package member

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/pelletier/go-toml/v2"
	"github.com/thomashartm/motley/internal/crew"
	"github.com/thomashartm/motley/internal/palette"
	"github.com/thomashartm/motley/internal/state"
	"github.com/thomashartm/motley/internal/tmux"
)

func Color(m Manifest, crews []crew.Crew) palette.Color {
	c, _ := crew.Find(crews, m.Crew)
	return palette.Resolve(m.ID, m.Color, c.Color)
}
func validateIdentity(crewID, color string, crews []crew.Crew) error {
	if crewID != "" {
		if _, ok := crew.Find(crews, crewID); !ok {
			return fmt.Errorf("crew %q not found", crewID)
		}
	}
	if color != "" {
		if _, ok := palette.Lookup(color); !ok {
			return fmt.Errorf("unknown colour %q", color)
		}
	}
	return nil
}
func applyAppearance(m Manifest, crews []crew.Crew) error {
	c, _ := crew.Find(crews, m.Crew)
	return tmux.Appearance(m.ID, m.Name, m.Ticket, m.Agent, c.Title, Color(m, crews))
}
func saveManifest(dir string, m Manifest) error {
	data, err := toml.Marshal(m)
	if err != nil {
		return err
	}
	return state.WriteAtomic(filepath.Join(dir, m.ID+".toml"), data)
}
func syncAppearance(ms []Manifest, crews []crew.Crew) error {
	if len(ms) == 0 {
		return nil
	}
	sessions, err := tmux.Sessions()
	if err != nil {
		return fmt.Errorf("saved changes; cannot refresh live sessions: %w", err)
	}
	var failures []error
	for _, m := range ms {
		for _, s := range sessions {
			if s.Name == tmux.SessionName(m.ID) && s.MemberID == m.ID {
				if err := applyAppearance(m, crews); err != nil {
					failures = append(failures, fmt.Errorf("%s: %w", m.ID, err))
				}
			}
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("saved changes; live refresh incomplete: %w", errors.Join(failures...))
	}
	return nil
}

func AddCrew(title, url, color, gig string) (crew.Crew, error) {
	dir, err := state.MembersDir()
	if err != nil {
		return crew.Crew{}, err
	}
	lock, err := state.LockSpawn(dir)
	if err != nil {
		return crew.Crew{}, err
	}
	defer func() { _ = lock.Close() }()
	crews, err := crew.Load()
	if err != nil {
		return crew.Crew{}, err
	}
	c, err := newCrew(crews, title, url, color, gig)
	if err != nil {
		return c, err
	}
	return c, crew.Save(append(crews, c))
}

// NewCrewID is the id AddCrew would assign to title, given the existing crews.
func NewCrewID(crews []crew.Crew, title string) string {
	base := slug(strings.TrimSpace(title))
	if base == "" || base == "none" {
		base = "crew"
	}
	id := base
	for n := 2; ; n++ {
		if _, found := crew.Find(crews, id); !found {
			return id
		}
		id = fmt.Sprintf("%s-%d", base, n)
	}
}

// newCrew builds a validated crew without saving or locking; callers hold the
// lifecycle lock.
func newCrew(crews []crew.Crew, title, url, color, gig string) (crew.Crew, error) {
	title = strings.TrimSpace(title)
	if color == "" {
		used := map[string]bool{}
		for _, c := range crews {
			used[c.Color] = true
		}
		color = palette.Colors[len(crews)%len(palette.Colors)].Name
		for _, c := range palette.Colors {
			if !used[c.Name] {
				color = c.Name
				break
			}
		}
	}
	c := crew.Crew{ID: NewCrewID(crews, title), Title: title, Gig: strings.TrimSpace(gig), URL: url, Kind: crew.Kind(url), Color: color}
	return c, crew.Validate(c)
}

type CrewEdit struct{ Title, URL, Color, Gig *string }

func EditCrew(id string, edit CrewEdit) error {
	dir, err := state.MembersDir()
	if err != nil {
		return err
	}
	lock, err := state.LockSpawn(dir)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Close() }()
	crews, err := crew.Load()
	if err != nil {
		return err
	}
	found := false
	for i := range crews {
		c := &crews[i]
		if c.ID != id {
			continue
		}
		found = true
		if edit.Title != nil {
			c.Title = strings.TrimSpace(*edit.Title)
		}
		if edit.Gig != nil {
			c.Gig = strings.TrimSpace(*edit.Gig)
		}
		if edit.URL != nil {
			c.URL = *edit.URL
		}
		if edit.Color != nil {
			c.Color = *edit.Color
		}
		c.Kind = crew.Kind(c.URL)
		if err := crew.Validate(*c); err != nil {
			return err
		}
	}
	if !found {
		return fmt.Errorf("crew %q not found", id)
	}
	ms, err := loadAll(dir)
	if err != nil {
		return err
	}
	var affected []Manifest
	for _, m := range ms {
		if m.Crew == id {
			affected = append(affected, m)
		}
	}
	if err := crew.Save(crews); err != nil {
		return err
	}
	return syncAppearance(affected, crews)
}
func RemoveCrew(id string, force bool) error {
	dir, err := state.MembersDir()
	if err != nil {
		return err
	}
	lock, err := state.LockSpawn(dir)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Close() }()
	crews, err := crew.Load()
	if err != nil {
		return err
	}
	if _, found := crew.Find(crews, id); !found {
		return fmt.Errorf("crew %q not found", id)
	}
	ms, err := loadAll(dir)
	if err != nil {
		return err
	}
	var affected []Manifest
	for _, m := range ms {
		if m.Crew == id {
			affected = append(affected, m)
		}
	}
	if len(affected) > 0 && !force {
		return fmt.Errorf("crew %s is referenced by %d members; --force unassigns them", id, len(affected))
	}
	for i := range affected {
		affected[i].Crew = ""
		if err := saveManifest(dir, affected[i]); err != nil {
			return err
		}
	}
	remaining := []crew.Crew{}
	for _, c := range crews {
		if c.ID != id {
			remaining = append(remaining, c)
		}
	}
	if err := crew.Save(remaining); err != nil {
		return err
	}
	return syncAppearance(affected, remaining)
}

type IdentityEdit struct{ Name, Ticket, Crew, Color *string }

func EditIdentity(id string, edit IdentityEdit) error {
	dir, err := state.MembersDir()
	if err != nil {
		return err
	}
	lock, err := state.LockSpawn(dir)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Close() }()
	m, err := Load(dir, id)
	if err != nil {
		return err
	}
	crews, err := crew.Load()
	if err != nil {
		return err
	}
	if edit.Name != nil {
		m.Name = strings.TrimSpace(*edit.Name)
		if m.Name == "" {
			return fmt.Errorf("name must not be empty")
		}
	}
	if edit.Ticket != nil && *edit.Ticket != m.Ticket {
		// The recorded issue belongs to the old ticket; u fetches the new one.
		m.Ticket, m.Issue = *edit.Ticket, nil
	}
	if edit.Crew != nil {
		m.Crew = *edit.Crew
		if m.Crew == "none" {
			m.Crew = ""
		}
	}
	if edit.Color != nil {
		m.Color = *edit.Color
	}
	for _, v := range []string{m.Name, m.Ticket} {
		if strings.IndexFunc(v, unicode.IsControl) >= 0 {
			return fmt.Errorf("name and ticket must not contain control characters")
		}
	}
	if err := validateIdentity(m.Crew, m.Color, crews); err != nil {
		return err
	}
	if err := saveManifest(dir, m); err != nil {
		return err
	}
	return syncAppearance([]Manifest{m}, crews)
}
