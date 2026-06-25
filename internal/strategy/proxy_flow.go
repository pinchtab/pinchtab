package strategy

import (
	"fmt"
	"net/http"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/httpx"
	"github.com/pinchtab/pinchtab/internal/orchestrator"
)

// EnrichAndProxy applies route/target activity enrichment and proxies the
// request to target (preserving the request path).
func EnrichAndProxy(o *orchestrator.Orchestrator, w http.ResponseWriter, r *http.Request, target string) {
	activity.EnrichRouteActivity(r)
	EnrichForTarget(r, o, target)
	o.ProxyToTarget(w, r, target+r.URL.Path)
}

// ProxyToFirstRunning proxies to the first running instance, writing 503 with
// emptyMsg when none is running.
func ProxyToFirstRunning(o *orchestrator.Orchestrator, w http.ResponseWriter, r *http.Request, emptyMsg string) {
	target, status, err := o.FirstRunningURLForRequest(r)
	if err != nil {
		httpx.Error(w, status, err)
		return
	}
	if target == "" {
		httpx.Error(w, 503, fmt.Errorf("%s", emptyMsg))
		return
	}
	EnrichAndProxy(o, w, r, target)
}

// ProxyTabsToFirst handles GET /tabs by proxying to the first running instance,
// returning an empty tab list when none is running.
func ProxyTabsToFirst(o *orchestrator.Orchestrator, w http.ResponseWriter, r *http.Request) {
	target, status, err := o.FirstRunningURLForRequest(r)
	if err != nil {
		httpx.Error(w, status, err)
		return
	}
	if target == "" {
		httpx.JSON(w, 200, map[string]any{"tabs": []any{}})
		return
	}
	o.ProxyToTarget(w, r, target+"/tabs")
}
