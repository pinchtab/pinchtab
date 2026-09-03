package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/browsers"
	"github.com/pinchtab/pinchtab/internal/session"
)

// refusedKey records a key the product deliberately does not carry: patch writes the
// ignored value the census looks for, and reason is the wording the refusal must keep, so
// a user who reaches for the key learns why it is not on offer.
type refusedKey struct {
	reason string
	patch  string
	value  string
}

// refusedRatherThanCarried holds the keys `config set` says no to instead of accepting and
// dropping. A key silently ignored is worse than a key with an invisible default, so an
// entry here is a promise the refusal exists and explains itself.
var refusedRatherThanCarried = map[string]refusedKey{
	"observability.activity.stateDir": {
		reason: ActivityStateDirRefusal,
		patch:  `{"observability":{"activity":{"stateDir":"/tmp/census-elsewhere"}}}`,
		value:  "/tmp/census-elsewhere",
	},
}

// acceptedAndDropped holds the keys `config set` still takes and the value in effect never
// reflects. Each is a filed defect, not a licence: the entry names the mechanism and the
// card that owns it, and the census reds again the moment one of them starts working.
var acceptedAndDropped = map[string]string{}

// censusValueFor holds the values the generic candidate list cannot guess: a key whose
// setter or loader takes only a fixed vocabulary needs a member of it, or the census would
// measure the rejection instead of the round-trip.
var censusValueFor = map[string]string{
	"server.logLevel":                      "error",
	"server.port":                          "9123",
	"server.bind":                          "0.0.0.0",
	"browser.extraFlags":                   "--census-flag",
	"instanceDefaults.mode":                "headed",
	"instanceDefaults.stealthLevel":        "full",
	"instanceDefaults.tabEvictionPolicy":   "close_oldest",
	"instanceDefaults.tabPolicy.eviction":  "reject",
	"instanceDefaults.tabPolicy.lifecycle": "close_idle",
	"multiInstance.strategy":               "explicit",
	"multiInstance.allocationPolicy":       "round_robin",
	"scheduler.strategy":                   "fair-fifo",
	"security.attach.allowSchemes":         "wss",
	"security.attach.allowHosts":           "10.0.0.1",
	"security.allowedDomains":              "census.example.com",
	"security.downloadAllowedDomains":      "census.example.com",
	"security.trustedProxyCIDRs":           "10.0.0.0/8",
	"security.trustedResolveCIDRs":         "10.0.0.0/8",
	"autoSolver.solvers":                   "semantic",
	"sessions.agent.mode":                  session.ModeOff,
}

// censusCandidates are tried in order for every other key: the first one the setter takes
// and that changes the file value is what the round-trip is measured with.
var censusCandidates = []string{"census-value", "17", "false", "true", "/tmp/census-dir"}

// TestEverySettableConfigKeyReachesTheRuntime is the standing census behind this
// behaviour: a key `config set` accepts and writes to the file, and that the value in
// effect never reflects, changes nothing and says so nowhere. Adding such a key fails here
// until the value arrives or the product refuses it with a reason.
//
// ITS SCOPE IS THE ADDRESSABLE SET, which is narrower than the declared one. It walks the
// keys `config set` can address, so a section the editor cannot address at all is invisible
// to it — no candidate is ever tried, and nothing reds. sessions.agent was exactly that: the
// schema declared it, the loader honoured it, `config patch` accepted and discarded it, and
// this census could not see any of that because `config set sessions.agent.enabled` answered
// "unknown field". The class of defect this cannot detect is therefore an UNADDRESSABLE key,
// and the guard for that one is TestTheWireTwinCarriesEveryDeclaredField, which walks the
// declared types instead. The two are complements; neither subsumes the other.
func TestEverySettableConfigKeyReachesTheRuntime(t *testing.T) {
	paths := addressableConfigPaths(t)
	if len(paths) < 100 {
		t.Fatalf("found only %d addressable config paths, so this census is not walking FileConfig", len(paths))
	}

	settable := 0
	for _, path := range paths {
		file, want, ok := censusFileValue(path)
		if !ok {
			checkRefusedKey(t, path)
			continue
		}
		settable++

		got := censusRuntimeValue(t, path, file)
		reason, accounted := acceptedAndDropped[path]
		switch {
		case got == want && accounted:
			t.Errorf("%s now survives into the value in effect; drop its entry so the census keeps meaning what it says", path)
		case got == want:
		case accounted:
			t.Logf("accounted: %s is set to %q and the value in effect is %q — %s", path, want, got, reason)
		default:
			t.Errorf("config set %s is accepted but the value never comes back: file says %q, the value in effect is %q; carry it, or refuse it with the reason", path, want, got)
		}
	}
	if settable < 100 {
		t.Fatalf("only %d of %d addressable paths took a census value, so this census is barely measuring anything", settable, len(paths))
	}

	for path := range acceptedAndDropped {
		if !slicesContains(paths, path) {
			t.Errorf("%s is accounted as dropped but is no longer an addressable key; drop the entry", path)
		}
	}
	for path := range refusedRatherThanCarried {
		if !slicesContains(paths, path) {
			t.Errorf("%s is accounted as refused but is no longer an addressable key; drop the entry", path)
		}
	}
}

