// Package claudehooks contains the versioned Claude integration template.
package claudehooks

import _ "embed"

//go:embed settings.hooks.json
var Settings []byte
