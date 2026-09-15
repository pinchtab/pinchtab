package semantic

import (
	"context"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/autosolver"
)

type stubPage struct {
	title string
	url   string
	html  string
}

func (p stubPage) URL() string                              { return p.url }
func (p stubPage) Title() string                            { return p.title }
func (p stubPage) HTML() (string, error)                    { return p.html, nil }
func (p stubPage) HTMLWithin(time.Duration) (string, error) { return p.html, nil }
func (p stubPage) Screenshot() ([]byte, error)              { return nil, nil }

func detect(t *testing.T, title, url string) *autosolver.Intent {
	t.Helper()
	intent, err := NewAdapter(nil).DetectIntent(context.Background(), stubPage{title: title, url: url})
	if err != nil {
		t.Fatalf("DetectIntent(%q, %q): %v", title, url, err)
	}
	return intent
}

// Mixed case on purpose, but these rows survive dropping either lowercasing on its
// own — DetectIntent folds the title and DetectChallengeIntent folds again — so what
// they pin is that the pair still folds between them. The adapter's own ToLower is
// pinned by the local-rule and golden rows below, which reach the tables the canonical
// detector does not refold for; DetectChallengeIntent's is pinned on the autosolver
// side, the only entry point that hands it a raw title.
var cloudflareTriggerTitles = []string{
	"Just a moment...",
	"Attention Required! | Cloudflare",
	"Checking your browser before accessing",
}

func TestDetectIntent_CloudflareTitlesAreTurnstile(t *testing.T) {
	for _, title := range cloudflareTriggerTitles {
		intent := detect(t, title, "")
		if intent.ChallengeType != "turnstile" {
			t.Fatalf("%q: expected turnstile challenge type, got %q", title, intent.ChallengeType)
		}
		if intent.Confidence != 0.95 {
			t.Fatalf("%q: expected confidence 0.95, got %v", title, intent.Confidence)
		}
		if intent.Details != "cloudflare turnstile challenge detected" {
			t.Fatalf("%q: expected the canonical detector's details, got %q", title, intent.Details)
		}
	}
}

// ruleReachability names one title per pattern in captchaRules and flowRules.
// Each row must reach the local rule rather than DetectChallengeIntent, which
// runs first; a pattern the canonical detector already answers cannot appear
// here and must be deleted from its table instead.
var ruleReachability = []struct {
	pattern string
	title   string
}{
	{"challenge", "Security challenge"},
	{"verify", "Verify your identity"},
	{"log in", "Log In"},
	{"login", "Login"},
	{"sign in", "Sign In"},
	{"sign up", "Sign Up"},
	{"signup", "Signup"},
	{"register", "Register"},
	{"create account", "Create Account"},
	{"getting started", "Getting Started"},
	{"welcome", "Welcome"},
	{"onboarding", "Onboarding"},
	{"complete your profile", "Complete your profile"},
	{"step 1", "Step 1 of 3"},
	{"continue", "Continue"},
	{"next step", "Next step"},
	{"choose option", "Choose option"},
	{"select plan", "Select plan"},
	{"wizard", "Setup wizard"},
	{"application form", "Application form"},
	{"contact form", "Contact form"},
	{"checkout", "Checkout"},
	{"survey", "Survey"},
	{"questionnaire", "Questionnaire"},
}

func TestIntentRules_EveryPatternIsReachable(t *testing.T) {
	covered := make(map[string]string, len(ruleReachability))
	for _, row := range ruleReachability {
		if prev, dup := covered[row.pattern]; dup {
			t.Fatalf("pattern %q covered twice (%q and %q)", row.pattern, prev, row.title)
		}
		covered[row.pattern] = row.title
	}

	var rules []autosolver.IntentRule
	rules = append(rules, captchaRules...)
	rules = append(rules, flowRules...)

	for _, rule := range rules {
		for _, pattern := range rule.Patterns {
			title, ok := covered[pattern]
			if !ok {
				t.Fatalf("pattern %q has no reachability row; prove it reachable or delete it", pattern)
			}
			intent := detect(t, title, "")
			if intent.Details != rule.Details {
				t.Fatalf("pattern %q via %q: expected local details %q, got %q", pattern, title, rule.Details, intent.Details)
			}
			if intent.Type != rule.Type || intent.Confidence != rule.Confidence {
				t.Fatalf("pattern %q via %q: expected %q/%v, got %q/%v", pattern, title, rule.Type, rule.Confidence, intent.Type, intent.Confidence)
			}
			if intent.ChallengeType != "" {
				t.Fatalf("pattern %q via %q: local rules must not set a challenge type, got %q", pattern, title, intent.ChallengeType)
			}
			delete(covered, pattern)
		}
	}

	for pattern := range covered {
		t.Fatalf("reachability row for %q matches no pattern in captchaRules or flowRules", pattern)
	}
}

