// Package urls provides URL normalization and validation utilities.
package urls

import (
	"fmt"
	"net/url"
	"strings"
)

// Normalize adds https:// if no protocol is specified. It is EnsureScheme under
// another name, kept because the CLI reads better for it.
//
// It used to recognise only the literal "http://" and "https://" prefixes, so a
// target written with any other slash run — "https:/example.com/x",
// "https:\\example.com\x", "https:example.com/x" — was not recognised as having a
// scheme at all and got another one prepended in front of it, producing
// "https://https:/example.com/x", whose host reads as "https". The CLI silently sent
// the server a different destination than the user typed.
func Normalize(rawURL string) string {
	return EnsureScheme(rawURL)
}

// Sanitize normalizes a URL. Bare hostnames get https:// added.
// All explicit schemes are passed through (user knows what they're doing).
func Sanitize(rawURL string) (string, error) {
	if rawURL == "" {
		return "", fmt.Errorf("empty URL")
	}

	normalized := EnsureScheme(rawURL)
	if normalized == rawURL {
		return rawURL, nil
	}

	parsed, err := url.Parse(normalized)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}

	if parsed.Host == "" {
		return "", fmt.Errorf("missing host in URL")
	}

	return parsed.String(), nil
}

// IsValid returns true if URL is safe for navigation.
func IsValid(rawURL string) bool {
	_, err := Sanitize(rawURL)
	return err == nil
}

// EnsureScheme is the ONE place a scheme-less target acquires one, so no consumer
// has to hand-roll a bare-URL branch to find the host inside it.
//
// It also has to agree with the BROWSER about where the authority begins, because
// the guard that decides whether a target is safe reads its answer and Chrome reads
// its own. WHATWG's special-authority-ignore-slashes state skips any run of "/" and
// "\" after a special scheme's colon, so "https:\\10.0.0.5\x", "https:/10.0.0.5/x",
// "https:////10.0.0.5/x" and even "https:10.0.0.5/x" all name the host 10.0.0.5 to a
// browser — and none of them did to net/url, which read them as hostless. A hostless
// target skipped resolution and the private-IP check, so those spellings reached a
// private address the correctly-spelled one is refused for. Collapsing the run is
// what makes the two parsers agree.
//
// The protocol-relative form is the same defect one step earlier: "//10.0.0.5/x" has
// no scheme, so this used to prepend one and produce "https:////10.0.0.5/x" — the
// bypass manufactured out of the most ordinary input of the set. Stripping the
// leading run before prepending is what stops a prepend destroying an authority the
// input already had.
//
// "file" is deliberately not collapsed: its authority rules differ, and
// "file:///tmp/x" means an empty authority and an absolute path rather than the host
// "tmp".
//
// https:// is the default a bare host gets. It is the assumption Normalize already
// makes for the CLI and Sanitize for MCP, and it is the safer of the two: a target
// written without a scheme is upgraded rather than downgraded.
func EnsureScheme(rawURL string) string {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return trimmed
	}
	if scheme, rest, ok := splitAuthorityScheme(trimmed); ok {
		return scheme + "://" + browserAuthorityAndPath(rest)
	}
	if strings.Contains(trimmed, "://") {
		return trimmed
	}
	parsed, err := url.Parse(trimmed)
	if err == nil && parsed.Scheme != "" && !opaqueStartsWithPort(parsed.Opaque) {
		return trimmed
	}
	return "https://" + browserAuthorityAndPath(trimmed)
}

// browserAuthorityAndPath rewrites the part of a special-scheme URL that a browser
// reads with backslashes and slashes interchangeable: the leading run between the
// colon and the authority is skipped, and every remaining backslash up to the query
// or fragment is a slash. Query and fragment are left alone — the parser does not
// remap them either, and rewriting them would change what the page receives.
func browserAuthorityAndPath(rest string) string {
	rest = strings.TrimLeft(rest, authoritySlashes)
	cut := strings.IndexAny(rest, "?#")
	if cut < 0 {
		return strings.ReplaceAll(rest, `\`, "/")
	}
	return strings.ReplaceAll(rest[:cut], `\`, "/") + rest[cut:]
}

// authoritySlashes are the two characters WHATWG skips between a special scheme's
// colon and the authority. A backslash is not a typo the parser rejects — it is a
// slash for this purpose, which is exactly why the backslash spellings reached a
// private host past a guard that read them as hostless.
const authoritySlashes = `/\`

// authorityCollapsingSchemes are WHATWG's special schemes minus "file". Every one of
// them takes an authority, so anything between its colon and the host is punctuation
// the browser ignores.
var authorityCollapsingSchemes = map[string]bool{
	"http": true, "https": true, "ws": true, "wss": true, "ftp": true,
}

// splitAuthorityScheme recognises a leading special scheme and returns it with the
// remainder. The scheme must be a real scheme token, so "./rel://x" is not mistaken
// for one and left alone rather than rewritten.
func splitAuthorityScheme(raw string) (scheme, rest string, ok bool) {
	colon := strings.Index(raw, ":")
	if colon <= 0 {
		return "", "", false
	}
	candidate := raw[:colon]
	if !isSchemeToken(candidate) || !authorityCollapsingSchemes[strings.ToLower(candidate)] {
		return "", "", false
	}
	return candidate, raw[colon+1:], true
}

func isSchemeToken(candidate string) bool {
	for i, r := range candidate {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case i > 0 && (r >= '0' && r <= '9' || r == '+' || r == '-' || r == '.'):
		default:
			return false
		}
	}
	return candidate != ""
}

// opaqueStartsWithPort reports whether an opaque remainder begins with a port —
// "8080" or "8080/path" — which is what makes "intranet:8080" a host and a port
// rather than a scheme and a body.
func opaqueStartsWithPort(opaque string) bool {
	digits := strings.SplitN(opaque, "/", 2)[0]
	if digits == "" {
		return false
	}
	for _, r := range digits {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
