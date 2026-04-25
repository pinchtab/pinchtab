package autosolver

import "strings"

// detectIntentByTitle provides a lightweight fallback for intent
// detection when the semantic engine is unavailable. It uses
// well-known page-title patterns to classify the page.
func detectIntentByTitle(title string) *Intent {
	lower := strings.ToLower(title)
	if challenge := DetectChallengeIntent(title, "", ""); challenge != nil {
		return challenge
	}

	// Cloudflare challenge patterns
	cfPatterns := []string{"just a moment", "attention required", "checking your browser"}
	for _, p := range cfPatterns {
		if strings.Contains(lower, p) {
			return &Intent{
				Type:          IntentCaptcha,
				Confidence:    0.9,
				ChallengeType: "cloudflare",
				Details:       "cloudflare challenge detected via title",
			}
		}
	}

	// Generic CAPTCHA patterns
	captchaPatterns := []string{"captcha", "verify you are human", "robot", "bot detection"}
	for _, p := range captchaPatterns {
		if strings.Contains(lower, p) {
			return &Intent{
				Type:       IntentCaptcha,
				Confidence: 0.7,
				Details:    "generic captcha detected via title",
			}
		}
	}

	// Login patterns
	loginPatterns := []string{"log in", "login", "sign in", "signin"}
	for _, p := range loginPatterns {
		if strings.Contains(lower, p) {
			return &Intent{
				Type:       IntentLogin,
				Confidence: 0.6,
				Details:    "login page detected via title",
			}
		}
	}

	// Signup patterns
	signupPatterns := []string{"sign up", "signup", "register", "create account", "join"}
	for _, p := range signupPatterns {
		if strings.Contains(lower, p) {
			return &Intent{
				Type:       IntentSignup,
				Confidence: 0.6,
				Details:    "signup page detected via title",
			}
		}
	}

	// Onboarding patterns
	onboardingPatterns := []string{"getting started", "welcome", "onboarding", "complete your profile", "step 1"}
	for _, p := range onboardingPatterns {
		if strings.Contains(lower, p) {
			return &Intent{
				Type:       IntentOnboarding,
				Confidence: 0.65,
				Details:    "onboarding flow detected via title",
			}
		}
	}

	// Navigation/task-flow patterns
	navigationPatterns := []string{"continue", "next step", "choose option", "select plan", "wizard"}
	for _, p := range navigationPatterns {
		if strings.Contains(lower, p) {
			return &Intent{
				Type:       IntentNavigation,
				Confidence: 0.6,
				Details:    "navigation flow detected via title",
			}
		}
	}

	// Generic form-filling patterns
	formPatterns := []string{"application form", "contact form", "checkout", "survey", "questionnaire"}
	for _, p := range formPatterns {
		if strings.Contains(lower, p) {
			return &Intent{
				Type:       IntentForm,
				Confidence: 0.6,
				Details:    "form flow detected via title",
			}
		}
	}

	// Blocked/access-denied patterns
	blockedPatterns := []string{"access denied", "forbidden", "blocked", "unauthorized"}
	for _, p := range blockedPatterns {
		if strings.Contains(lower, p) {
			return &Intent{
				Type:       IntentBlocked,
				Confidence: 0.7,
				Details:    "blocked page detected via title",
			}
		}
	}

	return &Intent{
		Type:       IntentNormal,
		Confidence: 0.5,
		Details:    "no challenge indicators found in title",
	}
}
