package dashboard

import (
	"crypto/rand"
	"encoding/hex"
	"io/fs"
	"net/http"
)

func (d *Dashboard) RegisterHandlers(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/events", d.handleSSE)
	mux.HandleFunc("GET /api/agents", d.handleAgents)
	mux.HandleFunc("GET /api/agents/{id}", d.handleAgent)
	mux.HandleFunc("GET /api/agents/{id}/events", d.handleAgentSSE)
	mux.HandleFunc("POST /api/agents/{id}/events", d.handleAgentEventsByID)

	sub, _ := fs.Sub(dashboardFS, "dashboard")
	fileServer := http.FileServer(http.FS(sub))

	// Serve static assets under /dashboard/ with long cache (hashed filenames)
	mux.Handle("GET /dashboard/assets/", http.StripPrefix("/dashboard", d.withLongCache(fileServer)))
	mux.Handle("GET /dashboard/favicon.png", http.StripPrefix("/dashboard", d.withLongCache(fileServer)))

	// SPA: serve dashboard.html for /, /login, and /dashboard/*
	mux.Handle("GET /{$}", d.withNoCache(http.HandlerFunc(d.handleDashboardUI)))
	mux.Handle("GET /login", d.withNoCache(http.HandlerFunc(d.handleDashboardUI)))
	mux.Handle("GET /dashboard", d.withNoCache(http.HandlerFunc(d.handleDashboardUI)))
	mux.Handle("GET /dashboard/{path...}", d.withNoCache(http.HandlerFunc(d.handleDashboardUI)))
}

const fallbackHTML = `<!DOCTYPE html>
<html lang="en"><head><meta charset="UTF-8"/><meta name="viewport" content="width=device-width,initial-scale=1.0"/>
<title>PinchTab Dashboard</title>
<style>body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;display:flex;justify-content:center;align-items:center;min-height:100vh;margin:0;background:#0a0a0a;color:#e0e0e0}.c{text-align:center;max-width:480px;padding:2rem}h1{font-size:1.5rem;margin-bottom:.5rem}p{color:#888;line-height:1.6}code{background:#1a1a2e;padding:2px 8px;border-radius:4px;font-size:.9em}</style>
</head><body><div class="c"><h1>🦀 Dashboard not built</h1>
<p>The React dashboard needs to be compiled before use.<br/>
Run <code>./dev build</code> or <code>./scripts/build-dashboard.sh</code> then rebuild the Go binary.</p>
</div></body></html>`

func (d *Dashboard) handleDashboardUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	data, err := dashboardFS.ReadFile("dashboard/dashboard.html")
	if err != nil {
		_, _ = w.Write([]byte(fallbackHTML))
		return
	}
	_, _ = w.Write(data)
}

func (d *Dashboard) withNoCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		next.ServeHTTP(w, r)
	})
}

func (d *Dashboard) withLongCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Assets have hashes in filenames - cache for 1 year
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		next.ServeHTTP(w, r)
	})
}

func generateID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
