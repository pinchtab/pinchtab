package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/config/workflow"
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
	var health healthEnvelope
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

type postureInstances struct {
	instances []bridge.Instance
	postures  map[string]*workflow.EnforcedSecurity
}

func (s postureInstances) List() []bridge.Instance { return s.instances }

func (s postureInstances) EnforcedSecurity() map[string]*workflow.EnforcedSecurity {
	return s.postures
}

func securedHealth(t *testing.T, instances InstanceLister, cfg *config.RuntimeConfig) healthEnvelope {
	t.Helper()
	api := newConfigAPIForTest(cfg, instances, nil, nil, nil, "test", time.Now())
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	api.HandleHealth(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	var health healthEnvelope
	if err := json.NewDecoder(w.Body).Decode(&health); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return health
}

// /health used to report the front door's own RuntimeConfig as the security
// posture while every instance keeps enforcing what it booted with — the readout
// spoke for a fleet it never asked. Two policies are in play here: the front door
// configured with IDPI on and one allowed domain, an instance enforcing IDPI off
// and a bare wildcard, and a second instance nobody could reach.
func TestHealthTellsTheFrontDoorsConfiguredPolicyApartFromWhatInstancesEnforce(t *testing.T) {
	cfg := config.Load()
	cfg.IDPI.Enabled = true
	cfg.AllowedDomains = []string{"example.com"}

	health := securedHealth(t, postureInstances{
		instances: []bridge.Instance{
			{ID: "inst_lagging", Status: "running"},
			{ID: "inst_silent", Status: "running"},
		},
		postures: map[string]*workflow.EnforcedSecurity{
			"inst_lagging": {IDPIEnabled: false, AllowedDomains: []string{"*"}},
			"inst_silent":  nil,
		},
	}, cfg)

	if health.Security == nil || health.EnforcedSecurity == nil {
		t.Fatalf("health carries no configured/enforced pair: %+v", health)
	}
	if health.Security.Scope != frontDoorConfigurationScope {
		t.Errorf("security.scope = %q, want %q: the block is one process's configuration, not the fleet's posture", health.Security.Scope, frontDoorConfigurationScope)
	}
	if !health.Security.IDPIEnabled {
		t.Errorf("security.idpiEnabled = false, want the front door's own configured value")
	}
	if !health.EnforcedSecurity.Divergent {
		t.Errorf("enforcedSecurity.divergent = false while an instance enforces IDPI off under a wildcard")
	}
	if len(health.EnforcedSecurity.Instances) != 2 {
		t.Fatalf("enforced instances = %+v, want one entry per running instance", health.EnforcedSecurity.Instances)
	}

	lagging := health.EnforcedSecurity.Instances[0]
	if lagging.ID != "inst_lagging" || lagging.Comparison != "diverges" || !lagging.Queried {
		t.Errorf("lagging instance = %+v, want a queried entry marked diverges", lagging)
	}
	if lagging.Policy == nil || lagging.Policy.IDPIEnabled {
		t.Errorf("lagging policy = %+v, want the posture the instance enforces, IDPI off", lagging.Policy)
	}

	silent := health.EnforcedSecurity.Instances[1]
	if silent.Comparison != "unknown" || silent.Queried {
		t.Errorf("unreachable instance = %+v, want comparison unknown and queried false — not an instance enforcing nothing", silent)
	}
	if silent.Policy != nil {
		t.Errorf("unreachable instance carries a policy %+v, which nobody read", silent.Policy)
	}
}

// An instance enforcing exactly what the front door has configured is the only
// case that reads as agreement, so "divergent" cannot be a constant.
func TestHealthReportsNoDivergenceWhenTheInstanceEnforcesTheConfiguredPolicy(t *testing.T) {
	cfg := config.Load()
	cfg.IDPI.Enabled = true
	cfg.AllowedDomains = []string{"example.com"}

	health := securedHealth(t, postureInstances{
		instances: []bridge.Instance{{ID: "inst_current", Status: "running"}},
		postures: map[string]*workflow.EnforcedSecurity{
			"inst_current": ptr(workflow.EnforcedSecurityFor(cfg)),
		},
	}, cfg)

	if health.EnforcedSecurity.Divergent {
		t.Errorf("divergent = true for an instance enforcing the configured policy: %+v", health.EnforcedSecurity.Instances)
	}
	if got := health.EnforcedSecurity.Instances[0].Comparison; got != "match" {
		t.Errorf("comparison = %q, want match", got)
	}
}

func ptr[T any](v T) *T { return &v }
