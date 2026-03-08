package strategy

import (
	"net/http"

	"github.com/pinchtab/pinchtab/internal/web"
)

// RegisterCapabilityRoute registers a route that either proxies to the active
// handler or returns a feature-gated 403 when the capability is disabled.
func RegisterCapabilityRoute(mux *http.ServeMux, route string, enabled bool, feature, setting, code string, handler http.HandlerFunc) {
	if enabled {
		mux.HandleFunc(route, handler)
		return
	}
	mux.HandleFunc(route, web.DisabledEndpointHandler(feature, setting, code))
}
