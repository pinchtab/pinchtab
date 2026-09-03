package httpx

import (
	"net/http"
	"strings"
)

const (
	HeaderContentSecurityPolicy   = "Content-Security-Policy"
	HeaderFrameOptions            = "X-Frame-Options"
	HeaderContentTypeOptions      = "X-Content-Type-Options"
	HeaderStrictTransportSecurity = "Strict-Transport-Security"
)

var outerChainResponseHeaders = []string{
	RequestIDHeader,
	HeaderContentSecurityPolicy,
	HeaderFrameOptions,
	HeaderContentTypeOptions,
	HeaderStrictTransportSecurity,
}

var hopByHopHeaders = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Te",
	"Trailers",
	"Transfer-Encoding",
	"Upgrade",
	"Host",
	// The failure-reason pair is consumed per hop: the proxy reads it into
	// RecordFailureReason, which re-stamps it on its own response for any
	// further hop, so a copied-through duplicate would be the one value no
	// recorder produced.
	FailureCodeHeader,
	FailureMessageHeader,
}

func OuterChainResponseHeaders() []string {
	return append([]string(nil), outerChainResponseHeaders...)
}

func OuterChainOwnsResponseHeader(key string) bool {
	return containsFold(outerChainResponseHeaders, key)
}

func IsHopByHopHeader(key string) bool {
	return containsFold(hopByHopHeaders, key)
}

func containsFold(names []string, key string) bool {
	for _, name := range names {
		if strings.EqualFold(name, key) {
			return true
		}
	}
	return false
}

// ForwardRequestID is the one mechanism that decides the request id an instance is told,
// and it is the request-side companion to CopyProxiedResponseHeaders below.
//
// A hop that tells the instance nothing is the reason a phantom id existed: the instance
// found no inbound id, minted its own, and returned it on a response that already carried
// the outer chain's, so a caller could be handed a string that appears in no log on disk.
// Forwarding the outer chain's resolved value instead makes ONE proxied request findable
// by ONE id in both logs.
//
// It is deliberately narrow — this header and no other. A hop that fixed traceability by
// widening into a wholesale request copy would carry the caller's cookie and network
// identity to the instance along with it, which is what the per-hop strip lists exist to
// prevent. Callers that strip this header from their blind copy call this AFTER the strip:
// the two are different permissions, since a copy forwards whatever arrived under that
// name while this forwards the one value the outer chain resolved for this request.
func ForwardRequestID(dst, src http.Header) {
	if rid := strings.TrimSpace(src.Get(RequestIDHeader)); rid != "" {
		dst.Set(RequestIDHeader, rid)
	}
}

// CopyProxiedResponseHeaders is the one response-header copy every proxy hop uses. The
// outer middleware chain has already set its own X-Request-Id and security headers on this
// response, so appending the upstream copies makes them multi-valued: a caller then sees
// two request ids, one of which the outer process never minted and therefore never logged.
func CopyProxiedResponseHeaders(dst, src http.Header) {
	for key, values := range src {
		if IsHopByHopHeader(key) || OuterChainOwnsResponseHeader(key) {
			continue
		}
		for _, v := range values {
			dst.Add(key, v)
		}
	}
}
