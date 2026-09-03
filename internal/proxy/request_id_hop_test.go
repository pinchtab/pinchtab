package proxy

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/pinchtab/pinchtab/internal/handlers"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

// requestIDLogCapture reads the requestId attribute off the access-log records
// LoggingMiddleware emits, so a test can assert what a hop actually LOGGED rather than
// inferring it from what the hop was sent. That distinction is the whole criterion here:
// the id has to be findable on disk, and the access log is where an operator looks.
type requestIDLogCapture struct {
	mu  sync.Mutex
	ids []string
}

func (c *requestIDLogCapture) Enabled(context.Context, slog.Level) bool { return true }

func (c *requestIDLogCapture) WithAttrs([]slog.Attr) slog.Handler { return c }

func (c *requestIDLogCapture) WithGroup(string) slog.Handler { return c }

func (c *requestIDLogCapture) Handle(_ context.Context, record slog.Record) error {
	record.Attrs(func(attr slog.Attr) bool {
		if attr.Key == "requestId" {
			c.mu.Lock()
			c.ids = append(c.ids, attr.Value.String())
			c.mu.Unlock()
		}
		return true
	})
	return nil
}

func (c *requestIDLogCapture) logged() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.ids...)
}

func captureAccessLog(t *testing.T) *requestIDLogCapture {
	t.Helper()

	capture := &requestIDLogCapture{}
	previous := slog.Default()
	slog.SetDefault(slog.New(capture))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return capture
}

// The criterion literally: ONE proxied request, and the SAME id in BOTH access logs. Both
// hops run the real RequestIDMiddleware and the real LoggingMiddleware, and the assertion
// reads the emitted log records — so this fails if the instance logs an id of its own,
// which is exactly the state that made a quoted id unfindable.
func TestTheSameRequestIDIsLoggedByBothTheOuterServerAndTheInstance(t *testing.T) {
	capture := captureAccessLog(t)

	instance := httptest.NewServer(handlers.RequestIDMiddleware(handlers.LoggingMiddleware(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))))
	t.Cleanup(instance.Close)

	targetURL, err := url.Parse(instance.URL)
	if err != nil {
		t.Fatal(err)
	}
	front := handlers.RequestIDMiddleware(handlers.LoggingMiddleware(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			Forward(w, r, targetURL, Options{})
		})))
	front.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/tabs", nil))

	logged := capture.logged()
	if len(logged) != 2 {
		t.Fatalf("access log produced %d requestId entries (%v), want one from each hop; this test is measuring nothing otherwise", len(logged), logged)
	}
	if logged[0] == "" || logged[1] == "" {
		t.Fatalf("an access-log entry carried no requestId: %v", logged)
	}
	if logged[0] != logged[1] {
		t.Errorf("instance logged %q and the outer server logged %q — one request wrote two ids, so a caller quoting either can be found in only one log", logged[0], logged[1])
	}
}

// instanceEchoingItsOwnRequestID stands in for the instance: it runs the SAME
// RequestIDMiddleware the outer server does, which is the whole point — that middleware
// honours an inbound id and mints one only when there is none, so what it resolves is
// exactly what the instance would write in its own log. The resolved id is echoed in the
// body so a test can read what the instance logged, not merely what it was sent.
func instanceEchoingItsOwnRequestID(t *testing.T) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(handlers.RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.Header.Get(httpx.RequestIDHeader)))
	})))
	t.Cleanup(srv.Close)
	return srv
}

func proxyThroughOuterChain(t *testing.T, upstream *httptest.Server, inbound string) (outerID string, instanceID string, response *httptest.ResponseRecorder) {
	t.Helper()

	targetURL, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	front := handlers.RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// What the outer chain resolved for this request: RequestIDMiddleware stamps it
		// onto the request, and it is the value msg=request logs.
		outerID = r.Header.Get(httpx.RequestIDHeader)
		Forward(w, r, targetURL, Options{})
	}))

	req := httptest.NewRequest("GET", "/tabs", nil)
	if inbound != "" {
		req.Header.Set(httpx.RequestIDHeader, inbound)
	}
	front.ServeHTTP(rec, req)
	return outerID, rec.Body.String(), rec
}

