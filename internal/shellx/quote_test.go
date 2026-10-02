package shellx

import (
	"os/exec"
	"testing"
)

func TestQuoteRoundTripsThroughSh(t *testing.T) {
	for _, value := range []string{"", "plain", "Epic: FX", "it's", `a "b" $c`, "`x`; rm -rf /", "new\nline"} {
		out, err := exec.Command("/bin/sh", "-c", "printf '%s' "+Quote(value)).Output()
		if err != nil || string(out) != value {
			t.Fatalf("Quote(%q) through sh = %q, %v", value, out, err)
		}
	}
}
