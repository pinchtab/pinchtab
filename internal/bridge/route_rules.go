package bridge

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/pinchtab/pinchtab/internal/security"
)

type RouteAction string

const (
	RouteActionContinue RouteAction = "continue"
	RouteActionAbort    RouteAction = "abort"
	RouteActionFulfill  RouteAction = "fulfill"
)

// allowedFulfillContentTypes is the explicit safe-list of MIME types that may
// be used as the Content-Type of a fulfilled (mocked) response.
//
// Inclusion criterion: browsers do not execute scripts in this MIME. That is
// the security property fulfill needs — the response body runs in the
// response origin's security context, so anything the browser would parse
// and execute (HTML, JS, XHTML, SVG with embedded <script>, XSLT) becomes a
// scripted-injection vector in that origin.
//
// Implicitly denied (the absences are intentional, not an oversight):
//   - text/html, application/xhtml+xml — render and run scripts.
//   - text/javascript, application/javascript, application/x-javascript,
//     text/ecmascript, application/ecmascript — script files.
//   - image/svg+xml — SVG can carry inline <script>.
//   - text/xsl, application/xslt+xml — transform XML into HTML+JS at parse
//     time (note: application/xml itself is allowed because it does not
//     trigger XSLT processing on its own).
//   - application/x-shockwave-flash, application/x-msdownload — historic
//     auto-execute risk.
//
// Expanding this list is a deliberate security decision; do it with review,
// not as a casual addition.
var allowedFulfillContentTypes = map[string]struct{}{
	"application/json":         {},
	"application/xml":          {},
	"application/pdf":          {},
	"application/octet-stream": {},
	"text/plain":               {},
	"text/csv":                 {},
	"text/xml":                 {},
	"image/png":                {},
	"image/jpeg":               {},
	"image/gif":                {},
	"image/webp":               {},
	"video/mp4":                {},
	"video/webm":               {},
	"audio/mpeg":               {},
	"audio/ogg":                {},
	"audio/wav":                {},
}

// validResourceTypes is the CDP Network.ResourceType enum lowercased. Match
// is case-insensitive (see ruleMatches), so callers can pass "Script", "xhr",
// etc., but the rule must name a known category.
var validResourceTypes = map[string]struct{}{
	"document":           {},
	"stylesheet":         {},
	"image":              {},
	"media":              {},
	"font":               {},
	"script":             {},
	"texttrack":          {},
	"xhr":                {},
	"fetch":              {},
	"prefetch":           {},
	"eventsource":        {},
	"websocket":          {},
	"manifest":           {},
	"signedexchange":     {},
	"ping":               {},
	"cspviolationreport": {},
	"preflight":          {},
	"other":              {},
}

// IsFulfillContentTypeAllowed reports whether ct is on the safe-list and free
// of header-injection control bytes. Empty ct returns true — AddRule defaults
// it to application/json before storing.
func IsFulfillContentTypeAllowed(ct string) bool {
	if ct == "" {
		return true
	}
	if containsHeaderControlChar(ct) {
		return false
	}
	base := strings.TrimSpace(strings.ToLower(strings.SplitN(ct, ";", 2)[0]))
	_, ok := allowedFulfillContentTypes[base]
	return ok
}

// IsResourceTypeValid reports whether rt names a known CDP resource category.
// Empty rt is valid (means "no resource-type filter").
func IsResourceTypeValid(rt string) bool {
	if rt == "" {
		return true
	}
	_, ok := validResourceTypes[strings.ToLower(rt)]
	return ok
}

// normalizeHTTPMethod returns the upper-case form of m (trimmed) when it is a
// recognised HTTP method, and ok=false otherwise. Only methods the browser
// actually emits are accepted; custom verbs would never match a browser
// request anyway, so there's no point letting them through.
func normalizeHTTPMethod(m string) (string, bool) {
	upper := strings.ToUpper(strings.TrimSpace(m))
	switch upper {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS", "CONNECT", "TRACE":
		return upper, true
	}
	return "", false
}

// containsHeaderControlChar reports whether s contains any byte that would
// allow header injection if the string were spliced into an HTTP header value
// (CR, LF, NUL).
func containsHeaderControlChar(s string) bool {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\r', '\n', 0:
			return true
		}
	}
	return false
}

// RouteRule describes a single interception rule for a tab.
//
// Pattern matching semantics (deliberately asymmetric, mirrors the user-facing
// CLI ergonomics):
//
//   - No wildcards in Pattern → SUBSTRING match against the request URL.
//     "tracker.io" matches "https://tracker.io/x" and "https://x?ref=tracker.io".
//
//   - Pattern contains '*' or '?' → ANCHORED full-URL glob match.
//     "*.png" compiles to "^.*\.png$" and matches URLs ending in .png.
//     "*api*" matches any URL containing "api". To get substring-style
//     matching with a wildcard, wrap the pattern in '*'s on both sides.
//
// The asymmetry is intentional: bare strings are the common
// "block-this-domain" case where substring is what users mean; wildcards are
// the structural case (extension, host shape) where anchored full-URL is
// what they mean. ResourceType, when set, narrows matches to a specific CDP
// resource category (e.g. "script", "xhr", "image").
type RouteRule struct {
	Pattern      string      `json:"pattern"`
	Action       RouteAction `json:"action"`
	Body         string      `json:"body,omitempty"`
	ContentType  string      `json:"contentType,omitempty"`
	Status       int         `json:"status,omitempty"`
	ResourceType string      `json:"resourceType,omitempty"`

	// Method, when non-empty, narrows matches to the given HTTP method
	// (case-insensitive). Empty Method matches any method, with one exception:
	// fulfill rules skip OPTIONS preflights by default — fulfilling preflights
	// can break or bypass CORS, so the operator must explicitly set
	// Method="OPTIONS" to opt in.
	Method string `json:"method,omitempty"`

	// compiled is the precompiled regex for wildcard patterns. It is nil for
	// substring patterns (Pattern has no '*' or '?'). Set once at AddRule time
	// and read-only thereafter so the listener doesn't recompile per event.
	// Unexported so encoding/json skips it on the way out of /network/route.
	compiled *regexp.Regexp
}

