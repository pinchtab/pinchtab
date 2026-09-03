package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

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
	pm := profiles.NewProfileManager(baseDir)

	api := newConfigAPIForTest(config.Load(), nil, pm, nil, nil, "test", time.Now())
	w := httptest.NewRecorder()
	api.HandleHealth(w, httptest.NewRequest(http.MethodGet, "/health", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	var health healthEnvelope
	if err := json.NewDecoder(w.Body).Decode(&health); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if health.Profiles != 2 || health.TemporaryProfiles != 3 || health.QuarantinedProfiles != 1 {
		t.Errorf("profiles/temporary/quarantined = %d/%d/%d, want 2/3/1", health.Profiles, health.TemporaryProfiles, health.QuarantinedProfiles)
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
