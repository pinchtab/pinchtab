package assets

import (
	_ "embed"
)

//go:embed stealth.js
var StealthScript string

//go:embed popup_guard.js
var PopupGuardScript string

//go:embed readability.js
var ReadabilityJS string

//go:embed screencast_repaint_start.js
var ScreencastRepaintStartJS string

//go:embed screencast_repaint_stop.js
var ScreencastRepaintStopJS string

//go:embed welcome.html
var WelcomeHTML string
