package tmux

import "testing"

func TestBindingCommandFromWholeTable(t *testing.T) {
	table := "bind-key -T prefix h display-popup 'motley'\n" +
		"bind-key -r -T prefix H display-message 'my custom binding'\n" +
		"bind-key -T prefix M select-pane -M\n"
	if got := bindingCommand(table, "H"); got != "display-message 'my custom binding'" {
		t.Fatalf("lost existing binding: %q", got)
	}
	if got := bindingCommand(table, "MouseUp1StatusLeft"); got != "" {
		t.Fatalf("unbound key picked another action: %q", got)
	}
}
