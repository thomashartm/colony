package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thomashartm/motley/internal/crew"
	"github.com/thomashartm/motley/internal/member"
)

func TestCrewsAndLiveColours(t *testing.T) {
	bin := buildLifecycleBinary(t)
	f := newMemberFixture(t, bin, "main")
	f.motley("crew", "add", "--title", "FX & Banking", "--gig", " Ship FX caching ", "--url", "https://github.com/acme/api/issues/400", "--color", "blue")
	f.motley("crew", "add", "--title", "FX & Banking")
	f.motley("crew", "add", "--title", "Spikes")
	var crews []crew.Crew
	if err := json.Unmarshal([]byte(f.motley("crew", "list", "--json")), &crews); err != nil {
		t.Fatal(err)
	}
	if len(crews) != 3 || crews[0].ID != "fx-banking" || crews[0].Kind != "issue" || crews[1].ID != "fx-banking-2" || crews[1].Color != "red" || crews[2].Color != "orange" {
		t.Fatalf("crews: %+v", crews)
	}
	if crews[0].Gig != "Ship FX caching" || !strings.Contains(f.motley("crew", "list"), "Ship FX caching") {
		t.Fatalf("missing gig: %+v", crews[0])
	}
	f.refused("crew", "edit", "fx-banking", "--gig", "bad\x1bgig")
	f.motley("crew", "edit", "fx-banking", "--gig", " Banking rollout ")
	if err := json.Unmarshal([]byte(f.motley("crew", "list", "--json")), &crews); err != nil || crews[0].Gig != "Banking rollout" {
		t.Fatalf("edit gig: %+v, %v", crews, err)
	}
	f.motley("crew", "edit", "fx-banking", "--gig", "")
	if err := json.Unmarshal([]byte(f.motley("crew", "list", "--json")), new([]crew.Crew)); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(f.motley("crew", "list", "--json"), "\"gig\"") {
		t.Fatal("empty gig was not cleared")
	}
	f.refused("crew", "add", "--title", "")
	f.refused("crew", "edit", "fx-banking", "--color", "pink")
	f.refused("spawn", "--repo", "api", "--branch", "feat/invalid", "--crew", "missing", "--detach")
	if _, err := exec.Command("git", "-C", f.repo, "show-ref", "--verify", "refs/heads/feat/invalid").Output(); err == nil {
		t.Fatal("invalid crew created a branch")
	}
	for _, branch := range []string{"inherited", "override"} {
		args := []string{"spawn", "--repo", "api", "--branch", "feat/" + branch, "--crew", "fx-banking", "--detach"}
		if branch == "override" {
			args = append(args, "--color", "green")
		}
		f.motley(args...)
	}
	read := func(id string) string {
		return f.tmux("-u", "display-message", "-p", "-t", "="+id+":", "#{@motley_crew}|#{@motley_color}|#{@motley_emoji}|#{status-style}|#{E:set-titles-string}")
	}
	if got := read("feat-inherited"); !strings.Contains(got, "FX & Banking|blue|🔵|bg=colour33,fg=white") {
		t.Fatal(got)
	}
	if m := f.manifest("feat-inherited"); m.Crew != "fx-banking" || m.Color != "" {
		t.Fatalf("manifest must retain inheritance: %+v", m)
	}
	f.motley("crew", "edit", "fx-banking", "--title", "Banking", "--color", "yellow", "--url", "")
	if got := read("feat-inherited"); !strings.Contains(got, "Banking|yellow|🟡|bg=colour220,fg=black") {
		t.Fatal("live recolour", got)
	}
	if got := read("feat-override"); !strings.Contains(got, "Banking|green|🟢") {
		t.Fatal("override lost", got)
	}
	f.motley("crew", "assign", "feat-override", "spikes")
	if got := read("feat-override"); !strings.HasPrefix(got, "Spikes|green|") {
		t.Fatal(got)
	}
	// Editing names and tickets must pass literal text to tmux, not formats.
	name := "Literal #{session_name}"
	ticket := "88"
	color := "purple"
	if err := member.EditIdentity("feat-inherited", member.IdentityEdit{Name: &name, Ticket: &ticket, Color: &color}); err != nil {
		t.Fatal(err)
	}
	if got := read("feat-inherited"); !strings.Contains(got, "🟣 88 Literal #{session_name}") {
		t.Fatal("title interpreted as a format", got)
	}
	f.tmux("kill-session", "-t", "=feat-inherited")
	if out := f.refused("crew", "rm", "fx-banking"); !strings.Contains(out, "1 members") {
		t.Fatal("dead references must also block removal", out)
	}
	f.motley("crew", "rm", "fx-banking", "--force")
	if m := f.manifest("feat-inherited"); m.Crew != "" || m.Color != "purple" {
		t.Fatal("forced unassign lost override", m)
	}
	f.motley("revive", "feat-inherited")
	if got := read("feat-inherited"); !strings.HasPrefix(got, "|purple|🟣") {
		t.Fatal("revive lost colour", got)
	}
	f.motley("crew", "assign", "feat-inherited", "spikes")
	f.motley("crew", "rm", "spikes", "--force")
	if got := read("feat-override"); !strings.HasPrefix(got, "|green|") {
		t.Fatal("live force-unassign failed", got)
	}
	// Adoption applies the same crew and colour rules.
	path := filepath.Join(f.trees, "manual")
	f.git(f.repo, "worktree", "add", "-b", "feat/manual", path, "main")
	f.tmux("new-session", "-d", "-s", "manual", "-c", path, "/bin/sh")
	t.Setenv("TMUX_PANE", f.tmux("display-message", "-p", "-t", "=manual:", "#{pane_id}"))
	cmd := exec.Command(bin, "adopt", "--crew", "fx-banking-2", "--color", "brown")
	cmd.Dir = path
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("adopt: %v %s", err, out)
	}
	if got := read("feat-manual"); !strings.Contains(got, "FX & Banking|brown|🟤") {
		t.Fatal("adopt colour", got)
	}
	data, err := os.ReadFile(filepath.Join(f.state, "motley/crews.toml"))
	if err != nil || !strings.Contains(string(data), "schema = 1") {
		t.Fatal("crew schema missing", err)
	}
}
