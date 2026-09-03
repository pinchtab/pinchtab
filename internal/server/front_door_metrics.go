package server

import (
	"net/http"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/browsersession"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/handlers"
	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/routes"
	"github.com/pinchtab/pinchtab/internal/session"
)

// FrontDoorHandler is the orchestrator front door's whole middleware chain. It
// takes the publication point rather than a config: the chain is built once and
// serves for the life of the process, so a captured *RuntimeConfig would freeze
// every setting it reads at its boot value while the dashboard reported saves as
// applied. It is the orchestrator's Live, shared with ConfigAPI and AuthAPI, so a
// save is visible to all four holders at once.
//
// It is a named function rather than an expression inside RunDashboard because
// the counters live in the middleware and the rejections that were invisible happen
// ABOVE the mux: proving they are counted needs the real chain, and the isolated
// middleware already behaved correctly, which is why nothing caught this.
func FrontDoorHandler(
	live *config.Live,
	recorder activity.Recorder,
	sessions *browsersession.Manager,
	sessionStore *session.Store,
	mux http.Handler,
) http.Handler {
	return handlers.StripInternalHeadersMiddleware(
		handlers.RequestIDMiddleware(
			handlers.ClientIPMiddleware(live, activity.Middleware(
				recorder,
				"server",
				handlers.SecurityHeadersMiddleware(live,
					handlers.LoggingMiddleware(handlers.RateLimitMiddleware(handlers.CorsMiddleware(live, handlers.AuthMiddlewareWithSessions(live, sessions, sessionStore, mux)))),
				),
			)),
		),
	)
}

// registerFrontDoorMetrics serves the front door's OWN counters at /metrics.
// It used to be a proxied route, so a client read the instance's numbers and the
// front-door counters — the only ones that see auth rejections and unrouted paths
// — were unreachable by any client. The instance figures did not become
// unreachable in exchange: an instance still answers its own /metrics, reachable
// through GET /instances/{id}/metrics.
func registerFrontDoorMetrics(mux *http.ServeMux) {
	for _, route := range routes.FrontDoorLocalRoutes() {
		mux.HandleFunc(route, func(w http.ResponseWriter, r *http.Request) {
			httpx.JSON(w, http.StatusOK, handlers.DiagnosticsSnapshot(handlers.LayerFrontDoor))
		})
	}
}
