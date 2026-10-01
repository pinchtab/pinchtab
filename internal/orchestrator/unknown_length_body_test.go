package orchestrator

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/profiles"
)

func unknownLength(req *http.Request) *http.Request {
	req.ContentLength = -1
	return req
}

func TestAnEmptyUnknownLengthStartBodyIsAbsent(t *testing.T) {
	baseDir := t.TempDir()
	o := NewOrchestratorWithRunner(baseDir, &mockRunner{portAvail: true})
	o.hooks.portAvailable = func(int) bool { return true }
	o.hooks.processAlive = func(pid int) bool { return pid > 0 }
	pm := profiles.NewProfileManager(baseDir)
	if err := pm.CreateWithMeta("work", profiles.ProfileMeta{}); err != nil {
		t.Fatal(err)
	}
	o.profiles = pm

	req := unknownLength(httptest.NewRequest(http.MethodPost, "/profiles/work/start", strings.NewReader("")))
	req.SetPathValue("id", "work")
	w := httptest.NewRecorder()
	o.handleStartByID(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 for an empty body of unknown length: %s", w.Code, w.Body.String())
	}
}

func TestAnUnknownLengthStartBodyIsDecoded(t *testing.T) {
	baseDir := t.TempDir()
	o := NewOrchestratorWithRunner(baseDir, &mockRunner{portAvail: true})
	o.ApplyRuntimeConfig(&config.RuntimeConfig{
		DefaultTarget: "cloak-1",
		Targets:       config.BrowserTargetsConfig{"cloak-1": {Provider: config.BrowserCloak}},
	})
	pm := profiles.NewProfileManager(baseDir)
	if err := pm.CreateWithMeta("work", profiles.ProfileMeta{}); err != nil {
		t.Fatal(err)
	}
	o.profiles = pm

	req := unknownLength(httptest.NewRequest(http.MethodPost, "/profiles/work/start", strings.NewReader(`{"browser":"ghost"}`)))
	req.SetPathValue("id", "work")
	w := httptest.NewRecorder()
	o.handleStartByID(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want the 400 a decoded browser:ghost earns: %s", w.Code, w.Body.String())
	}
}

func TestAnUnknownLengthTabOpenBodyCarriesItsURL(t *testing.T) {
	var got map[string]any
	o, _ := orchestratorOverStubChild(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"tabId":"t1"}`))
	}))
	for _, tc := range []struct{ body, wantURL string }{
		{`{"url":"https://example.com"}`, "https://example.com"},
		{"", ""},
	} {
		got = nil
		req := unknownLength(httptest.NewRequest(http.MethodPost, "/instances/"+stubInstanceID+"/tabs/open", strings.NewReader(tc.body)))
		req.SetPathValue("id", stubInstanceID)
		w := httptest.NewRecorder()
		o.handleInstanceTabOpen(w, req)
		if w.Code != http.StatusOK || got["url"] != tc.wantURL {
			t.Fatalf("body %q: %d %s, instance saw %v, want url %q", tc.body, w.Code, w.Body.String(), got, tc.wantURL)
		}
	}
}

func TestTheTabIDPeekReadsAnUnknownLengthBodyAndReplaysItWhole(t *testing.T) {
	small := `{"tabId":"TAB_A","kind":"click"}`
	large := `{"tabId":"TAB_B","pad":"` + strings.Repeat("x", maxBodyPeek) + `"}`
	for _, tc := range []struct{ body, wantTabID string }{{small, "TAB_A"}, {large, ""}} {
		req := unknownLength(httptest.NewRequest(http.MethodPost, "/action", strings.NewReader(tc.body)))
		req.Header.Set("Content-Type", "application/json")
		if got := peekBodyTabID(req); got != tc.wantTabID {
			t.Fatalf("peek of a %d-byte body = %q, want %q", len(tc.body), got, tc.wantTabID)
		}
		rest, err := io.ReadAll(req.Body)
		if err != nil || !bytes.Equal(rest, []byte(tc.body)) {
			t.Fatalf("downstream read %d bytes of %d after the peek (err %v)", len(rest), len(tc.body), err)
		}
	}
}
