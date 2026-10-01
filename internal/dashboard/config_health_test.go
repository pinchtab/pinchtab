package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/api/types"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/profiles"
)

func profileDir(t *testing.T, baseDir, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(baseDir, name, "Default"), 0o700); err != nil {
		t.Fatal(err)
	}
}

func defaultProfileListing(t *testing.T, pm *profiles.ProfileManager) []map[string]any {
	t.Helper()
	mux := http.NewServeMux()
	pm.RegisterHandlers(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/profiles", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /profiles = %d: %s", rec.Code, rec.Body.String())
	}
	var listed []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode listing: %v", err)
	}
	return listed
}

// /health used to count every non-quarantined profile, temporaries included, while
// GET /profiles hides temporaries by default — so the two disagreed on one word.
// Each profile now lands in exactly one bucket, and the buckets reconcile with the
// list: the default listing keeps quarantined profiles and hides temporaries.
func TestHealthCountsEachProfileInExactlyOneBucketThatReconcilesWithTheListing(t *testing.T) {
	baseDir := t.TempDir()
	profileDir(t, baseDir, "default")
	profileDir(t, baseDir, "work")
	profileDir(t, baseDir, "instance-9868")
	profileDir(t, baseDir, "instance-9869")
	profileDir(t, baseDir, "instance-9870")
	profileDir(t, baseDir, "default.quarantine-1700000001")
	// A quarantined TEMPORARY: the real quarantine flow renames instance-9871 to
	// instance-9871.quarantine-<ts>, so List() reports it Temporary AND Quarantined.
	// GET /profiles hides it as temporary, so /health must count it as temporary too
	// or profiles + quarantinedProfiles no longer equals the default list length.
	profileDir(t, baseDir, "instance-9871.quarantine-1700000002")
	pm := profiles.NewProfileManager(baseDir)

	api := newConfigAPIForTest(config.Load(), nil, pm, nil, nil, "test", time.Now())
	w := httptest.NewRecorder()
	api.HandleHealth(w, httptest.NewRequest(http.MethodGet, "/health", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	var health types.DashboardHealth
	if err := json.NewDecoder(w.Body).Decode(&health); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if health.Profiles != 2 || health.TemporaryProfiles != 4 || health.QuarantinedProfiles != 1 {
		t.Errorf("profiles/temporary/quarantined = %d/%d/%d, want 2/4/1 — the quarantined temporary belongs in temporaryProfiles, the bucket GET /profiles treats it as", health.Profiles, health.TemporaryProfiles, health.QuarantinedProfiles)
	}
	all, err := pm.List()
	if err != nil {
		t.Fatal(err)
	}
	if health.Profiles+health.TemporaryProfiles+health.QuarantinedProfiles != len(all) {
		t.Errorf("buckets sum to %d, want every one of the %d profiles counted once", health.Profiles+health.TemporaryProfiles+health.QuarantinedProfiles, len(all))
	}

	listed := defaultProfileListing(t, pm)
	if health.Profiles+health.QuarantinedProfiles != len(listed) {
		t.Errorf("profiles+quarantinedProfiles = %d, want the default GET /profiles length %d", health.Profiles+health.QuarantinedProfiles, len(listed))
	}
	for _, entry := range listed {
		if entry["temporary"] == true {
			t.Errorf("the default listing served a temporary profile, so profiles cannot be reconciled against it: %v", entry)
		}
	}
}

type routedInstances struct {
	instances []bridge.Instance
	def       bridge.Instance
	routable  bool
}

func (s routedInstances) List() []bridge.Instance { return s.instances }

func (s routedInstances) DefaultInstance() (bridge.Instance, bool) { return s.def, s.routable }

func TestHealthDefaultInstanceNamesTheRoutedInstanceRatherThanTheFirstListed(t *testing.T) {
	listed := []bridge.Instance{
		{ID: "inst_first", Status: "stopped"},
		{ID: "inst_routed", Status: "running", Responsiveness: bridge.ResponsivenessResponsive},
	}
	lister := routedInstances{instances: listed, def: listed[1], routable: true}
	for i := range 10 {
		def, _ := healthBody(t, lister)["defaultInstance"].(map[string]any)
		if def["id"] != "inst_routed" || def["status"] != "running" || def["responsiveness"] != bridge.ResponsivenessResponsive {
			t.Fatalf("call %d: defaultInstance = %v, want the routed inst_routed", i, def)
		}
	}
}

func TestHealthDefaultInstanceFallsBackToTheFirstListedWithoutARoutedInstance(t *testing.T) {
	listed := []bridge.Instance{{ID: "inst_starting", Status: "starting"}, {ID: "inst_other", Status: "starting"}}
	for name, lister := range map[string]InstanceLister{
		"no routable instance":   routedInstances{instances: listed},
		"lister without routing": plainInstances{instances: listed},
	} {
		def, _ := healthBody(t, lister)["defaultInstance"].(map[string]any)
		if def["id"] != "inst_starting" || def["status"] != "starting" {
			t.Errorf("%s: defaultInstance = %v, want the first listed inst_starting", name, def)
		}
	}
	if _, present := healthBody(t, routedInstances{})["defaultInstance"]; present {
		t.Error("defaultInstance present with no instances")
	}
}
