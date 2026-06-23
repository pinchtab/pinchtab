package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/routes"
)

// httpMethods is the set of OpenAPI path-item keys that denote an operation;
// other keys (parameters, x-* extensions) are skipped during route extraction.
var httpMethods = map[string]string{
	"get":    "GET",
	"post":   "POST",
	"put":    "PUT",
	"delete": "DELETE",
	"patch":  "PATCH",
}

// TestOpenAPIPathsAreRegisteredRoutes guards the hand-curated OpenAPI doc against
// drift: every documented path+method must map to a route the bridge actually
// registers, so a stale/phantom entry (e.g. the former GET /nav) cannot survive.
func TestOpenAPIPathsAreRegisteredRoutes(t *testing.T) {
	h := &Handlers{}
	rec := httptest.NewRecorder()
	h.HandleOpenAPI(rec, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("HandleOpenAPI status = %d, want 200", rec.Code)
	}

	var doc struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode openapi: %v", err)
	}
	if len(doc.Paths) == 0 {
		t.Fatal("openapi doc has no paths")
	}

	registered := expectedRoutes()

	for path, item := range doc.Paths {
		for verb := range item {
			method, ok := httpMethods[strings.ToLower(verb)]
			if !ok {
				continue
			}
			route := method + " " + path
			if !registered[route] {
				t.Errorf("documented route %q is not registered", route)
			}
		}
	}
}

// TestOpenAPICoversCatalogRoutes guards the OTHER direction: every catalog
// endpoint (root unless tab-only, plus its /tabs/{id}/... variant) must be
// documented. A new routes.Core() entry that isn't generated into the doc fails
// here, so the OpenAPI response can no longer omit registered catalog routes.
func TestOpenAPICoversCatalogRoutes(t *testing.T) {
	h := &Handlers{}
	rec := httptest.NewRecorder()
	h.HandleOpenAPI(rec, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("HandleOpenAPI status = %d, want 200", rec.Code)
	}

	var doc struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode openapi: %v", err)
	}

	documented := func(method, path string) bool {
		item, ok := doc.Paths[path]
		if !ok {
			return false
		}
		_, ok = item[strings.ToLower(method)]
		return ok
	}

	for _, ep := range routes.Core() {
		if !tabOnlyRoutes[ep.Route()] && !documented(ep.Method, ep.Path) {
			t.Errorf("catalog route %q is not documented in /openapi.json", ep.Route())
		}
		if ep.TabScoped && !documented(ep.Method, "/tabs/{id}"+ep.Path) {
			t.Errorf("tab-scoped catalog route %q is not documented in /openapi.json", ep.TabRoute())
		}
	}
}
