package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

// tabScopedFamily is every /tabs/{id}/... handler that decodes a typed body and
// reconciles it against the path id through decodeJSONBody + requirePathTabIDMatch.
// The prologue lives in one place now, so the point of walking all six is that a
// handler which stops routing through it fails here rather than drifting quietly.
var tabScopedFamily = []struct {
	name      string
	handler   func(*Handlers) http.HandlerFunc
	validBody string
}{
	{"emulation/viewport", func(h *Handlers) http.HandlerFunc { return h.HandleTabSetViewport }, `{"width":800,"height":600}`},
	{"emulation/geolocation", func(h *Handlers) http.HandlerFunc { return h.HandleTabSetGeolocation }, `{"latitude":1,"longitude":2}`},
	{"emulation/media", func(h *Handlers) http.HandlerFunc { return h.HandleTabSetMedia }, `{"feature":"prefers-color-scheme","value":"dark"}`},
	{"emulation/offline", func(h *Handlers) http.HandlerFunc { return h.HandleTabSetOffline }, `{"offline":true}`},
	{"emulation/credentials", func(h *Handlers) http.HandlerFunc { return h.HandleTabSetCredentials }, `{"username":"u","password":"p"}`},
	{"emulation/headers", func(h *Handlers) http.HandlerFunc { return h.HandleTabSetHeaders }, `{"headers":{"X-Probe":"1"}}`},
}

func driveTabScoped(t *testing.T, handler http.HandlerFunc, pathID, body string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/tabs/tab_abc/probe", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if pathID != "" {
		req.SetPathValue("id", pathID)
	}
	w := httptest.NewRecorder()
	handler(w, req)

	var resp struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w.Code, resp.Error
}

func TestTabScopedPrologueRejections(t *testing.T) {
	oversized := `{"tabId":"` + strings.Repeat("x", maxBodySize+1) + `"}`

	cases := []struct {
		name       string
		pathID     string
		body       func(valid string) string
		wantError  string
		wantStatus int
	}{
		{
			name:      "missing tab id",
			pathID:    "",
			body:      func(valid string) string { return valid },
			wantError: "tab id required",
		},
		{
			name:      "malformed body",
			pathID:    "tab_abc",
			body:      func(string) string { return `{"tabId":` },
			wantError: "decode: unexpected EOF",
		},
		{
			name:       "oversized body",
			pathID:     "tab_abc",
			body:       func(string) string { return oversized },
			wantError:  "decode: http: request body too large",
			wantStatus: http.StatusRequestEntityTooLarge,
		},
		{
			name:      "body tabId disagrees with the path",
			pathID:    "tab_abc",
			body:      func(string) string { return `{"tabId":"tab_other"}` },
			wantError: "tabId in body does not match path id",
		},
	}

	for _, ep := range tabScopedFamily {
		for _, tc := range cases {
			t.Run(ep.name+"/"+tc.name, func(t *testing.T) {
				h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
				status, gotError := driveTabScoped(t, ep.handler(h), tc.pathID, tc.body(ep.validBody))

				wantStatus := tc.wantStatus
				if wantStatus == 0 {
					wantStatus = http.StatusBadRequest
				}
				if status != wantStatus {
					t.Fatalf("status = %d, want %d (error=%q)", status, wantStatus, gotError)
				}
				if gotError != tc.wantError {
					t.Fatalf("error = %q, want %q", gotError, tc.wantError)
				}
			})
		}
	}

	if len(tabScopedFamily) == 0 || len(cases) == 0 {
		t.Fatal("the family or the case table is empty, so this test proves nothing")
	}
}

// TestTabScopedPrologueDecodesBeforeCheckingThePathID pins the one ordering the
// fold changed: the body is decoded first, so a request that is both malformed
// and missing its path id reports the decode error. No client can reach it —
// ServeMux redirects /tabs//... rather than matching {id} to an empty segment,
// which TestEmptyPathSegmentNeverReachesATabHandler holds — but the direct-call
// precedence is now uniform across the family instead of split two ways.
func TestTabScopedPrologueDecodesBeforeCheckingThePathID(t *testing.T) {
	for _, ep := range tabScopedFamily {
		t.Run(ep.name, func(t *testing.T) {
			h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
			status, gotError := driveTabScoped(t, ep.handler(h), "", `{"tabId":`)

			if status != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (error=%q)", status, gotError)
			}
			if gotError != "decode: unexpected EOF" {
				t.Fatalf("error = %q, want the decode error to win over the missing path id", gotError)
			}
		})
	}
}

// TestEmptyPathSegmentNeverReachesATabHandler is what makes the reordering above
// safe to reason about: the "missing tab id" branch is unreachable over HTTP, so
// its position in the prologue cannot change any response a client can observe.
func TestEmptyPathSegmentNeverReachesATabHandler(t *testing.T) {
	reached := false
	mux := http.NewServeMux()
	mux.HandleFunc("POST /tabs/{id}/emulation/viewport", func(w http.ResponseWriter, r *http.Request) {
		reached = true
	})

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/tabs//emulation/viewport", strings.NewReader("{}")))

	if reached {
		t.Fatal("an empty {id} segment reached the handler, so the missing-tab-id branch is client-reachable after all and its ordering is observable")
	}
	if w.Code != http.StatusTemporaryRedirect {
		t.Fatalf("status = %d, want 307; ServeMux is expected to redirect the empty segment rather than match it", w.Code)
	}
}
