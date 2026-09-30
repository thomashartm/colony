// Package palette keeps terminal and overview identity colours consistent.
package palette

import "hash/fnv"

type Color struct{ Name, Tmux, Hex, Emoji, Foreground string }

var Colors = []Color{
	{"red", "colour160", "#d70000", "🔴", "white"},
	{"orange", "colour208", "#ff8700", "🟠", "black"},
	{"yellow", "colour220", "#ffd700", "🟡", "black"},
	{"green", "colour34", "#00af00", "🟢", "black"},
	{"blue", "colour33", "#0087ff", "🔵", "white"},
	{"purple", "colour135", "#af5fff", "🟣", "white"},
	{"brown", "colour130", "#af5f00", "🟤", "white"},
	{"grey", "colour245", "#8a8a8a", "⚪", "black"},
}

func Lookup(name string) (Color, bool) {
	for _, c := range Colors {
		if c.Name == name {
			return c, true
		}
	}
	return Color{}, false
}
func Resolve(id, override, crew string) Color {
	if c, ok := Lookup(override); ok {
		return c
	}
	if c, ok := Lookup(crew); ok {
		return c
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(id))
	return Colors[int(h.Sum32())%len(Colors)]
}
func Badge(agent string) (string, Color) {
	name, color := "??", "grey"
	switch agent {
	case "claude":
		name, color = "CC", "orange"
	case "codex":
		name, color = "CX", "green"
	case "opencode":
		name, color = "OC", "purple"
	}
	c, _ := Lookup(color)
	return name, c
}