// checkRefusedKey covers the keys `config set` will not take. Most are file-only fields and
// none of this census's business; the ones a refusal was written for must keep both halves
// of the bargain — the refusal carries its reason, and the value still does not arrive by
// the file route the refusal leaves open.
func checkRefusedKey(t *testing.T, path string) {
	t.Helper()

	entry, ok := refusedRatherThanCarried[path]
	if !ok {
		return
	}

	fc := DefaultFileConfig()
	err := SetConfigValue(&fc, path, entry.value)
	if err == nil {
		t.Errorf("config set %s accepted %q; a key the runtime does not carry must be refused", path, entry.value)
		return
	}
	if !strings.Contains(err.Error(), entry.reason) {
		t.Errorf("config set %s refusal = %q, want it to carry the reason %q", path, err.Error(), entry.reason)
	}

	patched := DefaultFileConfig()
	if err := PatchConfigJSON(&patched, entry.patch); err != nil {
		t.Fatalf("PatchConfigJSON(%s) error = %v", path, err)
	}
	// Contains, not equality: a key that starts working may be a root the runtime joins a
	// subdirectory onto, and that still means the file value arrived.
	if got := censusRuntimeValue(t, path, &patched); strings.Contains(got, entry.value) {
		t.Errorf("%s now reaches the runtime (%q); drop its entry and stop refusing it", path, got)
	}
}

// censusFileValue picks a value the setter takes and that lands in the file, and hands back
// the whole file so the caller loads exactly what was written.
func censusFileValue(path string) (*FileConfig, string, bool) {
	candidates := censusCandidates
	if override, ok := censusValueFor[path]; ok {
		candidates = append([]string{override}, candidates...)
	}
	candidates = append(candidates, browsers.IDs()...)

	for _, candidate := range candidates {
		fc := DefaultFileConfig()
		before, err := GetConfigValue(&fc, path)
		if err != nil {
			return nil, "", false
		}
		if err := SetConfigValue(&fc, path, candidate); err != nil {
			continue
		}
		after, err := GetConfigValue(&fc, path)
		if err != nil || after == before || strings.TrimSpace(after) == "" {
			continue
		}
		return &fc, after, true
	}
	return nil, "", false
}

// censusRuntimeValue writes the census file and reads the path back the way `config get`
// does — through the loaded RuntimeConfig, never from the file bytes, which is the whole
// point: the file value is what is under suspicion.
func censusRuntimeValue(t *testing.T, path string, file *FileConfig) string {
	t.Helper()

	body, err := json.Marshal(file)
	if err != nil {
		t.Fatalf("marshal census config for %s: %v", path, err)
	}
	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, body, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	t.Setenv("PINCHTAB_CONFIG", configPath)
	t.Setenv("PINCHTAB_TOKEN", "")

	got, err := resolvedConfigValue(path)
	if err != nil {
		t.Fatalf("resolvedConfigValue(%q) error = %v", path, err)
	}
	return got
}
