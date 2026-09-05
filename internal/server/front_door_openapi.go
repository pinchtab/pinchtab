package server

import (
	"net/http"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/handlers"
)

// frontDoorOpenAPIScope is the info.description carried by the spec the front door
// serves. The document is the catalogue-derived instance surface the front door
// forwards to; the orchestrator's own management routes are registered
// imperatively rather than in the shared route catalogue, so enumerating them
// here would add a second hand-maintained route list — the drift this endpoint
// exists to avoid — and they are named as a scope instead.
const frontDoorOpenAPIScope = "Browser-automation API reachable through the pinchtab server front door; requests are forwarded to the active instance. The front door also serves orchestrator management routes under /instances/ and /profiles/, which are not enumerated in this document."

// frontDoorOpenAPIRoutes is the single source of truth for API-discovery routes
// registered specially on the front door rather than through the route catalogue.
var frontDoorOpenAPIRoutes = []string{
	"GET /openapi.json",
	"GET /help",
}

// registerFrontDoorOpenAPI answers GET /openapi.json and its /help alias on the
// front door, which otherwise fall through to the mux's plain-text 404. It reads
// the live config per request so capability states track a dashboard save, and
// builds the same document the bridge serves from the shared route catalogue —
// one generator, two ports — carrying the scope note.
func registerFrontDoorOpenAPI(mux *http.ServeMux, live *config.Live) {
	serve := func(w http.ResponseWriter, _ *http.Request) {
		(&handlers.Handlers{Config: live.Get()}).ServeOpenAPI(w, frontDoorOpenAPIScope)
	}
	for _, route := range frontDoorOpenAPIRoutes {
		mux.HandleFunc(route, serve)
	}
}
