package tmux

import (
	"strings"

	"github.com/thomashartm/motley/internal/palette"
)

func Appearance(id, name, ticket, agent, crew string, color palette.Color) error {
	target := "=" + SessionName(id) + ":"
	title := strings.TrimSpace(color.Emoji + " " + ticket + " " + name)
	// A name is literal text, never an executable tmux format such as #(command).
	title = strings.ReplaceAll(title, "#", "##")
	options := [][2]string{{"@motley_ticket", ticket}, {"@motley_agent", agent}, {"@motley_crew", crew}, {"@motley_color", color.Name}, {"@motley_emoji", color.Emoji}, {"status-style", "bg=" + color.Tmux + ",fg=" + color.Foreground}, {"set-titles", "on"}, {"set-titles-string", title}}
	var args []string
	for _, o := range options {
		if len(args) > 0 {
			args = append(args, ";")
		}
		args = append(args, "set-option", "-t", target, o[0], o[1])
	}
	_, err := run(args...)
	return err
}