// forbiddenFulfillSchemes lists URL schemes where fulfill must always be
// rejected regardless of the host allowlist. data:/javascript: are inline
// pseudo-URLs that don't traverse the network so a fulfill is meaningless;
// file:/blob:/ftp: bypass the web origin model in surprising ways; chrome:
// and chrome-extension: are privileged browser origins where forging a
// response is dangerous; about: should never be intercepted at all (about:
// blank in particular is special-cased earlier in the stack).
var forbiddenFulfillSchemes = map[string]struct{}{
	"javascript":       {},
	"data":             {},
	"file":             {},
	"blob":             {},
	"ftp":              {},
	"chrome":           {},
	"chrome-extension": {},
	"about":            {},
}

// fulfillForgeryPermittedFor reports whether forging a response (action
// fulfill) is permitted for rawURL. The name is deliberate: this is a
// permission check on RESPONSE FORGERY, not a generic "is this URL allowed"
// query, and it inverts the host-allowlist on purpose.
//
// Policy:
//
//  1. Special-scheme URLs (data:, javascript:, file:, blob:, ftp:, chrome:,
//     chrome-extension:, about:) are ALWAYS denied — fulfilling those either
//     makes no sense or escapes the web origin model in surprising ways.
//  2. With no allowlist configured (nil accessor or empty list) the policy
//     is inactive and forgery is permitted on all normal-scheme URLs.
//  3. Otherwise: forgery is BLOCKED on hosts listed in security.allowedDomains
//     and PERMITTED on all other hosts. Rationale: allowedDomains marks the
//     sensitive surfaces the operator has authorized the agent to interact
//     with (banking, email, internal SaaS) — exactly the origins where
//     injecting attacker-controlled responses is most damaging, because the
//     body executes in that origin's security context with access to its
//     cookies, localStorage, and DOM. Unlisted hosts are the typical mocking
//     targets (third-party APIs, CDNs, analytics endpoints) where response
//     forgery is a normal test/dev tool.
//
// Yes, this is intentionally the opposite of "is the host allowlisted." If
// you find yourself reading `!security.HostAllowed(...)` and reaching for the
// `!`, that is the policy.
func (rm *RouteManager) fulfillForgeryPermittedFor(rawURL string) bool {
	if isForbiddenFulfillScheme(rawURL) {
		return false
	}
	if rm.allowedDomainsFn == nil {
		return true
	}
	allow := rm.allowedDomainsFn()
	if len(allow) == 0 {
		return true
	}
	return !security.HostAllowed(rawURL, allow)
}

// isForbiddenFulfillScheme reports whether rawURL uses a scheme where
// fulfill must always be rejected (see forbiddenFulfillSchemes for the set
// and rationale). Unparseable URLs are treated as forbidden.
func isForbiddenFulfillScheme(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return true
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if scheme == "" {
		return false
	}
	_, forbidden := forbiddenFulfillSchemes[scheme]
	return forbidden
}

// ruleMatchesURL reports whether r matches url, using the precompiled regex
// when present (wildcard patterns) and a substring check otherwise. AddRule
// always populates r.compiled for wildcard patterns; the on-the-fly fallback
// here keeps the function correct when state is populated by other paths
// (e.g. unit tests) and is never hit in production.
func ruleMatchesURL(r RouteRule, url string) bool {
	if r.Pattern == "" {
		return false
	}
	if r.compiled != nil {
		return r.compiled.MatchString(url)
	}
	if strings.ContainsAny(r.Pattern, "*?") {
		return globMatch(r.Pattern, url)
	}
	return strings.Contains(url, r.Pattern)
}

// compileRulePattern returns the precompiled regex for wildcard patterns or
// nil for substring patterns. Errors only when a wildcard pattern fails to
// compile (escaped char issues).
func compileRulePattern(pattern string) (*regexp.Regexp, error) {
	if pattern == "" || !strings.ContainsAny(pattern, "*?") {
		return nil, nil
	}
	return globToRegex(pattern)
}

// globMatch is a convenience used by tests; production code uses
// ruleMatchesURL with the precompiled regex stored on the rule.
func globMatch(pattern, url string) bool {
	if pattern == "" {
		return false
	}
	if !strings.ContainsAny(pattern, "*?") {
		return strings.Contains(url, pattern)
	}
	re, err := globToRegex(pattern)
	if err != nil {
		return false
	}
	return re.MatchString(url)
}

func globToRegex(pattern string) (*regexp.Regexp, error) {
	var sb strings.Builder
	sb.WriteString("^")
	for _, r := range pattern {
		switch r {
		case '*':
			sb.WriteString(".*")
		case '?':
			sb.WriteString(".")
		case '.', '+', '(', ')', '[', ']', '{', '}', '|', '^', '$', '\\':
			sb.WriteRune('\\')
			sb.WriteRune(r)
		default:
			sb.WriteRune(r)
		}
	}
	sb.WriteString("$")
	return regexp.Compile(sb.String())
}
