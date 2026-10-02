// Package shellx quotes values for POSIX shells.
package shellx

import "strings"

// Quote returns value as one single-quoted POSIX shell word.
func Quote(value string) string { return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'" }
