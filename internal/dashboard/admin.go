package dashboard

import (
	"net/http"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

// AdminDeps holds the components needed for dashboard admin route registration.
type AdminDeps struct {
	ConfigAPI       *ConfigAPI
	AuthAPI         *AuthAPI
	AgentSessionAPI *AgentSessionAPI
	Activity        activity.Recorder
	ServerMetrics   func() map[string]any
}

// RegisterAdminRoutes registers all /api/* dashboard admin endpoints.
func (d *Dashboard) RegisterAdminRoutes(mux *http.ServeMux, deps AdminDeps) {
	d.RegisterHandlers(mux)
	deps.ConfigAPI.RegisterHandlers(mux)
	deps.AuthAPI.RegisterHandlers(mux)
	if deps.AgentSessionAPI != nil {
		deps.AgentSessionAPI.RegisterHandlers(mux)
	}
	activity.RegisterHandlers(mux, deps.Activity)
	mux.HandleFunc("GET /api/metrics", func(w http.ResponseWriter, r *http.Request) {
		httpx.JSON(w, 200, map[string]any{"metrics": deps.ServerMetrics()})
	})
}
