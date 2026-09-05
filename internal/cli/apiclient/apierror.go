package apiclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/pinchtab/pinchtab/internal/remedy"
)

// mustRequest is doRequest with the common fatal-on-transport-error policy.
func mustRequest(client *http.Client, token string, r request) (int, []byte) {
	status, body, err := doRequest(client, token, r)
	if err != nil {
		fatal("Request failed: %v", err)
	}
	return status, body
}

// exitOnAPIError is a terminal path: the command is ending, so this is where the
// failure is HANDLED rather than formatted, and where the cached tab that produced
// a dead id is dropped.
func exitOnAPIError(r request, status int, body []byte) {
	if status >= 400 {
		fmt.Fprint(os.Stderr, renderAPIError(r, status, body)+clearCachedTabOnFailure(status, body))
		os.Exit(1)
	}
}

// renderAPIError formats an HTTP error for the terminal. A route-level 404
// (the mux's plain-text "404 page not found", as opposed to a JSON
// application error) means the running instance predates the requested
// endpoint — say so explicitly instead of a bare 404 that reads as if the
// user's target site failed.
func renderAPIError(r request, statusCode int, body []byte) string {
	if isRouteNotFound(statusCode, body) {
		return routeNotFoundMessage(r.method, r.url)
	}
	return renderAPIErrorBody(statusCode, body)
}

// isRouteNotFound reports whether a 404 came from the HTTP mux (no such
// route) rather than an application handler, which always answers in JSON.
func isRouteNotFound(statusCode int, body []byte) bool {
	if statusCode != http.StatusNotFound {
		return false
	}
	var probe any
	return json.Unmarshal(body, &probe) != nil
}

func routeNotFoundMessage(method, rawURL string) string {
	addr, path := rawURL, ""
	if u, err := url.Parse(rawURL); err == nil && u.Host != "" {
		addr, path = u.Host, u.Path
	}
	return fmt.Sprintf("Error: the running pinchtab instance at %s does not support %s %s (it is likely an older version). Restart it with the current binary and retry.\n",
		addr, method, path)
}

// ExitWithAPIError is the other terminal path. See exitOnAPIError.
func ExitWithAPIError(statusCode int, body []byte) {
	fmt.Fprint(os.Stderr, renderAPIErrorBody(statusCode, body)+clearCachedTabOnFailure(statusCode, body))
	os.Exit(1)
}

// CachedTab is what the CLI knows before it issues a request and the server does
// not: this tab id came from the CLI's own cache rather than from the user, which
// server that cache belongs to, and which file holds it.
//
// It is data, not an installed callback. The remedy it feeds used to arrive as a
// func the command layer assigned to a package variable, and that func read the
// config and deleted the state file — so building an error string performed
// filesystem I/O on every render, including the returning paths a poll loop drives
// ten times a second. As data, rendering it cannot do either, by construction.
//
// The zero value asks for nothing: a binary that never sets it renders exactly as
// before, so behaviour no longer depends on which binary linked this package.
type CachedTab struct {
	TabID     string // the id the CLI substituted for one the user did not type
	Base      string // the server the cache belongs to, for the message
	StateFile string // the file the terminal path removes
}

var cachedTab CachedTab

// UseCachedTab records the cached tab the following requests target. The CLI calls
// it at the moment it substitutes its cached id, which is where both facts are
// already resolved — not while an error is being formatted.
func UseCachedTab(c CachedTab) { cachedTab = c }

// namesCachedTab is the one classifier both consumers share: the formatter for the
// wording, the terminal path for the clearing. Pure.
func namesCachedTab(statusCode int, body []byte) bool {
	return statusCode == http.StatusNotFound &&
		cachedTab.TabID != "" &&
		bytes.Contains(body, []byte(cachedTab.TabID))
}

// A fresh tab is the one command this CLI-side advice can name. The prose that used to
// carry it — where the id came from, and that a retry is enough — is a hint, because this
// line prints into the same slot a server remedy does and a caller cannot tell them apart:
// see internal/remedy for what that slot promises.
var openFreshTab = remedy.Declare("pinchtab nav <url>")

// staleTabAdvice says where the id came from, and claims nothing about the cache's
// current state — it is emitted on returning paths too, where nothing is cleared.
// A retry is still the right advice there: the cached id is re-probed at the start
// of every command, so the next one drops it.
func staleTabAdvice(statusCode int, body []byte) (string, remedy.Remedy) {
	if !namesCachedTab(statusCode, body) {
		return "", remedy.None
	}
	return fmt.Sprintf("that tab id is this CLI's cached current tab for %s, not something you asked for; retry the command, or open a fresh tab", cachedTab.Base),
		openFreshTab.Remedy()
}

