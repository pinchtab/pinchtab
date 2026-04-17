// Package semantic provides the adapter between the autosolver system
// and the pinchtab/semantic package. This is the ONLY file in the
// autosolver module that imports github.com/pinchtab/semantic.
package semantic

import (
	"context"
	"fmt"
	"strings"

	"github.com/pinchtab/pinchtab/internal/autosolver"
	"github.com/pinchtab/semantic"
)

// Adapter wraps the pinchtab/semantic ElementMatcher to implement
// autosolver.SemanticEngine. It translates between the autosolver
// type system and the semantic package's types.
type Adapter struct {
	matcher semantic.ElementMatcher
}

// NewAdapter creates a semantic adapter from an existing ElementMatcher.
func NewAdapter(m semantic.ElementMatcher) *Adapter {
	return &Adapter{matcher: m}
}

// DetectIntent classifies the current page state by analyzing the page
// title and URL using semantic matching against known patterns.
func (a *Adapter) DetectIntent(ctx context.Context, page autosolver.Page) (*autosolver.Intent, error) {
	title := strings.ToLower(page.Title())
	url := page.URL()
	html, _ := page.HTML()
	if challenge := autosolver.DetectChallengeIntent(title, url, html); challenge != nil {
		return challenge, nil
	}

	// Cloudflare challenge indicators
	cfIndicators := []string{"just a moment", "attention required", "checking your browser"}
	for _, indicator := range cfIndicators {
		if strings.Contains(title, indicator) {
			return &autosolver.Intent{
				Type:          autosolver.IntentCaptcha,
				Confidence:    0.95,
				ChallengeType: "cloudflare-turnstile",
				Details:       "cloudflare challenge detected via semantic title analysis",
			}, nil
		}
	}

	// CAPTCHA indicators (via URL or title patterns)
	captchaIndicators := []string{"captcha", "challenge", "verify", "recaptcha", "hcaptcha"}
	for _, indicator := range captchaIndicators {
		if strings.Contains(title, indicator) || strings.Contains(strings.ToLower(url), indicator) {
			return &autosolver.Intent{
				Type:       autosolver.IntentCaptcha,
				Confidence: 0.8,
				Details:    "captcha detected via semantic analysis",
			}, nil
		}
	}

	// Login page detection via semantic matching if matcher is available.
	if a.matcher != nil {
		intent, err := a.detectViaSemanticMatch(ctx, page)
		if err == nil && intent != nil {
			return intent, nil
		}
	}

	// Fallback to title-based heuristics for login/signup.
	loginPatterns := []string{"log in", "login", "sign in"}
	for _, p := range loginPatterns {
		if strings.Contains(title, p) {
			return &autosolver.Intent{
				Type:       autosolver.IntentLogin,
				Confidence: 0.7,
				Details:    "login page detected via semantic title analysis",
			}, nil
		}
	}

	signupPatterns := []string{"sign up", "signup", "register", "create account"}
	for _, p := range signupPatterns {
		if strings.Contains(title, p) {
			return &autosolver.Intent{
				Type:       autosolver.IntentSignup,
				Confidence: 0.7,
				Details:    "signup page detected via semantic title analysis",
			}, nil
		}
	}

	onboardingPatterns := []string{"getting started", "welcome", "onboarding", "complete your profile", "step 1"}
	for _, p := range onboardingPatterns {
		if strings.Contains(title, p) {
			return &autosolver.Intent{
				Type:       autosolver.IntentOnboarding,
				Confidence: 0.65,
				Details:    "onboarding flow detected via semantic title analysis",
			}, nil
		}
	}

	navigationPatterns := []string{"continue", "next step", "choose option", "select plan", "wizard"}
	for _, p := range navigationPatterns {
		if strings.Contains(title, p) {
			return &autosolver.Intent{
				Type:       autosolver.IntentNavigation,
				Confidence: 0.6,
				Details:    "navigation flow detected via semantic title analysis",
			}, nil
		}
	}

	formPatterns := []string{"application form", "contact form", "checkout", "survey", "questionnaire"}
	for _, p := range formPatterns {
		if strings.Contains(title, p) {
			return &autosolver.Intent{
				Type:       autosolver.IntentForm,
				Confidence: 0.6,
				Details:    "form flow detected via semantic title analysis",
			}, nil
		}
	}

	return &autosolver.Intent{
		Type:       autosolver.IntentNormal,
		Confidence: 0.6,
		Details:    "no challenge indicators detected",
	}, nil
}

