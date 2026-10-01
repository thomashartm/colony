package palette

import "testing"

func TestResolution(t *testing.T) {
	if got := Resolve("member", "red", "blue"); got.Name != "red" {
		t.Fatal("override lost", got)
	}
	if got := Resolve("member", "", "blue"); got.Name != "blue" {
		t.Fatal("crew colour lost", got)
	}
	if a, b := Resolve("member", "", ""), Resolve("member", "", ""); a != b || a.Name == "" {
		t.Fatal("hash must be deterministic")
	}
	for _, c := range Colors {
		if c.Emoji == "" || c.Tmux == "" || c.Hex == "" || c.Foreground == "" {
			t.Fatal("incomplete palette", c)
		}
	}
	label, c := Badge("claude")
	if label != "CC" || c.Name != "orange" {
		t.Fatal(label, c)
	}
}