// clearCachedTabOnFailure drops the cache that produced the dead id and reports
// that it did. Terminal paths only, and the past-tense sentence is returned only
// when the file is actually gone, so the text can never claim a clearing that did
// not happen.
func clearCachedTabOnFailure(statusCode int, body []byte) string {
	if !namesCachedTab(statusCode, body) || cachedTab.StateFile == "" {
		return ""
	}
	if err := os.Remove(cachedTab.StateFile); err != nil && !os.IsNotExist(err) {
		return ""
	}
	return "   The cached current tab is now cleared, so that retry will target the server's current tab.\n"
}

// genericErrorCode is the code httpx.Error stamps on every UNCODED refusal — the largest
// error family in the product by a wide margin. It carries no information beyond the status
// that is already printed, so rendering it appends a bare "(error)" to the most common error
// shape and leaves a reader wondering what it denotes.
//
// Declared here rather than imported: the CLI is a client of the WIRE, not of the server's
// error helper, so it should not take a runtime dependency on it. TestTheGenericSentinelStill
// MatchesWhatTheServerSends drives httpx.Error and asserts the wire code is this value, which
// pins the agreement more tightly than a shared symbol would — it compares the bytes.
const genericErrorCode = "error"

// codeAddsInformation is the rule the suffix exists for, stated once. A code repeats nothing
// the reader already has: not the message beside it, and not the generic sentinel that every
// uncoded refusal carries.
func codeAddsInformation(code, message string) bool {
	return code != "" && code != message && code != genericErrorCode
}

func renderAPIErrorBody(statusCode int, body []byte) string {
	var errResp struct {
		Error   string         `json:"error"`
		Code    string         `json:"code"`
		Details map[string]any `json:"details"`
	}

	if err := json.Unmarshal(body, &errResp); err != nil {
		return fmt.Sprintf("Error %d: %s%s\n", statusCode, string(body), requestIDSuffix())
	}

	var b strings.Builder
	switch {
	case errResp.Error != "" && codeAddsInformation(errResp.Code, errResp.Error):
		fmt.Fprintf(&b, "Error %d: %s (%s)%s\n", statusCode, errResp.Error, errResp.Code, requestIDSuffix())
	case errResp.Error != "":
		fmt.Fprintf(&b, "Error %d: %s%s\n", statusCode, errResp.Error, requestIDSuffix())
	default:
		fmt.Fprintf(&b, "Error %d: %s%s\n", statusCode, string(body), requestIDSuffix())
	}

	if errResp.Details != nil {
		hint, _ := errResp.Details["hint"].(string)
		line, _ := errResp.Details["remedy"].(string)
		b.WriteString(renderGuidance(hint, remedy.Remedy(line)))
	}
	b.WriteString(renderGuidance(rejectedTokenProvenance(statusCode, errResp.Code), remedy.None))
	b.WriteString(renderGuidance(staleTabAdvice(statusCode, body)))
	return b.String()
}

// requestIDSuffix names the id the server logged this failure under, so the
// documented grep has an input. Empty when no response arrived.
func requestIDSuffix() string {
	if lastRequestID == "" {
		return ""
	}
	return " [requestId " + lastRequestID + "]"
}

// TokenSource is where the credential the CLI just sent came from — resolved at
// the moment of resolution, recorded as data so rendering an error performs no
// I/O. The zero value asks for nothing.
var tokenSource string

// UseTokenSource records the provenance of the token the following requests
// carry (an env var name, or the config file path the token was read from).
func UseTokenSource(source string) { tokenSource = source }

// rejectedTokenProvenance names where a rejected credential came from. That
// single sentence is the fix in the common remote case: the CLI silently sent
// the LOCAL machine's token to the host --server pointed at.
func rejectedTokenProvenance(statusCode int, code string) string {
	if statusCode != http.StatusUnauthorized || code != "bad_token" || tokenSource == "" {
		return ""
	}
	return fmt.Sprintf("the rejected token came from %s", tokenSource)
}

// renderGuidance is the ONE writer of both guidance slots, so a second producer cannot
// print prose into the line a caller reads as executable. Either half may be absent.
func renderGuidance(hint string, r remedy.Remedy) string {
	var b strings.Builder
	if hint != "" {
		fmt.Fprintf(&b, "\n💡 %s\n", hint)
	}
	if !r.Empty() {
		fmt.Fprintf(&b, "   Remedy: %s\n", r)
	}
	return b.String()
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

// AnnouncedCachedTab reports the cached tab the CLI last announced. It exists for
// the command layer's tests: the handoff is the seam between the two packages, and
// asserting on it is what pins that the CLI still performs it.
func AnnouncedCachedTab() CachedTab { return cachedTab }
