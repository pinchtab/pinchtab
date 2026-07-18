package apiclient

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// mustRequest is doRequest with the common fatal-on-transport-error policy.
func mustRequest(client *http.Client, token string, r request) (int, []byte) {
	status, body, err := doRequest(client, token, r)
	if err != nil {
		fatal("Request failed: %v", err)
	}
	return status, body
}

func exitOnAPIError(r request, status int, body []byte) {
	if status >= 400 {
		fmt.Fprint(os.Stderr, renderAPIError(r, status, body))
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

func ExitWithAPIError(statusCode int, body []byte) {
	handleAPIError(statusCode, body)
	os.Exit(1)
}

func handleAPIError(statusCode int, body []byte) {
	fmt.Fprint(os.Stderr, renderAPIErrorBody(statusCode, body))
}

func renderAPIErrorBody(statusCode int, body []byte) string {
	var errResp struct {
		Error   string         `json:"error"`
		Code    string         `json:"code"`
		Details map[string]any `json:"details"`
	}

	if err := json.Unmarshal(body, &errResp); err != nil {
		return fmt.Sprintf("Error %d: %s\n", statusCode, string(body))
	}

	var b strings.Builder
	if errResp.Error != "" {
		fmt.Fprintf(&b, "Error %d: %s\n", statusCode, errResp.Error)
	} else {
		fmt.Fprintf(&b, "Error %d: %s\n", statusCode, string(body))
	}

	if errResp.Details != nil {
		if hint, ok := errResp.Details["hint"].(string); ok && hint != "" {
			fmt.Fprintf(&b, "\n💡 %s\n", hint)
		}
		if remedy, ok := errResp.Details["remedy"].(string); ok && remedy != "" {
			fmt.Fprintf(&b, "   Remedy: %s\n", remedy)
		}
	}
	return b.String()
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
