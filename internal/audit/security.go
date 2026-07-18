// Security rules (no external scanner; all computed from data the audit
// already collects, deterministically and offline):
//
//   - mixed-content (high): an https page loading http:// subresources,
//     from the observed network requests.
//   - insecure-form-action (medium): an https page with a form whose action
//     posts to plain http://.
//   - insecure-password-form (high): a form containing a password input
//     whose action is plain http://, regardless of the page's own scheme.
//   - exposed-endpoint (medium): a successful (non-4xx/5xx) request to a
//     sensitive path (.env, .git, .htaccess, .htpasswd, wp-config.php,
//     id_rsa) observed in the network log.
//   - directory-listing (low): a page title that is a server directory
//     index ("Index of /...").
package audit

import (
	"fmt"
	"net/url"
	"strings"
)

// FormFact is the DOM shape of one form, gathered by FormFactsScript.
type FormFact struct {
	// Action is the form's resolved action URL.
	Action string `json:"action"`
	// Method is the lowercased form method.
	Method string `json:"method"`
	// HasPassword reports whether the form contains a password input.
	HasPassword bool `json:"hasPassword"`
}

// FormFactsScript evaluates to the []FormFact JSON shape in the page.
const FormFactsScript = `(() => Array.from(document.forms).map((f) => ({
  action: f.action || '',
  method: (f.method || 'get').toLowerCase(),
  hasPassword: !!f.querySelector('input[type="password"]'),
})))()`

// exposedPathMarkers are sensitive path fragments that should never answer
// successfully on a production site.
var exposedPathMarkers = []string{".env", ".git", ".htaccess", ".htpasswd", "wp-config.php", "id_rsa"}

func isHTTPS(raw string) bool {
	return strings.HasPrefix(strings.ToLower(raw), "https://")
}

func isHTTP(raw string) bool {
	return strings.HasPrefix(strings.ToLower(raw), "http://")
}

// EvaluateSecurity derives the rule-based security findings for one page
// from its URL, title, observed network requests, and form facts. Output
// order is deterministic: rules in documentation order, items in input
// order.
func EvaluateSecurity(pageURL, title string, requests []NetworkRequest, forms []FormFact) []SecurityFinding {
	var findings []SecurityFinding
	pageSecure := isHTTPS(pageURL)

	if pageSecure {
		for _, req := range requests {
			if req.URL == pageURL || !isHTTP(req.URL) {
				continue
			}
			findings = append(findings, SecurityFinding{
				RuleID:   "mixed-content",
				Severity: "high",
				Detail:   fmt.Sprintf("https page loads insecure subresource %s", req.URL),
				URL:      pageURL,
			})
		}
	}

	for _, form := range forms {
		if pageSecure && isHTTP(form.Action) && !form.HasPassword {
			findings = append(findings, SecurityFinding{
				RuleID:   "insecure-form-action",
				Severity: "medium",
				Detail:   fmt.Sprintf("form on https page submits to insecure %s", form.Action),
				URL:      pageURL,
			})
		}
		if form.HasPassword && isHTTP(form.Action) {
			findings = append(findings, SecurityFinding{
				RuleID:   "insecure-password-form",
				Severity: "high",
				Detail:   fmt.Sprintf("password form submits over plain http to %s", form.Action),
				URL:      pageURL,
			})
		}
	}

	for _, req := range requests {
		if req.Status >= 400 || req.Status == 0 {
			continue
		}
		if marker := exposedPathMarker(req.URL); marker != "" {
			findings = append(findings, SecurityFinding{
				RuleID:   "exposed-endpoint",
				Severity: "medium",
				Detail:   fmt.Sprintf("sensitive path %q answered %d at %s", marker, req.Status, req.URL),
				URL:      pageURL,
			})
		}
	}

	if strings.HasPrefix(strings.TrimSpace(title), "Index of /") {
		findings = append(findings, SecurityFinding{
			RuleID:   "directory-listing",
			Severity: "low",
			Detail:   fmt.Sprintf("server directory listing exposed (%q)", strings.TrimSpace(title)),
			URL:      pageURL,
		})
	}

	return findings
}

// exposedPathMarker returns the matched sensitive fragment in the URL path,
// or "" when none matches. Only the path is inspected, not the query.
func exposedPathMarker(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	path := strings.ToLower(u.Path)
	for _, marker := range exposedPathMarkers {
		if strings.Contains(path, marker) {
			return marker
		}
	}
	return ""
}
