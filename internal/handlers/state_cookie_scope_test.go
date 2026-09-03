package handlers

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/state"
)

const secretCookieValue = "SUPERSECRET-SESSION-TOKEN"

type secretJarBridge struct{ mockBridge }

func newSecretJarBridge() *secretJarBridge {
	b := &secretJarBridge{}
	b.evaluateFn = func(_ string, result any) error {
		if p, ok := result.(*string); ok {
			*p = "{}"
		}
		return nil
	}
	return b
}

func (secretJarBridge) GetRawCookies(context.Context) ([]bridge.RawCookie, error) {
	return []bridge.RawCookie{{Name: "session", Value: secretCookieValue, Domain: "localhost", Path: "/"}}, nil
}

func stateResponse(t *testing.T, h *Handlers, serve func(*Handlers, *httptest.ResponseRecorder)) (string, map[string]any) {
	t.Helper()
	w := httptest.NewRecorder()
	serve(h, w)
	if w.Code != 200 {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return w.Body.String(), body
}

func serveStateCurrent(h *Handlers, w *httptest.ResponseRecorder) {
	h.HandleStateCurrent(w, httptest.NewRequest("GET", "/state?tabId=tab1", nil))
}

func serveStateShow(h *Handlers, w *httptest.ResponseRecorder) {
	h.HandleStateShow(w, httptest.NewRequest("GET", "/state/show?name=saved", nil))
}

func savedStateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if _, err := state.Save(dir, &state.StateFile{
		Name:    "saved",
		Cookies: []state.Cookie{{Name: "session", Value: secretCookieValue, Domain: "localhost", Path: "/"}},
	}, ""); err != nil {
		t.Fatal(err)
	}
	return dir
}

// allowStateExport alone used to hand back the full cookie jar that allowCookies
// refuses one route over. Cookie VALUES need the cookies capability whatever route
// answers; without it the count stands in and the route still answers 200.
func TestStateReadBackWithholdsCookieValuesWithoutTheCookiesCapability(t *testing.T) {
	for _, tc := range []struct {
		name  string
		serve func(*Handlers, *httptest.ResponseRecorder)
	}{
		{"GET /state", serveStateCurrent},
		{"GET /state/show", serveStateShow},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := savedStateDir(t)
			withheld := New(newSecretJarBridge(), &config.RuntimeConfig{AllowStateExport: true, StateDir: dir}, nil, nil, nil)
			raw, body := stateResponse(t, withheld, tc.serve)
			if strings.Contains(raw, secretCookieValue) {
				t.Errorf("the secret cookie value reached an allowStateExport-only caller: %s", raw)
			}
			if body["cookies"] != float64(1) {
				t.Errorf("cookies = %v, want the count 1 standing in for the values", body["cookies"])
			}

			full := New(newSecretJarBridge(), &config.RuntimeConfig{AllowStateExport: true, AllowCookies: true, StateDir: dir}, nil, nil, nil)
			raw, body = stateResponse(t, full, tc.serve)
			jar, ok := body["cookies"].([]any)
			if !ok || len(jar) != 1 || !strings.Contains(raw, secretCookieValue) {
				t.Errorf("with both capabilities cookies = %v, want the full values", body["cookies"])
			}
		})
	}
}

func TestSaveAndLoadRoundTripWithoutTheCookiesCapability(t *testing.T) {
	dir := t.TempDir()
	h := New(newSecretJarBridge(), &config.RuntimeConfig{AllowStateExport: true, StateDir: dir}, nil, nil, nil)

	raw, saved := stateResponse(t, h, func(h *Handlers, w *httptest.ResponseRecorder) {
		h.HandleStateSave(w, httptest.NewRequest("POST", "/state/save", strings.NewReader(`{"name":"trip","tabId":"tab1"}`)))
	})
	if saved["cookies"] != float64(1) || strings.Contains(raw, secretCookieValue) {
		t.Fatalf("save answered %s, want a count and no value", raw)
	}
	sf, err := state.Load(state.ResolvePath(dir, "trip"), "")
	if err != nil || len(sf.Cookies) != 1 || sf.Cookies[0].Value != secretCookieValue {
		t.Fatalf("the saved file does not hold the cookie server-side: %+v %v", sf, err)
	}

	raw, loaded := stateResponse(t, h, func(h *Handlers, w *httptest.ResponseRecorder) {
		h.HandleStateLoad(w, httptest.NewRequest("POST", "/state/load", strings.NewReader(`{"name":"trip","tabId":"tab1"}`)))
	})
	if loaded["cookiesRestored"] != float64(1) || strings.Contains(raw, secretCookieValue) {
		t.Fatalf("load answered %s, want one cookie restored and no value", raw)
	}
}
