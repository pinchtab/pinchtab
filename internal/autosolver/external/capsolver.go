// Package external provides skeleton implementations for third-party
// CAPTCHA solving services (Capsolver, 2Captcha). These are designed
// as pluggable solvers enabled via configuration.
package external

import (
	"context"
	"fmt"
	"strings"

	"github.com/pinchtab/pinchtab/internal/autosolver"
)

// CapsolverConfig holds Capsolver API configuration.
type CapsolverConfig struct {
	APIKey  string `json:"apiKey"`
	BaseURL string `json:"baseUrl,omitempty"` // Default: https://api.capsolver.com
}

// Capsolver implements autosolver.Solver using the Capsolver API.
// It supports reCAPTCHA v2/v3, hCaptcha, and Cloudflare Turnstile.
type Capsolver struct {
	config CapsolverConfig
}

// NewCapsolver creates a Capsolver solver with the given configuration.
func NewCapsolver(cfg CapsolverConfig) *Capsolver {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.capsolver.com"
	}
	return &Capsolver{config: cfg}
}

func (c *Capsolver) Name() string  { return "capsolver" }
func (c *Capsolver) Priority() int { return 200 }

// CanHandle checks if the page contains a supported CAPTCHA type.
func (c *Capsolver) CanHandle(ctx context.Context, page autosolver.Page) (bool, error) {
	if c.config.APIKey == "" {
		return false, nil
	}

	html, err := page.HTML()
	if err != nil {
		return false, nil
	}

	lower := strings.ToLower(html)
	captchaPatterns := []string{
		"recaptcha",
		"hcaptcha",
		"challenges.cloudflare.com/turnstile",
		"g-recaptcha",
		"h-captcha",
	}
	for _, p := range captchaPatterns {
		if strings.Contains(lower, p) {
			return true, nil
		}
	}
	return false, nil
}

// Solve submits the CAPTCHA to the Capsolver API and injects the result.
//
// This is a skeleton implementation. The actual HTTP client logic
// (create task → poll result → inject token) must be filled in
// with the Capsolver API v1 protocol.
func (c *Capsolver) Solve(ctx context.Context, page autosolver.Page, executor autosolver.ActionExecutor) (*autosolver.Result, error) {
	result := &autosolver.Result{SolverUsed: "capsolver"}

	if c.config.APIKey == "" {
		result.Error = "capsolver API key not configured"
		return result, fmt.Errorf("capsolver API key not configured")
	}

	html, err := page.HTML()
	if err != nil {
		result.Error = fmt.Sprintf("get HTML: %v", err)
		return result, err
	}

	captchaType := detectCaptchaType(html)
	if captchaType == "" {
		result.Error = "no supported CAPTCHA detected"
		return result, fmt.Errorf("no supported CAPTCHA detected on page")
	}

	sitekey := extractSitekey(html, captchaType)
	if sitekey == "" {
		result.Error = "sitekey not found"
		return result, fmt.Errorf("could not extract sitekey from page")
	}

	// TODO: Implement HTTP client for Capsolver API v1.
	// POST /createTask with task type + sitekey + page URL.
	_ = sitekey
	_ = page.URL()

	result.Error = "capsolver API client not yet implemented"
	return result, fmt.Errorf("capsolver: API client not yet implemented — skeleton only")
}

func detectCaptchaType(html string) string {
	lower := strings.ToLower(html)
	switch {
	case strings.Contains(lower, "g-recaptcha") || strings.Contains(lower, "recaptcha"):
		return "recaptcha"
	case strings.Contains(lower, "h-captcha") || strings.Contains(lower, "hcaptcha"):
		return "hcaptcha"
	case strings.Contains(lower, "challenges.cloudflare.com/turnstile"):
		return "turnstile"
	default:
		return ""
	}
}

func extractSitekey(html, captchaType string) string {
	var attrNames []string
	switch captchaType {
	case "recaptcha":
		attrNames = []string{"data-sitekey"}
	case "hcaptcha":
		attrNames = []string{"data-sitekey"}
	case "turnstile":
		attrNames = []string{"data-sitekey"}
	default:
		return ""
	}

	for _, attr := range attrNames {
		idx := strings.Index(html, attr+`="`)
		if idx == -1 {
			idx = strings.Index(html, attr+`='`)
		}
		if idx == -1 {
			continue
		}
		start := idx + len(attr) + 2
		quote := html[start-1]
		end := strings.IndexByte(html[start:], quote)
		if end == -1 {
			continue
		}
		return html[start : start+end]
	}
	return ""
}
