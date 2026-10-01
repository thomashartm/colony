package member

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/thomashartm/motley/internal/tmux"
)

func TestCatalogReloadAndJoin(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.toml")
	write := func(name string) {
		t.Helper()
		if err := os.WriteFile(path, []byte("schema=1\nid='a'\nname='"+name+"'\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("first")
	c := &Catalog{}
	first, err := c.Load(dir)
	if err != nil || first[0].Name != "first" {
		t.Fatalf("load %v %v", first, err)
	}
	write("changed")
	now := time.Now().Add(time.Second)
	if err := os.Chtimes(path, now, now); err != nil {
		t.Fatal(err)
	}
	next, err := c.Load(dir)
	if err != nil || next[0].Name != "changed" || first[0].Name != "first" {
		t.Fatalf("reload %v %v", next, err)
	}
	if !Join(next, []tmux.Session{{Name: "a", MemberID: "a"}})[0].Alive {
		t.Fatal("live join")
	}
	if Join(next, []tmux.Session{{Name: "a"}, {Name: "other", MemberID: "a"}})[0].Alive {
		t.Fatal("unmarked or wrong session cannot make member live")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	gone, err := c.Load(dir)
	if err != nil || len(gone) != 0 {
		t.Fatalf("removed %v %v", gone, err)
	}
}
