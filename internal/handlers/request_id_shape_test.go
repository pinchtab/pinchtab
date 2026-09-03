package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func servedRequestID(t *testing.T, inbound string) (onRequest, onResponse string) {
	t.Helper()
	handler := RequestIDMiddleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		onRequest = r.Header.Get("X-Request-Id")
	}))
	req := httptest.NewRequest("GET", "/test", nil)
	if inbound != "" {
		req.Header.Set("X-Request-Id", inbound)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	return onRequest, w.Header().Get("X-Request-Id")
}

// Honouring an inbound id is the point: it is what makes one request findable in
// the outer log and in the proxied instance's log. Every shape a real client
// sends must survive.
func TestRequestIDMiddlewareKeepsAWellFormedInboundID(t *testing.T) {
	for _, id := range []string{
		"req-123",
		"3f2504e0-4f89-11d3-9a0c-0305e82c3301",
		"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
		"01ARZ3NDEKTSV4RRFFQ69G5FAV",
		"trace.id_with:separators",
	} {
		onRequest, onResponse := servedRequestID(t, id)
		if onRequest != id || onResponse != id {
			t.Errorf("inbound %q became request=%q response=%q; a well-formed id must be forwarded verbatim", id, onRequest, onResponse)
		}
	}
}

// The adopted id is copied into every audit and access log line for the request,
// into the dashboard activity stream, and onto the proxied hop. The server
// accepts 256 KiB of headers, so an unconstrained id is an arbitrary payload
// with a guaranteed path into all of them.
func TestRequestIDMiddlewareReplacesAnIDThatIsNotShapedLikeOne(t *testing.T) {
	cases := map[string]string{
		"over length":       strings.Repeat("a", maxRequestIDLen+1),
		"newline":           "req\r\n123",
		"space":             "req 123",
		"control character": "req\x00123",
		"log field syntax":  `req" level=INFO msg="forged`,
		"non-ascii":         "req-123-ü",
	}
	for name, inbound := range cases {
		t.Run(name, func(t *testing.T) {
			onRequest, onResponse := servedRequestID(t, inbound)
			if onRequest == inbound {
				t.Errorf("the request carries the supplied id verbatim; it reaches the audit log, the activity stream and the proxied hop unchanged")
			}
			if onRequest != onResponse {
				t.Errorf("request id %q and response id %q disagree; one request must have one id", onRequest, onResponse)
			}
			if onRequest == "" {
				t.Error("no id was generated to replace the rejected one")
			}
			if !usableRequestID(onRequest) {
				t.Errorf("the generated replacement %q is itself not a usable id", onRequest)
			}
		})
	}
}

// The length bound has to be a bound, not a coincidence of the fixtures.
func TestRequestIDLengthBoundIsInclusive(t *testing.T) {
	atLimit := strings.Repeat("a", maxRequestIDLen)
	if !usableRequestID(atLimit) {
		t.Errorf("an id of exactly maxRequestIDLen was rejected; the bound must be inclusive")
	}
	if usableRequestID(atLimit + "a") {
		t.Error("an id one character over the bound was accepted")
	}
}

// Absent stays generated, which is the behaviour every other test in this
// package relies on.
func TestRequestIDMiddlewareGeneratesWhenNoneIsSupplied(t *testing.T) {
	onRequest, onResponse := servedRequestID(t, "")
	if onRequest == "" || onRequest != onResponse {
		t.Errorf("request=%q response=%q, want one generated id on both", onRequest, onResponse)
	}
}
