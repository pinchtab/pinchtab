package autosolver

import "strings"

// DetectChallengeIntent classifies known challenge pages using title, URL,
// and HTML markers. It returns nil when no challenge signal is found.
func DetectChallengeIntent(title, url, html string) *Intent {
	lowerTitle := strings.ToLower(title)
	lowerURL := strings.ToLower(url)
	lowerHTML := strings.ToLower(html)
	v3 := isRecaptchaV3Challenge(lowerURL, lowerHTML)

	if isTurnstileChallenge(lowerTitle, lowerURL, lowerHTML) {
		return &Intent{
			Type:          IntentCaptcha,
			Confidence:    0.95,
			ChallengeType: "turnstile",
			Details:       "cloudflare turnstile challenge detected",
		}
	}

	if v3 {
		return &Intent{
			Type:          IntentCaptcha,
			Confidence:    0.9,
			ChallengeType: "recaptcha-v3",
			Details:       "reCAPTCHA v3 challenge detected",
		}
	}

	if !v3 && isRecaptchaV2Challenge(lowerURL, lowerHTML) {
		return &Intent{
			Type:          IntentCaptcha,
			Confidence:    0.9,
			ChallengeType: "recaptcha-v2",
			Details:       "reCAPTCHA v2 challenge detected",
		}
	}

	if isHCaptchaChallenge(lowerURL, lowerHTML) {
		return &Intent{
			Type:          IntentCaptcha,
			Confidence:    0.9,
			ChallengeType: "hcaptcha",
			Details:       "hCaptcha challenge detected",
		}
	}

	if isCustomJSChallenge(lowerTitle, lowerURL, lowerHTML) {
		return &Intent{
			Type:          IntentBlocked,
			Confidence:    0.85,
			ChallengeType: "custom-js",
			Details:       "custom JavaScript anti-bot challenge detected",
		}
	}

	if isGenericCaptcha(lowerTitle, lowerURL, lowerHTML) {
		return &Intent{
			Type:          IntentCaptcha,
			Confidence:    0.7,
			ChallengeType: "captcha-generic",
			Details:       "generic captcha challenge detected",
		}
	}

	return nil
}

// isGenericCaptcha is the last rung: no vendor was identified, so all that is left
// is the word itself. It applies the same rule the vendor checks above do — the
// title and the URL are guesses, and a guess is only worth making when there is no
// page to check it against.
//
// Every branch used to be sufficient on its own, and the URL branch listed
// "turnstile", which is a word about stadiums as often as about Cloudflare. An
// operations report at /ops/turnstile-report, or an article at
// /blog/how-recaptcha-works, was a captcha challenge as far as this was concerned.
//
// What remains deliberately ambiguous is a page whose BODY says "captcha". A
// vendor comparison and a captcha page both contain the word, and nothing here can
// separate them; 0.7 and a name of "generic" is what that uncertainty is for.
func isGenericCaptcha(title, url, html string) bool {
	if containsAny(html, "captcha", "verify you are human", "i am not a robot") {
		return true
	}
	if html == "" {
		return containsAny(title, "captcha", "verify you are human", "i am not a robot") ||
			containsAny(url, "captcha", "turnstile")
	}
	return false
}

func isTurnstileChallenge(title, url, html string) bool {
	if containsAny(url,
		"cdn-cgi/challenge-platform",
		"/cdn-cgi/challenge",
	) || containsAny(html,
		"challenges.cloudflare.com/turnstile",
		"cf-turnstile",
		"turnstile.render(",
		// An interstitial is served AT the address that was asked for, so the
		// challenge-platform script shows up in the markup while the URL still
		// looks like the site. Matching it only in the URL meant the real page
		// was recognised by its title and not by the thing running it.
		"cdn-cgi/challenge-platform",
		"__cf_chl_",
		"cf-browser-verification",
	) {
		return true
	}

	// The title is a guess, and it is only worth making when there is no page to
	// check it against. "Attention required" and "just a moment" are ordinary
	// English: a legal notice headed "Attention Required Before You Sign", or a
	// support page saying "Just a moment while we finish setup", was being called
	// a Cloudflare interstitial at 0.95 confidence and sent to the solver instead
	// of being read.
	//
	// An interstitial that reached us WITH its markup always carries a marker
	// above — the challenge-platform script is what runs it. So once html is in
	// hand, a bare title match is a coincidence. With no html there is nothing to
	// corroborate against and the title is all the caller has, which is the mode
	// heuristics.go asks for by passing url and html empty.
	if html == "" {
		return containsAny(title,
			"just a moment",
			"attention required",
			"checking your browser",
		)
	}
	return false
}

func isRecaptchaV3Challenge(url, html string) bool {
	return containsAny(html,
		"grecaptcha.execute(",
		"grecaptcha.enterprise.execute(",
		"recaptcha/api.js?render=",
		"recaptcha/enterprise.js?render=",
	)
}

func isRecaptchaV2Challenge(url, html string) bool {
	// Matched a bare "recaptcha" anywhere in the URL, which is a word a URL is
	// allowed to contain: /blog/how-recaptcha-works was classified as a reCAPTCHA
	// challenge. The vendor's own host is the signal; the word is not.
	return containsAny(url,
		"google.com/recaptcha",
	) || containsAny(html,
		"g-recaptcha",
		"recaptcha-checkbox",
		"api2/anchor",
		"google.com/recaptcha/api.js",
	)
}

func isHCaptchaChallenge(url, html string) bool {
	// Same correction, and the html list had the same flaw more sharply: a bare
	// "hcaptcha" matches any page that merely NAMES hCaptcha, so vendor
	// comparisons and docs about captchas were themselves read as captchas. The
	// script and the widget class stay, because those are the thing rather than
	// the word for it.
	return containsAny(url,
		"hcaptcha.com",
	) || containsAny(html,
		"hcaptcha.com/1/api.js",
		"h-captcha",
	)
}

func isCustomJSChallenge(title, url, html string) bool {
	titleSignal := containsAny(title,
		"please enable javascript",
		"browser integrity check",
		"access denied",
		"forbidden",
		"blocked",
	)
	urlSignal := containsAny(url,
		"challenge",
		"bot",
		"verify",
	)
	htmlSignal := containsAny(html,
		"__cf_chl",
		"window._cf_chl_opt",
		"challenge-form",
		"jschl",
		"bot challenge",
		"anti-bot",
		"anti bot",
		"please enable javascript",
		"checking your browser before accessing",
		"browser integrity check",
		"navigator.webdriver",
	)

	// Primary signal comes from HTML markers.
	if htmlSignal {
		return true
	}

	// Fallback when HTML cannot be read.
	if html == "" && titleSignal && urlSignal {
		return true
	}

	return false
}

func containsAny(haystack string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(haystack, needle) {
			return true
		}
	}
	return false
}
