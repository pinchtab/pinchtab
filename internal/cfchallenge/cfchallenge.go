// Package cfchallenge holds the substrate-free Cloudflare challenge data
// (title indicators, challenge-type tokens, turnstile selector JS, spinner
// text) shared verbatim by the two Cloudflare solver flows —
// internal/bridge/cloudflare.go and internal/autosolver/solvers/cloudflare.go —
// so the duplicated copies can't drift. It depends on nothing but the stdlib so
// both packages can import it without an import cycle. Control flow (jitter,
// attempt counts, retry, detect/solve) stays in each flow; only the verbatim
// data lives here.
package cfchallenge

import "strings"

// TitleIndicators are lowercase substrings that mark a Cloudflare challenge
// interstitial in the page <title>.
var TitleIndicators = []string{
	"just a moment",
	"attention required",
	"checking your browser",
}

// IsChallengeTitle reports whether a page title looks like a Cloudflare
// challenge page.
func IsChallengeTitle(title string) bool {
	lower := strings.ToLower(title)
	for _, ind := range TitleIndicators {
		if strings.Contains(lower, ind) {
			return true
		}
	}
	return false
}

// CTypeTokens are the Turnstile cType values scanned for in page HTML, in
// priority order. Each is matched against the literal `cType: '<token>'`.
var CTypeTokens = []string{"non-interactive", "managed", "interactive"}

// SpinnerText is the Turnstile "verifying" spinner text; its disappearance from
// document.body.innerText signals the spinner has cleared.
const SpinnerText = "Verifying you are human"

// EmbeddedTurnstileScriptJS evaluates to true when the Turnstile script tag is
// present on the page.
const EmbeddedTurnstileScriptJS = `!!document.querySelector('script[src*="challenges.cloudflare.com/turnstile/v"]')`

// TurnstileBoxJS is an IIFE that returns the Turnstile widget's bounding box
// ({x,y,width,height}) or null. Whitespace is irrelevant to the runtime result.
const TurnstileBoxJS = `(() => {
	const patterns = [
		'iframe[src*="challenges.cloudflare.com/cdn-cgi/challenge-platform"]',
		'iframe[src*="challenges.cloudflare.com"]',
	];
	for (const sel of patterns) {
		const iframe = document.querySelector(sel);
		if (iframe) {
			const r = iframe.getBoundingClientRect();
			if (r.width > 0 && r.height > 0) {
				return {x: r.x, y: r.y, width: r.width, height: r.height};
			}
		}
	}
	const containers = [
		'#cf_turnstile div', '#cf-turnstile div', '.turnstile>div>div',
		'.main-content p+div>div>div',
	];
	for (const sel of containers) {
		const el = document.querySelector(sel);
		if (el) {
			const r = el.getBoundingClientRect();
			if (r.width > 0 && r.height > 0) {
				return {x: r.x, y: r.y, width: r.width, height: r.height};
			}
		}
	}
	return null;
})()`
