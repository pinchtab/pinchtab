// Package all registers all built-in browser providers.
package all

import (
	_ "github.com/pinchtab/pinchtab/internal/browsers/chrome/chromekit"
	_ "github.com/pinchtab/pinchtab/internal/browsers/cloak/cloakkit"
	_ "github.com/pinchtab/pinchtab/internal/browsers/ghostchrome/bridgekit"
)
