package server

import (
	"net/http"

	"github.com/pinchtab/pinchtab/internal/httpx"
)

// notFoundEnvelope wraps a mux so a path it does not route answers the JSON error
// envelope every other failure uses, instead of net/http's plain-text
// "404 page not found" / "405 method not allowed". A registered handler still
// wins — including the coded per-family refusals like the bridge-mode session
// family — because it matches a pattern and this fallback fires only when none
// does. The mux's own not-found/method handler is run against a throwaway writer
// only to recover the status it would have written, so a wrong-method request
// stays a 405.
//
// A routed request is handed back to mux.ServeHTTP rather than to the handler
// mux.Handler returned: Handler reports WHICH pattern matched but does not
// populate the request's wildcard matches, so dispatching its result directly
// reaches every {id} route with r.PathValue("id") empty.
func notFoundEnvelope(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, pattern := mux.Handler(r); pattern != "" {
			mux.ServeHTTP(w, r)
			return
		}

		probe := &statusProbe{header: http.Header{}}
		mux.ServeHTTP(probe, r)
		status := probe.status
		if status == 0 {
			status = http.StatusNotFound
		}
		code := "not_found"
		if status == http.StatusMethodNotAllowed {
			code = "method_not_allowed"
		}
		httpx.ErrorCode(w, status, code, "no route for "+r.Method+" "+r.URL.Path, false, nil)
	})
}

type statusProbe struct {
	header http.Header
	status int
}

func (p *statusProbe) Header() http.Header         { return p.header }
func (p *statusProbe) Write(b []byte) (int, error) { return len(b), nil }
func (p *statusProbe) WriteHeader(status int)      { p.status = status }