func TestCaptchaRules_MatchTheURLToo(t *testing.T) {
	for _, url := range []string{"https://example.com/challenge/step", "https://example.com/verify-me"} {
		intent := detect(t, "Home page", url)
		if intent.Details != "captcha detected via semantic analysis" {
			t.Fatalf("%q: expected the local captcha rule, got %q", url, intent.Details)
		}
	}
}

// detectIntentGolden is the classification an agent observes, captured before the
// ladders became data-driven. The trailing rows each match two rules, which is what
// pins the order inside flowRules — every single-match title leaves the order free.
// The last one matches only onboarding here and signup on the autosolver side: "join"
// is a heuristics-only pattern, so it pins that the two tables differ in CONTENT while
// sharing MatchIntentRules' semantics.
var detectIntentGolden = []struct {
	title         string
	url           string
	intentType    autosolver.IntentType
	challengeType string
	confidence    float64
	details       string
}{
	{"Just a moment...", "", autosolver.IntentCaptcha, "turnstile", 0.95, "cloudflare turnstile challenge detected"},
	{"Attention Required! | Cloudflare", "", autosolver.IntentCaptcha, "turnstile", 0.95, "cloudflare turnstile challenge detected"},
	{"Checking your browser before accessing", "", autosolver.IntentCaptcha, "turnstile", 0.95, "cloudflare turnstile challenge detected"},
	{"Captcha required", "", autosolver.IntentCaptcha, "captcha-generic", 0.7, "generic captcha challenge detected"},
	{"Please verify you are human", "", autosolver.IntentCaptcha, "captcha-generic", 0.7, "generic captcha challenge detected"},
	{"I am not a robot", "", autosolver.IntentCaptcha, "captcha-generic", 0.7, "generic captcha challenge detected"},
	{"reCAPTCHA verification", "", autosolver.IntentCaptcha, "captcha-generic", 0.7, "generic captcha challenge detected"},
	{"hCaptcha check", "", autosolver.IntentCaptcha, "captcha-generic", 0.7, "generic captcha challenge detected"},
	{"Are you a robot?", "", autosolver.IntentNormal, "", 0.6, "no challenge indicators detected"},
	{"Bot detection in progress", "", autosolver.IntentNormal, "", 0.6, "no challenge indicators detected"},
	{"Security challenge", "", autosolver.IntentCaptcha, "", 0.8, "captcha detected via semantic analysis"},
	{"Verify your identity", "", autosolver.IntentCaptcha, "", 0.8, "captcha detected via semantic analysis"},
	{"Home page", "https://example.com/challenge/step", autosolver.IntentCaptcha, "", 0.8, "captcha detected via semantic analysis"},
	{"Home page", "https://example.com/verify-me", autosolver.IntentCaptcha, "", 0.8, "captcha detected via semantic analysis"},
	{"Home page", "https://example.com/captcha", autosolver.IntentCaptcha, "captcha-generic", 0.7, "generic captcha challenge detected"},
	// A URL that merely contains a vendor's name, on a page titled "Home page",
	// used to be named as that vendor's challenge at 0.9. It is a word in a path;
	// /blog/how-recaptcha-works is the same shape. Naming a vendor is a claim
	// about the page, and the URL alone does not support one — these land on
	// generic at 0.7 now, which says captcha-ish without asserting which.
	{"Home page", "https://example.com/recaptcha", autosolver.IntentCaptcha, "captcha-generic", 0.7, "generic captcha challenge detected"},
	{"Home page", "https://example.com/hcaptcha", autosolver.IntentCaptcha, "captcha-generic", 0.7, "generic captcha challenge detected"},
	{"Log In", "", autosolver.IntentLogin, "", 0.7, "login page detected via semantic title analysis"},
	{"Login", "", autosolver.IntentLogin, "", 0.7, "login page detected via semantic title analysis"},
	{"Sign In", "", autosolver.IntentLogin, "", 0.7, "login page detected via semantic title analysis"},
	{"Signin", "", autosolver.IntentNormal, "", 0.6, "no challenge indicators detected"},
	{"Sign Up", "", autosolver.IntentSignup, "", 0.7, "signup page detected via semantic title analysis"},
	{"Signup", "", autosolver.IntentSignup, "", 0.7, "signup page detected via semantic title analysis"},
	{"Register", "", autosolver.IntentSignup, "", 0.7, "signup page detected via semantic title analysis"},
	{"Create Account", "", autosolver.IntentSignup, "", 0.7, "signup page detected via semantic title analysis"},
	{"Join our community", "", autosolver.IntentNormal, "", 0.6, "no challenge indicators detected"},
	{"Getting Started", "", autosolver.IntentOnboarding, "", 0.65, "onboarding flow detected via semantic title analysis"},
	{"Welcome", "", autosolver.IntentOnboarding, "", 0.65, "onboarding flow detected via semantic title analysis"},
	{"Onboarding", "", autosolver.IntentOnboarding, "", 0.65, "onboarding flow detected via semantic title analysis"},
	{"Complete your profile", "", autosolver.IntentOnboarding, "", 0.65, "onboarding flow detected via semantic title analysis"},
	{"Step 1 of 3", "", autosolver.IntentOnboarding, "", 0.65, "onboarding flow detected via semantic title analysis"},
	{"Continue", "", autosolver.IntentNavigation, "", 0.6, "navigation flow detected via semantic title analysis"},
	{"Next step", "", autosolver.IntentNavigation, "", 0.6, "navigation flow detected via semantic title analysis"},
	{"Choose option", "", autosolver.IntentNavigation, "", 0.6, "navigation flow detected via semantic title analysis"},
	{"Select plan", "", autosolver.IntentNavigation, "", 0.6, "navigation flow detected via semantic title analysis"},
	{"Setup wizard", "", autosolver.IntentNavigation, "", 0.6, "navigation flow detected via semantic title analysis"},
	{"Application form", "", autosolver.IntentForm, "", 0.6, "form flow detected via semantic title analysis"},
	{"Contact form", "", autosolver.IntentForm, "", 0.6, "form flow detected via semantic title analysis"},
	{"Checkout", "", autosolver.IntentForm, "", 0.6, "form flow detected via semantic title analysis"},
	{"Survey", "", autosolver.IntentForm, "", 0.6, "form flow detected via semantic title analysis"},
	{"Questionnaire", "", autosolver.IntentForm, "", 0.6, "form flow detected via semantic title analysis"},
	{"Access Denied", "", autosolver.IntentNormal, "", 0.6, "no challenge indicators detected"},
	{"Forbidden", "", autosolver.IntentNormal, "", 0.6, "no challenge indicators detected"},
	{"Blocked", "", autosolver.IntentNormal, "", 0.6, "no challenge indicators detected"},
	{"Unauthorized", "", autosolver.IntentNormal, "", 0.6, "no challenge indicators detected"},
	{"Home page", "", autosolver.IntentNormal, "", 0.6, "no challenge indicators detected"},

	{"Sign up or log in", "", autosolver.IntentLogin, "", 0.7, "login page detected via semantic title analysis"},
	{"Register or sign in", "", autosolver.IntentLogin, "", 0.7, "login page detected via semantic title analysis"},
	{"Welcome, register now", "", autosolver.IntentSignup, "", 0.7, "signup page detected via semantic title analysis"},
	{"Checkout, continue", "", autosolver.IntentNavigation, "", 0.6, "navigation flow detected via semantic title analysis"},
	{"Welcome, join our community", "", autosolver.IntentOnboarding, "", 0.65, "onboarding flow detected via semantic title analysis"},
}

func TestDetectIntent_Golden(t *testing.T) {
	for _, want := range detectIntentGolden {
		got := detect(t, want.title, want.url)
		if got.Type != want.intentType || got.ChallengeType != want.challengeType ||
			got.Confidence != want.confidence || got.Details != want.details {
			t.Fatalf("%q/%q: got %q/%q/%v/%q, want %q/%q/%v/%q", want.title, want.url,
				got.Type, got.ChallengeType, got.Confidence, got.Details,
				want.intentType, want.challengeType, want.confidence, want.details)
		}
	}
}
