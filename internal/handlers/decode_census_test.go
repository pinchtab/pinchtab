package handlers

import (
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/srccensus"
)

var jsonDecoderOwners = map[string]string{
	"limits.go::decodeRequiredJSON": "the shared required-body decoder",
	"limits.go::decodeOptionalJSON": "the shared optional-body decoder",
	"upload.go::HandleUpload":       "decodes under the configured upload request cap, not maxBodySize",
}

func TestRequestBodiesDecodeOnlyThroughTheSharedHelpers(t *testing.T) {
	found := map[string]bool{}
	for _, site := range srccensus.Load(t, ".", handlersSourceFileFloor).Calls(t, "json.NewDecoder") {
		key := site.File + "::" + site.Func
		found[key] = true
		if _, ok := jsonDecoderOwners[key]; !ok {
			t.Errorf("%s hand-rolls a JSON body decode; call decodeRequiredJSON, decodeOptionalJSON or decodeJSONBody so an oversized body answers 413", site)
		}
	}
	var stale []string
	for owner := range jsonDecoderOwners {
		if !found[owner] {
			stale = append(stale, owner)
		}
	}
	sort.Strings(stale)
	for _, owner := range stale {
		t.Errorf("%s no longer calls json.NewDecoder; drop it from jsonDecoderOwners", owner)
	}
}

func TestOversizedBodyAnswers413(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{AllowEvaluate: true}, nil, nil, nil)
	oversized := `{"url":"` + strings.Repeat("a", maxBodySize) + `","expression":"1"}`
	for _, tc := range []struct {
		path   string
		handle http.HandlerFunc
	}{
		{"/navigate", h.HandleNavigate},
		{"/evaluate", h.HandleEvaluate},
	} {
		t.Run(tc.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			tc.handle(w, httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(oversized)))
			if w.Code != http.StatusRequestEntityTooLarge {
				t.Fatalf("status = %d, want 413: %s", w.Code, w.Body.String())
			}
		})
	}
}