// The traceability half, and the criterion that defines done: ONE proxied request must be
// findable by ONE id in both logs. Before the forward the instance was told nothing, minted
// its own id and returned it, so a caller could be handed a string that appears in no log
// on disk.
//
// Both halves are asserted together on purpose. Filtering alone leaves the response
// single-valued but the instance still logging a different id; forwarding alone makes the
// two ids equal but the header still multi-valued. Neither substitutes for the other, so a
// build that did one and not the other must fail here.
func TestOneProxiedRequestIsTraceableByOneIDInBothLogs(t *testing.T) {
	for _, tc := range []struct {
		name    string
		inbound string
	}{
		{name: "the outer chain mints the id", inbound: ""},
		{name: "a caller supplies its own trace key", inbound: "caller-chosen-trace-key"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			outerID, instanceID, rec := proxyThroughOuterChain(t, instanceEchoingItsOwnRequestID(t), tc.inbound)

			if outerID == "" {
				t.Fatal("the outer chain resolved no request id; this test is measuring nothing")
			}
			if tc.inbound != "" && outerID != tc.inbound {
				t.Errorf("outer id = %q, want the caller's own %q honoured", outerID, tc.inbound)
			}

			// The forwarding half.
			if instanceID != outerID {
				t.Errorf("instance logged %q but the outer chain logged %q — a caller quoting one id can only be found in one of the two logs", instanceID, outerID)
			}

			// The doubling half, on the same response.
			values := rec.Header().Values(httpx.RequestIDHeader)
			if len(values) != 1 {
				t.Fatalf("%s = %v, want exactly one value; a caller cannot tell which of two ids their HTTP library will show them", httpx.RequestIDHeader, values)
			}
			if values[0] != outerID {
				t.Errorf("response carries %q but the outer chain logged %q, so the value a caller reads is not the one on disk", values[0], outerID)
			}
		})
	}
}

// The forward must not become a wholesale request-header copy. This is the guard-rail the
// gate asked for: the rest of the strip list protects things the instance has no business
// learning, and widening the forward to fix traceability would quietly hand them over.
func TestForwardingTheRequestIDDoesNotWidenIntoAWholesaleCopy(t *testing.T) {
	var received http.Header
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = r.Header.Clone()
	}))
	t.Cleanup(upstream.Close)

	targetURL, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/tabs", nil)
	for name, value := range map[string]string{
		"Cookie":            "pinchtab_auth_token=session-secret",
		"Forwarded":         "for=203.0.113.10",
		"X-Forwarded-For":   "203.0.113.10",
		"X-Forwarded-Host":  "app.example",
		"X-Forwarded-Proto": "https",
		"X-Real-Ip":         "203.0.113.10",
	} {
		req.Header.Set(name, value)
	}
	req.Header.Set(httpx.RequestIDHeader, "traced")

	front := handlers.RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		Forward(w, r, targetURL, Options{})
	}))
	front.ServeHTTP(httptest.NewRecorder(), req)

	if received == nil {
		t.Fatal("upstream received no request; this test is measuring nothing")
	}
	if got := received.Get(httpx.RequestIDHeader); got != "traced" {
		t.Fatalf("request id = %q, want it forwarded — without that the rest of this test cannot distinguish a narrow forward from no forward at all", got)
	}
	for _, name := range []string{"Cookie", "Forwarded", "X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto", "X-Real-Ip"} {
		if got := received.Get(name); got != "" {
			t.Errorf("%s = %q reached the instance; forwarding the request id widened into a wholesale copy and carried a stripped header with it", name, got)
		}
	}
}

// The strip list and the forward are two different permissions over one header name, and
// the census keeps them from collapsing into each other: x-request-id must stay ON the
// strip list (so a copied value never passes) while the forward re-adds the resolved one.
// Deleting it from the list would also forward it, and every behavioural test above would
// stay green — this is the only check that tells the two implementations apart.
func TestTheRequestIDIsForwardedByReAddingItRatherThanByLeavingTheStripList(t *testing.T) {
	if _, stripped := strippedProxyRequestHeaders[strings.ToLower(httpx.RequestIDHeader)]; !stripped {
		t.Error("x-request-id left strippedProxyRequestHeaders; it must stay stripped from the blind copy and be re-added by httpx.ForwardRequestID, or an arbitrary inbound value passes through on any path that skipped the outer middleware")
	}

	copied := http.Header{}
	copyRequestHeaders(copied, http.Header{httpx.RequestIDHeader: {"from-the-blind-copy"}})
	if got := copied.Get(httpx.RequestIDHeader); got != "" {
		t.Errorf("the blind copy carried the request id as %q; the re-add is meant to be the only way it travels", got)
	}
}