// FindElement uses the semantic matcher to locate a UI element by
// natural-language description.
func (a *Adapter) FindElement(ctx context.Context, page autosolver.Page, query string) (*autosolver.ElementMatch, error) {
	if a.matcher == nil {
		return nil, nil
	}

	// Get page HTML and extract element descriptors.
	// For now we use a simplified approach — the full integration
	// would use the accessibility snapshot like handlers/find.go does.
	html, err := page.HTML()
	if err != nil {
		return nil, err
	}

	// Build minimal element descriptors from interactive element patterns.
	descs := extractDescriptors(html)
	if len(descs) == 0 {
		return nil, nil
	}

	result, err := a.matcher.Find(ctx, query, descs, semantic.FindOptions{
		Threshold: 0.3,
		TopK:      1,
	})
	if err != nil {
		return nil, err
	}

	if result.BestRef == "" || len(result.Matches) == 0 {
		return nil, nil
	}

	best := result.Matches[0]
	// Semantic refs are selector-compatible in this adapter, so keep both
	// fields aligned for executors that prefer CSS selectors.
	return &autosolver.ElementMatch{
		Ref:        best.Ref,
		Selector:   best.Ref,
		Role:       best.Role,
		Name:       best.Name,
		Score:      best.Score,
		Confidence: result.ConfidenceLabel(),
	}, nil
}

// SuggestAction determines the next action based on the detected intent
// and available page elements.
func (a *Adapter) SuggestAction(ctx context.Context, page autosolver.Page, intent *autosolver.Intent) (*autosolver.SuggestedAction, error) {
	if intent == nil {
		return &autosolver.SuggestedAction{
			Action: autosolver.ActionNone,
			Reason: "no intent detected",
		}, nil
	}

	switch intent.Type {
	case autosolver.IntentCaptcha:
		return a.suggestCaptchaAction(ctx, page, intent)
	case autosolver.IntentLogin:
		return a.suggestLoginAction(ctx, page)
	case autosolver.IntentSignup:
		return a.suggestSignupAction(ctx, page)
	case autosolver.IntentOnboarding:
		return a.suggestOnboardingAction(ctx, page)
	case autosolver.IntentNavigation:
		return a.suggestNavigationAction(ctx, page)
	case autosolver.IntentForm:
		return a.suggestFormAction(ctx, page)
	case autosolver.IntentBlocked:
		return &autosolver.SuggestedAction{
			Action: autosolver.ActionWait,
			Reason: "page is blocked; waiting for challenge resolution",
		}, nil
	default:
		return &autosolver.SuggestedAction{
			Action: autosolver.ActionNone,
			Reason: "no action needed for current page state",
		}, nil
	}
}

// suggestCaptchaAction finds the captcha widget and suggests clicking it.
func (a *Adapter) suggestCaptchaAction(ctx context.Context, page autosolver.Page, intent *autosolver.Intent) (*autosolver.SuggestedAction, error) {
	match, err := a.FindElement(ctx, page, "captcha checkbox verify button")
	if err != nil || match == nil {
		return &autosolver.SuggestedAction{
			Action: autosolver.ActionWait,
			Reason: "captcha element not found; waiting for it to appear",
		}, nil
	}

	return &autosolver.SuggestedAction{
		Action:   autosolver.ActionClick,
		Selector: match.Selector,
		X:        match.X,
		Y:        match.Y,
		Reason:   "clicking captcha checkbox",
	}, nil
}

