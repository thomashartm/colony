package tmux

import "testing"

func TestBindingCommandFromWholeTable(t *testing.T) {
	table := "bind-key -T prefix h display-popup 'motley'\n" +
		"bind-key -r -T prefix m display-message 'my custom binding'\n" +
		"bind-key -T prefix M select-pane -M\n"
	if got := bindingCommand(table, "m"); got != "display-message 'my custom binding'" {
		t.Fatalf("lost existing binding: %q", got)
	}
	if got := bindingCommand(table, "MouseUp1StatusLeft"); got != "" {
		t.Fatalf("unbound key picked another action: %q", got)
	}
}

func TestBindingCommandPreservesChainedFallback(t *testing.T) {
	table := `bind-key -T root MouseDrag1Pane display-message "literal \\; and \"quote\"" \; copy-mode -M`
	want := `display-message "literal \\; and \"quote\"" ; copy-mode -M`
	if got := bindingCommand(table, "MouseDrag1Pane"); got != want {
		t.Fatalf("fallback = %q, want %q", got, want)
	}
}
