package orchestrator

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pinchtab/pinchtab/internal/httpx/httpxtest"
	"github.com/pinchtab/pinchtab/internal/profiles"
)

func TestEveryOptionalBodyOrchestratorSiteDecodesTheSameWay(t *testing.T) {

	withProfile := func(t *testing.T) *Orchestrator {
		baseDir := t.TempDir()
		o := NewOrchestratorWithRunner(baseDir, &mockRunner{portAvail: true})
		o.hooks.portAvailable = func(int) bool { return true }
		o.hooks.processAlive = func(pid int) bool { return pid > 0 }
		pm := profiles.NewProfileManager(baseDir)
		if err := pm.CreateWithMeta("work", profiles.ProfileMeta{}); err != nil {
			t.Fatal(err)
		}
		o.profiles = pm
		return o
	}
	sites := []struct {
		name    string
		target  string
		payload string
		serve   func(t *testing.T, w http.ResponseWriter, r *http.Request)
	}{
		{"launch by name", "/instances/launch", `{"mode":"headless"}`, func(t *testing.T, w http.ResponseWriter, r *http.Request) {
			withProfile(t).handleLaunchByName(w, r)
		}},
		{"start instance", "/instances/start", `{"profileId":"work"}`, func(t *testing.T, w http.ResponseWriter, r *http.Request) {
			withProfile(t).handleStartInstance(w, r)
		}},
		{"start profile by id", "/profiles/work/start", `{"headless":true}`, func(t *testing.T, w http.ResponseWriter, r *http.Request) {
			r.SetPathValue("id", "work")
			withProfile(t).handleStartByID(w, r)
		}},
		{"instance tab open", "/instances/" + stubInstanceID + "/tabs/open", `{"url":"https://example.com"}`, func(t *testing.T, w http.ResponseWriter, r *http.Request) {
			o, _ := orchestratorOverStubChild(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"tabId":"t1"}`))
			}))
			r.SetPathValue("id", stubInstanceID)
			o.handleInstanceTabOpen(w, r)
		}},
	}
	for _, site := range sites {
		for _, c := range httpxtest.OptionalBodyCases() {
			t.Run(site.name+"/"+c.Name, func(t *testing.T) {
				w := httptest.NewRecorder()

				site.serve(t, w, c.Request(http.MethodPost, site.target, site.payload))

				c.Check(t, true, w)
			})
		}
	}
}