// suggestLoginAction finds the first empty input field and suggests focusing it.
func (a *Adapter) suggestLoginAction(ctx context.Context, page autosolver.Page) (*autosolver.SuggestedAction, error) {
	match, err := a.FindElement(ctx, page, "username email input field")
	if err != nil || match == nil {
		return &autosolver.SuggestedAction{
			Action: autosolver.ActionNone,
			Reason: "login form elements not found",
		}, nil
	}

	return &autosolver.SuggestedAction{
		Action:   autosolver.ActionClick,
		Selector: match.Selector,
		Reason:   "focusing username/email input field",
	}, nil
}

// suggestSignupAction finds the first registration field.
func (a *Adapter) suggestSignupAction(ctx context.Context, page autosolver.Page) (*autosolver.SuggestedAction, error) {
	match, err := a.FindElement(ctx, page, "name email registration input field")
	if err != nil || match == nil {
		return &autosolver.SuggestedAction{
			Action: autosolver.ActionNone,
			Reason: "signup form elements not found",
		}, nil
	}

	return &autosolver.SuggestedAction{
		Action:   autosolver.ActionClick,
		Selector: match.Selector,
		Reason:   "focusing first registration input field",
	}, nil
}

func (a *Adapter) suggestOnboardingAction(ctx context.Context, page autosolver.Page) (*autosolver.SuggestedAction, error) {
	match, err := a.FindElement(ctx, page, "next continue skip done button")
	if err != nil || match == nil {
		return &autosolver.SuggestedAction{
			Action: autosolver.ActionWait,
			Reason: "onboarding controls not found yet",
		}, nil
	}

	return &autosolver.SuggestedAction{
		Action:   autosolver.ActionClick,
		Selector: match.Selector,
		Reason:   "advancing onboarding flow",
	}, nil
}

func (a *Adapter) suggestNavigationAction(ctx context.Context, page autosolver.Page) (*autosolver.SuggestedAction, error) {
	match, err := a.FindElement(ctx, page, "primary navigation button next continue link")
	if err != nil || match == nil {
		return &autosolver.SuggestedAction{
			Action: autosolver.ActionNone,
			Reason: "navigation controls not found",
		}, nil
	}

	return &autosolver.SuggestedAction{
		Action:   autosolver.ActionClick,
		Selector: match.Selector,
		Reason:   "advancing navigation flow",
	}, nil
}

func (a *Adapter) suggestFormAction(ctx context.Context, page autosolver.Page) (*autosolver.SuggestedAction, error) {
	match, err := a.FindElement(ctx, page, "required input field form control")
	if err != nil || match == nil {
		return &autosolver.SuggestedAction{
			Action: autosolver.ActionNone,
			Reason: "form fields not found",
		}, nil
	}

	return &autosolver.SuggestedAction{
		Action:   autosolver.ActionClick,
		Selector: match.Selector,
		Reason:   "focusing a required form field",
	}, nil
}

// detectViaSemanticMatch uses the semantic matcher to classify page
// elements and infer the page intent.
func (a *Adapter) detectViaSemanticMatch(ctx context.Context, page autosolver.Page) (*autosolver.Intent, error) {
	// Search for login-related elements
	match, err := a.FindElement(ctx, page, "login submit button")
	if err == nil && match != nil && match.Score > 0.6 {
		return &autosolver.Intent{
			Type:       autosolver.IntentLogin,
			Confidence: match.Score,
			Details:    "login form detected via semantic element matching",
		}, nil
	}

	// Search for signup-related elements
	match, err = a.FindElement(ctx, page, "register create account button")
	if err == nil && match != nil && match.Score > 0.6 {
		return &autosolver.Intent{
			Type:       autosolver.IntentSignup,
			Confidence: match.Score,
			Details:    "signup form detected via semantic element matching",
		}, nil
	}

	match, err = a.FindElement(ctx, page, "next continue done button")
	if err == nil && match != nil && match.Score > 0.6 {
		return &autosolver.Intent{
			Type:       autosolver.IntentOnboarding,
			Confidence: match.Score,
			Details:    "onboarding flow detected via semantic element matching",
		}, nil
	}

	match, err = a.FindElement(ctx, page, "primary navigation button link")
	if err == nil && match != nil && match.Score > 0.6 {
		return &autosolver.Intent{
			Type:       autosolver.IntentNavigation,
			Confidence: match.Score,
			Details:    "navigation flow detected via semantic element matching",
		}, nil
	}

	match, err = a.FindElement(ctx, page, "required input field submit button")
	if err == nil && match != nil && match.Score > 0.6 {
		return &autosolver.Intent{
			Type:       autosolver.IntentForm,
			Confidence: match.Score,
			Details:    "form flow detected via semantic element matching",
		}, nil
	}

	return nil, nil
}

