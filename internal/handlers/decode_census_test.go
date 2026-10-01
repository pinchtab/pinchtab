package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/srccensus"
)

func TestRequestBodiesDecodeOnlyThroughHTTPX(t *testing.T) {
	for _, site := range srccensus.Load(t, ".", handlersSourceFileFloor).CallsAllowingNone("json.NewDecoder") {
		t.Errorf("%s hand-rolls a JSON body decode; call decodeRequiredJSON, decodeOptionalJSON or httpx.DecodeJSON so an oversized body answers 413", site)
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
