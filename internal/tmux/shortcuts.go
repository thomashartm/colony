package tmux

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// One shared layout supplies both displayed labels and click targets. The normal
// status row stays intact; range=left is supported by our minimum tmux 3.2.
type shortcut struct{ label, action string }

func shortcutRows(prefix string) [][]shortcut {
	return [][]shortcut{
		{{" " + prefix + " then: ", ""}, {"H monitor", "monitor"}, {" | ", ""}, {"h details", "details"}, {" | ", ""}, {"d detach", "detach"}, {" | popup: q back / Enter switch", ""}},
		{{" " + prefix + " then: ", ""}, {"w windows", "windows"}, {" | ", ""}, {"p prev", "previous"}, {" / ", ""}, {"n next", "next"}, {" | arrows panes | ", ""}, {"[ scroll (q back)", "scroll"}},
	}
}

func footerRow(row []shortcut) string {
	var text strings.Builder
	for _, item := range row {
		if item.action != "" {
			text.WriteString("#[range=left,underscore]")
		}
		text.WriteString(item.label)
		if item.action != "" {
			text.WriteString("#[norange,nounderscore]")
		}
	}
	return text.String()
}

func shortcutOptions(args []string, id, original string) []string {
	target := "=" + SessionName(id) + ":"
	rows := shortcutRows("#{prefix}")
	for _, option := range [][2]string{
		{"status-format[0]", original}, {"status", "4"}, {"status-position", "bottom"}, {"mouse", "on"},
		{"status-format[1]", footerRow(rows[0])},
		{"status-format[2]", footerRow(rows[1])},
		{"status-format[3]", " New tab: Cmd-T (Ghostty/macOS), then: mtly attach #{@motley_member}"},
	} {
		if len(args) > 0 {
			args = append(args, ";")
		}
		args = append(args, "set-option", "-t", target, option[0], option[1])
	}
	return args
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }

func navigationBindings() error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	callback := shellQuote(self) + " navigation --client #{q:client_name}"
	if err := bindNavigation("prefix", "H", "#{||:#{@motley_member},#{@motley_monitor}}", "run-shell -b "+shellQuote(callback+" --action monitor")); err != nil {
		return err
	}
	guard := "#{&&:#{@motley_member},#{>:#{mouse_status_line},0}}"
	// Dispatch on release: tmux turns rapid presses into second/triple-click keys.
	// Suppress the original press binding only over our own controls.
	if err := bindNavigation("root", "MouseDown1StatusLeft", guard, ""); err != nil {
		return err
	}
	return bindNavigation("root", "MouseUp1StatusLeft", guard,
		"run-shell -b "+shellQuote(callback+" --row #{mouse_status_line} --column #{mouse_x} --prefix #{q:prefix}"))
}

// tmux key tables are server-wide. Retain the user's binding as the fallback
// outside Motley controls, and remember it so repeated attachment never nests it.
func bindNavigation(table, key, guard, command string) error {
	option := "@motley_navigation_" + table + "_" + key
	original, err := run("show-options", "-gqv", option)
	if err != nil {
		return err
	}
	if original == "" {
		binding, lookupErr := run("list-keys", "-T", table, key)
		if lookupErr == nil {
			match := regexp.MustCompile(`^bind-key\s+(?:-r\s+)?-T\s+\S+\s+\S+\s+(.+)`).FindStringSubmatch(binding)
			if len(match) != 2 {
				return fmt.Errorf("cannot preserve tmux binding %s %s", table, key)
			}
			original = match[1]
		} else if !strings.Contains(binding, "unknown key") {
			return lookupErr
		}
		if original == "" {
			original = " "
		}
		if _, err := run("set-option", "-g", option, original); err != nil {
			return err
		}
	}
	args := []string{"bind-key", "-T", table, key, "if-shell", "-F", guard, command}
	if strings.TrimSpace(original) != "" {
		args = append(args, strings.TrimSuffix(original, "\n"))
	}
	_, err = run(args...)
	return err
}

func showShortcuts(id string) error {
	if id == MonitorSession {
		return nil
	}
	original, err := run("show-options", "-A", "-v", "-t", "="+SessionName(id)+":", "status-format[0]")
	if err != nil {
		return err
	}
	if _, err := run(shortcutOptions(nil, id, strings.TrimSuffix(original, "\n"))...); err != nil {
		return err
	}
	return navigationBindings()
}

// Navigate dispatches only fixed navigation actions, never pane input. A click
// uses the exact client that generated it, even with several attached tabs.
func Navigate(client, action, prefix string, row, column int) error {
	if action == "" {
		rows := shortcutRows(prefix)
		if row < 1 || row > len(rows) || column < 0 {
			return nil
		}
		x := 0
		for _, item := range rows[row-1] {
			if column >= x && column < x+len(item.label) {
				action = item.action
				break
			}
			x += len(item.label)
		}
	}
	if action == "" {
		return nil
	}
	clients, err := Clients()
	if err != nil {
		return err
	}
	session := ""
	for _, c := range clients {
		if c.Name == client {
			session = c.Session
			break
		}
	}
	if session == "" {
		return fmt.Errorf("navigation client is no longer attached")
	}
	// Only sessions managed by Motley receive navigation commands.
	sessions, err := Sessions()
	if err != nil {
		return err
	}
	managed := false
	for _, s := range sessions {
		if s.Name == session && (s.MemberID != "" || s.Monitor) {
			managed = true
		}
	}
	if !managed {
		return nil
	}
	if action == "monitor" {
		if session == MonitorSession {
			_, err = run("switch-client", "-c", client, "-l")
			return err
		}
		if err := EnsureMonitor(); err != nil {
			return err
		}
		return SwitchClient(client, MonitorSession)
	}
	if action == "detach" {
		return DetachClient(client)
	}
	if action == "details" {
		self, err := os.Executable()
		if err != nil {
			return err
		}
		_, err = run("display-popup", "-c", client, "-E", "-w", "90%", "-h", "85%", shellQuote(self)+" --client "+shellQuote(client))
		return err
	}
	// Resolve the active pane for this client at dispatch time.
	pane, err := run("display-message", "-p", "-t", "="+session+":", "#{pane_id}")
	if err != nil {
		return err
	}
	target := strings.TrimSpace(pane)
	switch action {
	case "windows":
		_, err = run("choose-tree", "-Zw", "-t", target)
	case "previous":
		_, err = run("previous-window", "-t", "="+session)
	case "next":
		_, err = run("next-window", "-t", "="+session)
	case "scroll":
		_, err = run("copy-mode", "-t", target)
	default:
		return fmt.Errorf("unknown navigation action %q", action)
	}
	return err
}

func monitorNavigation() error {
	if _, err := run("set-option", "-t", "="+MonitorSession+":", "mouse", "on",
		";", "set-option", "-t", "="+MonitorSession+":", "status-right", "#{prefix} H: previous session",
		";", "set-option", "-t", "="+MonitorSession+":", "status-right-length", "60"); err != nil {
		return err
	}
	return navigationBindings()
}
