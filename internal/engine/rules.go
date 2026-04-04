package engine

// RouteRule inspects an incoming operation and returns a routing decision.
// Rules are evaluated in order; the first non-Undecided verdict wins.
type RouteRule interface {
	// Name returns a short human-readable identifier for logging.
	Name() string
	// Decide returns UseLite, UseChrome, or Undecided.
	Decide(op Capability, url string) Decision
}

// ---------- built-in rules ----------

// CapabilityRule routes chrome-only operations (screenshot, pdf, evaluate,
// cookies) to Chrome unconditionally.
type CapabilityRule struct{}

func (CapabilityRule) Name() string { return "capability" }

func (CapabilityRule) Decide(op Capability, _ string) Decision {
	switch op {
	case CapScreenshot, CapPDF, CapEvaluate, CapCookies:
		return UseChrome
	}
	return Undecided
}

// ContentHintRule uses a simple heuristic: if a URL path ends with common
// static-content extensions it is a good candidate for the lite engine.
type ContentHintRule struct{}

func (ContentHintRule) Name() string { return "content-hint" }

func (ContentHintRule) Decide(op Capability, url string) Decision {
	if op != CapNavigate && op != CapSnapshot && op != CapText {
		return Undecided
	}
	// Static content is well-suited for Gost-DOM because it does not
	// rely on JavaScript rendering.
	for _, ext := range []string{".html", ".htm", ".xml", ".txt", ".md"} {
		if len(url) > len(ext) && url[len(url)-len(ext):] == ext {
			return UseLite
		}
	}
	return Undecided
}

// DefaultLiteRule is a catch-all that sends every remaining DOM operation
// to the lite engine.  Used when Mode == ModeLite.
type DefaultLiteRule struct{}

func (DefaultLiteRule) Name() string { return "default-lite" }

func (DefaultLiteRule) Decide(op Capability, _ string) Decision {
	switch op {
	case CapNavigate, CapSnapshot, CapText, CapClick, CapType:
		return UseLite
	}
	return Undecided
}

// LightpandaCapabilityRule routes chrome-only operations to Chrome and
// everything Lightpanda supports to the alt engine.
type LightpandaCapabilityRule struct{}

func (LightpandaCapabilityRule) Name() string { return "lp-capability" }

func (LightpandaCapabilityRule) Decide(op Capability, _ string) Decision {
	switch op {
	case CapScreenshot, CapPDF, CapEvaluate, CapCookies:
		return UseChrome
	case CapNavigate, CapSnapshot, CapText, CapClick, CapType:
		return UseAlt
	}
	return Undecided
}

// DefaultLightpandaRule is a catch-all that sends every remaining DOM
// operation to the Lightpanda engine. Used when Mode == ModeLightpanda.
type DefaultLightpandaRule struct{}

func (DefaultLightpandaRule) Name() string { return "default-lightpanda" }

func (DefaultLightpandaRule) Decide(op Capability, _ string) Decision {
	switch op {
	case CapNavigate, CapSnapshot, CapText, CapClick, CapType:
		return UseAlt
	}
	return Undecided
}

// DefaultChromeRule is a catch-all that sends everything to Chrome.
// Used as the final fallback in ModeAuto.
type DefaultChromeRule struct{}

func (DefaultChromeRule) Name() string { return "default-chrome" }

func (DefaultChromeRule) Decide(_ Capability, _ string) Decision {
	return UseChrome
}
