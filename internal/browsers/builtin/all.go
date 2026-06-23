// Package builtin registers the built-in browser providers without any
// server/bridge-side hook wiring. It is safe to import from bridge/runtime.
package builtin

import (
	_ "github.com/pinchtab/pinchtab/internal/browsers/chrome"
	_ "github.com/pinchtab/pinchtab/internal/browsers/cloak"
	_ "github.com/pinchtab/pinchtab/internal/browsers/ghostchrome"
)
