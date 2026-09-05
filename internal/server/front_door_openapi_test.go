package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

// The reported defect: /openapi.json and /help answered a plain-text 404 on the
// full-server front door while the bridge served them. This pins both on the
// front-door mux so the two registration surfaces cannot drift apart again.
func TestFrontDoorServesOpenAPIAndHelp(t *testing.T) {
	mux := http.NewServeMux()
	registerFrontDoorOpenAPI(mux, config.NewLive(&config.RuntimeConfig{}))
	handler := notFoundEnvelope(mux)

	wantRoutes := map[string]bool{"GET /openapi.json": true, "GET /help": true}
	if len(frontDoorOpenAPIRoutes) != len(wantRoutes) {
		t.Fatalf("front-door API-discovery routes = %v, want exactly /openapi.json and /help", frontDoorOpenAPIRoutes)
	}
	for _, pattern := range frontDoorOpenAPIRoutes {
		if !wantRoutes[pattern] {
			t.Fatalf("unexpected front-door API-discovery route %q; want exactly /openapi.json and /help", pattern)
		}
		_, path, ok := strings.Cut(pattern, " ")
		if !ok {
			t.Fatalf("invalid front-door API-discovery route pattern %q", pattern)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))

		if w.Code != http.StatusOK {
			t.Fatalf("GET %s: status = %d, want 200: %s", path, w.Code, w.Body.String())
		}
		var doc map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
			t.Fatalf("GET %s: body is not JSON (%v): %s", path, err, w.Body.String())
		}
		if doc["openapi"] == nil || doc["paths"] == nil {
			t.Errorf("GET %s: not an OpenAPI document: %v", path, doc)
		}
		info, _ := doc["info"].(map[string]any)
		if info == nil || info["description"] == nil {
			t.Errorf("GET %s: the front-door spec must state its scope in info.description: %v", path, doc["info"])
		}
	}
}

// The scope note must actually name the surface it does not enumerate, so a
// caller reading a 200 is not told the instance surface is the whole API.
func TestFrontDoorOpenAPIStatesItsScope(t *testing.T) {
	mux := http.NewServeMux()
	registerFrontDoorOpenAPI(mux, config.NewLive(&config.RuntimeConfig{}))

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))

	var doc struct {
		Info struct {
			Description string `json:"description"`
		} `json:"info"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	for _, want := range []string{"/instances", "/profiles"} {
		if !strings.Contains(doc.Info.Description, want) {
			t.Errorf("scope note does not name %q as also-served-but-unenumerated: %q", want, doc.Info.Description)
		}
	}
}
