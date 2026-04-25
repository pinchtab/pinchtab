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

	if containsAny(lowerTitle, "captcha", "verify you are human", "i am not a robot") ||
		containsAny(lowerURL, "captcha", "recaptcha", "hcaptcha", "turnstile") ||
		containsAny(lowerHTML, "captcha", "verify you are human", "i am not a robot") {
		return &Intent{
			Type:          IntentCaptcha,
			Confidence:    0.7,
			ChallengeType: "captcha-generic",
			Details:       "generic captcha challenge detected",
		}
	}

	return nil
}

func isTurnstileChallenge(title, url, html string) bool {
	return containsAny(title,
		"just a moment",
		"attention required",
		"checking your browser",
	) || containsAny(url,
		"cdn-cgi/challenge-platform",
		"/cdn-cgi/challenge",
	) || containsAny(html,
		"challenges.cloudflare.com/turnstile",
		"cf-turnstile",
		"turnstile.render(",
	)
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
	return containsAny(url,
		"recaptcha",
		"google.com/recaptcha",
	) || containsAny(html,
		"g-recaptcha",
		"recaptcha-checkbox",
		"api2/anchor",
		"google.com/recaptcha/api.js",
	)
}

func isHCaptchaChallenge(url, html string) bool {
	return containsAny(url,
		"hcaptcha",
	) || containsAny(html,
		"hcaptcha.com/1/api.js",
		"h-captcha",
		"hcaptcha",
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