// extractDescriptors parses HTML to build minimal element descriptors
// for semantic matching. This is a simplified version — the production
// integration uses the full accessibility snapshot.
func extractDescriptors(html string) []semantic.ElementDescriptor {
	var descs []semantic.ElementDescriptor

	// Extract interactive elements by scanning for common patterns.
	// A full implementation would use the a11y tree from the bridge.
	patterns := []struct {
		searchTag string
		selTag    string
		role      string
	}{
		{"<input", "input", "textbox"},
		{"<button", "button", "button"},
		{"<a ", "a", "link"},
		{"<select", "select", "combobox"},
		{"<textarea", "textarea", "textbox"},
	}

	lower := strings.ToLower(html)
	ref := 0
	for _, p := range patterns {
		idx := 0
		for {
			pos := strings.Index(lower[idx:], p.searchTag)
			if pos == -1 {
				break
			}
			idx += pos + len(p.searchTag)

			// Extract a rough name from surrounding text.
			end := strings.Index(lower[idx:], ">")
			if end == -1 {
				break
			}
			snippet := html[idx : idx+end]
			selector := buildSelector(p.selTag, snippet)

			name := extractAttr(snippet, "placeholder")
			if name == "" {
				name = extractAttr(snippet, "aria-label")
			}
			if name == "" {
				name = extractAttr(snippet, "name")
			}
			if name == "" {
				name = selector
			}

			ref++
			descs = append(descs, semantic.ElementDescriptor{
				Ref:  selector,
				Role: p.role,
				Name: name,
			})

			if ref > 50 {
				return descs
			}
		}
	}
	return descs
}

func buildSelector(tag, snippet string) string {
	id := extractAttr(snippet, "id")
	if id != "" {
		return fmt.Sprintf("#%s", id)
	}

	name := extractAttr(snippet, "name")
	if name != "" {
		return fmt.Sprintf(`%s[name="%s"]`, tag, escapeAttrValue(name))
	}

	placeholder := extractAttr(snippet, "placeholder")
	if placeholder != "" {
		return fmt.Sprintf(`%s[placeholder="%s"]`, tag, escapeAttrValue(placeholder))
	}

	aria := extractAttr(snippet, "aria-label")
	if aria != "" {
		return fmt.Sprintf(`%s[aria-label="%s"]`, tag, escapeAttrValue(aria))
	}

	return tag
}

func escapeAttrValue(v string) string {
	v = strings.ReplaceAll(v, `\\`, `\\\\`)
	v = strings.ReplaceAll(v, `"`, `\\"`)
	return v
}

// extractAttr extracts an HTML attribute value from a tag snippet.
func extractAttr(snippet, attr string) string {
	lower := strings.ToLower(snippet)
	idx := strings.Index(lower, attr+"=")
	if idx == -1 {
		return ""
	}
	rest := snippet[idx+len(attr)+1:]
	if len(rest) == 0 {
		return ""
	}

	quote := rest[0]
	if quote != '"' && quote != '\'' {
		// Unquoted attribute — take until space or end.
		end := strings.IndexByte(rest, ' ')
		if end == -1 {
			return rest
		}
		return rest[:end]
	}

	end := strings.IndexByte(rest[1:], quote)
	if end == -1 {
		return ""
	}
	return rest[1 : 1+end]
}
