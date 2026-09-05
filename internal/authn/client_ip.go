package authn

import (
	"context"
	"net"
	"net/http"
	"strings"

	"github.com/pinchtab/pinchtab/internal/config"
)

const forwardedForDirective = "for"

type clientIPKey struct{}

// resolvedClientIP distinguishes "the chain resolved a value" from "nothing ran",
// which a bare string on the context cannot: absent must fall back to the peer
// address, and a resolved empty value must not be mistaken for it.
type resolvedClientIP struct{ value string }

// ResolveClientIP adjudicates the client identity for a request. It is the one
// place a forwarding header is read for that decision; every consumer reads the
// answer back through ClientIP.
//
// trustedHops is how many trusted proxies sit in front, and the identity is that
// many elements from the RIGHT of the forwarding chain. Leftmost is what a client
// controls: every appending proxy keeps the header the client sent and adds the
// address it saw after it, so the client-most element is the client's own claim.
func ResolveClientIP(r *http.Request, trustProxy bool, trustedHops int) string {
	if r == nil {
		return ""
	}
	if trustProxy {
		if forwarded := forwardedClientIP(r, trustedHops); forwarded != "" {
			return forwarded
		}
	}
	return peerIP(r)
}

func WithClientIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, clientIPKey{}, resolvedClientIP{value: ip})
}

func forwardedClientIP(r *http.Request, trustedHops int) string {
	if header := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); header != "" {
		if ip := forwardedIP(trustedChainElement(header, trustedHops)); ip != "" {
			return ip
		}
	}
	return forwardedIP(forwardedDirective(trustedChainElement(r.Header.Get("Forwarded"), trustedHops), forwardedForDirective))
}

// trustedChainElement returns the element the trusted hop count points at, or the
// empty string when the chain is shorter than that count, so a short chain falls
// back to the transport peer rather than to whichever element is left. That is
// the only protection a count can give: a client pads its own header to any
// length, so a count HIGHER than the proxies that really append hands the
// identity back to the client. The count must equal the appending proxies.
func trustedChainElement(header string, trustedHops int) string {
	hops := config.TrustedProxyHopsOrDefault(trustedHops)
	elements := forwardedElements(header)
	if len(elements) < hops {
		return ""
	}
	return elements[len(elements)-hops]
}

func forwardedElements(header string) []string {
	var elements []string
	for _, part := range strings.Split(header, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			elements = append(elements, trimmed)
		}
	}
	return elements
}

func forwardedIP(value string) string {
	value = strings.Trim(strings.TrimSpace(value), `"`)
	if value == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(value); err == nil && host != "" {
		value = host
	}
	return strings.Trim(value, "[]")
}

func peerIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}
