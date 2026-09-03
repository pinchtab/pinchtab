package bridge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/srccensus"
)

func reclaimFixture(t *testing.T) string {
	t.Helper()

	base := t.TempDir()
	writeProfileDir(t, filepath.Join(base, "default"), 32)
	writeProfileDir(t, filepath.Join(base, "work"), 32)
	writeProfileDir(t, quarantinePathAt(filepath.Join(base, "default"), 1700000001), 64)
	writeProfileDir(t, quarantinePathAt(filepath.Join(base, "default"), 1700000002), 128)
	writeProfileDir(t, quarantinePathAt(filepath.Join(base, "work"), 1700000003), 256)
	return base
}

func removalNames(removals []QuarantineRemoval) []string {
	var names []string
	for _, removal := range removals {
		names = append(names, filepath.Base(removal.Path))
	}
	return names
}

func totalBytes(removals []QuarantineRemoval) int64 {
	var total int64
	for _, removal := range removals {
		total += removal.Bytes
	}
	return total
}

// The reclaim the automatic prune cannot express: every quarantined directory under the
// base goes, across profiles, with no keep count. It reaches quarantines whose profile
// never quarantines again — the ones the sibling-scoped prune leaves on disk for ever.
func TestReclaimRemovesEveryQuarantinedProfileAndReportsWhatItFreed(t *testing.T) {
	base := reclaimFixture(t)

	removed, err := ReclaimQuarantinedProfiles(base, "")
	if err != nil {
		t.Fatalf("ReclaimQuarantinedProfiles() error = %v", err)
	}

	if len(removed) != 3 {
		t.Fatalf("removed = %v, want all three quarantined directories", removalNames(removed))
	}
	if got := totalBytes(removed); got != 64+128+256 {
		t.Errorf("reclaimed bytes = %d, want %d", got, 64+128+256)
	}
	if got := dirNames(t, base); strings.Join(got, ",") != "default,work" {
		t.Errorf("survivors = %v, want only the two live profiles", got)
	}
}

// The bare invocation is the rule an agent depends on, so the assertion is the negative
// one: the directories are still there afterwards, byte for byte. A test that only
// checked the reported total would pass on an implementation that deleted them.
func TestReclaimableReportsTheBacklogWithoutRemovingAnything(t *testing.T) {
	base := reclaimFixture(t)
	before := dirNames(t, base)

	reclaimable, err := ReclaimableQuarantinedProfiles(base, "")
	if err != nil {
		t.Fatalf("ReclaimableQuarantinedProfiles() error = %v", err)
	}

	if len(reclaimable) != 3 {
		t.Fatalf("reclaimable = %v, want the three quarantined directories", removalNames(reclaimable))
	}
	if got := totalBytes(reclaimable); got != 64+128+256 {
		t.Errorf("reclaimable bytes = %d, want %d", got, 64+128+256)
	}
	if got := dirNames(t, base); strings.Join(got, ",") != strings.Join(before, ",") {
		t.Fatalf("directories after the dry run = %v, want the untouched %v", got, before)
	}
	for _, removal := range reclaimable {
		if _, err := os.Stat(filepath.Join(removal.Path, "State")); err != nil {
			t.Errorf("%s lost its contents to a dry run: %v", removal.Path, err)
		}
	}
}

// quarantineKeep: 0 is the documented way to keep every quarantined copy, and it is the
// configuration with no other remedy — the automatic prune returns having done nothing,
// for ever. Reclaim must stay reachable there, which is why it does not take a keep count
// at all. Both halves are asserted on ONE tree so the comparison is between the two
// paths and not between two fixtures.
func TestReclaimStillWorksWhenTheAutomaticPruneIsSetToKeepEverything(t *testing.T) {
	base := reclaimFixture(t)
	profileDir := filepath.Join(base, "default")

	pruned, err := PruneQuarantinedProfiles(profileDir, "", KeepAllQuarantinedProfiles)
	if err != nil {
		t.Fatalf("PruneQuarantinedProfiles() error = %v", err)
	}
	if len(pruned) != 0 {
		t.Fatalf("the automatic prune removed %v under keep-everything", removalNames(pruned))
	}
	if len(dirNames(t, base)) != 5 {
		t.Fatalf("keep-everything changed the tree: %v", dirNames(t, base))
	}

	removed, err := ReclaimQuarantinedProfiles(base, "")
	if err != nil {
		t.Fatalf("ReclaimQuarantinedProfiles() error = %v", err)
	}
	if len(removed) != 3 {
		t.Fatalf("reclaim removed %v under keep-everything, want all three", removalNames(removed))
	}
}

// A live profile must be unreachable however the request is shaped. Two live profiles sit
// beside three quarantines here; what excludes them is IsQuarantinedProfileDir, applied
// both when the selection is built and again inside the deleter.
func TestReclaimLeavesLiveProfilesAlone(t *testing.T) {
	base := reclaimFixture(t)

	if _, err := ReclaimQuarantinedProfiles(base, ""); err != nil {
		t.Fatalf("ReclaimQuarantinedProfiles() error = %v", err)
	}

	for _, live := range []string{"default", "work"} {
		if _, err := os.Stat(filepath.Join(base, live, "State")); err != nil {
			t.Errorf("live profile %q was removed by a reclaim: %v", live, err)
		}
	}
}

// A user may name a profile "<something>.quarantine-<digits>" and that directory is
// indistinguishable on disk from a real quarantine — same name shape, same contents, no
// marker to tell them apart. So it IS reclaimed, and this test states that rather than
// pretending otherwise. What makes it acceptable: the name is the only thing quarantine
// itself writes, the product never reads a directory with that name as a profile (the
// listing flags it quarantined), and reclaim is explicit and confirmed. The exposure is
// bounded to a user who chose a name the product reserves.
func TestReclaimTakesALookalikeProfileAndSaysSo(t *testing.T) {
	base := t.TempDir()
	writeProfileDir(t, filepath.Join(base, "default"), 32)
	lookalike := filepath.Join(base, "mine.quarantine-1700000009")
	writeProfileDir(t, lookalike, 16)

	removed, err := ReclaimQuarantinedProfiles(base, "")
	if err != nil {
		t.Fatalf("ReclaimQuarantinedProfiles() error = %v", err)
	}

	if len(removed) != 1 || removed[0].Path != lookalike {
		t.Fatalf("removed = %v, want exactly the lookalike", removalNames(removed))
	}
	if _, err := os.Stat(filepath.Join(base, "default", "State")); err != nil {
		t.Errorf("the live profile beside the lookalike was removed: %v", err)
	}
	if IsQuarantinedProfileDir("mine") {
		t.Error("a plain profile name reads as quarantined, so the predicate no longer bounds this")
	}
}

// Caller text never becomes a path component: `only` is matched against directories that
// have already been enumerated and already passed the predicate. A path is refused by
// name so the caller is told what is wrong, but even without that refusal there is no
// join for a traversal to escape through.
func TestReclaimRefusesAPathInsteadOfAProfileName(t *testing.T) {
	base := reclaimFixture(t)
	outside := filepath.Join(t.TempDir(), "victim")
	writeProfileDir(t, outside, 8)

	for _, name := range []string{
		"../victim",
		"../../etc",
		outside,
		"default.quarantine-1700000001/Default",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ReclaimQuarantinedProfiles(base, name)
			if err == nil {
				t.Fatalf("reclaim accepted %q as a profile name", name)
			}
			// Refused AS A PATH, not as a name that happened to match nothing. Both
			// end in a refusal, so without this the message branch is unpinned: a
			// caller that sent a path would be told its profile does not exist and go
			// looking for the wrong thing.
			if !strings.Contains(err.Error(), "is a path, not a quarantined profile name") {
				t.Errorf("refusal of %q is %q, want it to say the input is a path", name, err)
			}
			if _, err := ReclaimableQuarantinedProfiles(base, name); err == nil {
				t.Errorf("the dry run accepted %q as a profile name", name)
			}
		})
	}

	if _, err := os.Stat(filepath.Join(outside, "State")); err != nil {
		t.Errorf("a directory outside the profiles base was removed: %v", err)
	}
	if len(dirNames(t, base)) != 5 {
		t.Errorf("a refused request still changed the tree: %v", dirNames(t, base))
	}
}

func TestReclaimNarrowedToOneProfileLeavesTheOtherQuarantinesAlone(t *testing.T) {
	base := reclaimFixture(t)

	removed, err := ReclaimQuarantinedProfiles(base, "work.quarantine-1700000003")
	if err != nil {
		t.Fatalf("ReclaimQuarantinedProfiles() error = %v", err)
	}

	if len(removed) != 1 || filepath.Base(removed[0].Path) != "work.quarantine-1700000003" {
		t.Fatalf("removed = %v, want only the named quarantine", removalNames(removed))
	}
	if got := len(dirNames(t, base)); got != 4 {
		t.Errorf("tree has %d entries, want 4: %v", got, dirNames(t, base))
	}
}

func TestReclaimRefusesToNameALiveProfile(t *testing.T) {
	base := reclaimFixture(t)

	if _, err := ReclaimQuarantinedProfiles(base, "default"); err == nil {
		t.Fatal("reclaim accepted a live profile name")
	}
	if _, err := os.Stat(filepath.Join(base, "default", "State")); err != nil {
		t.Errorf("the live profile named in a refused request was removed: %v", err)
	}
}

// The deleter re-applies the predicate rather than trusting the selection it is handed,
// because the two callers select differently and only this check is common to both. Test
// it directly: hand it a live profile the way a future third caller might.
func TestTheQuarantineDeleterRefusesAnythingNotQuarantined(t *testing.T) {
	base := t.TempDir()
	live := filepath.Join(base, "default")
	writeProfileDir(t, live, 32)
	quarantined := quarantinePathAt(live, 1700000001)
	writeProfileDir(t, quarantined, 64)

	removed := removeQuarantinedProfiles([]string{live, quarantined})

	if len(removed) != 1 || removed[0].Path != quarantined {
		t.Fatalf("removed = %v, want only the quarantined directory", removalNames(removed))
	}
	if _, err := os.Stat(filepath.Join(live, "State")); err != nil {
		t.Fatalf("the deleter removed a live profile handed to it directly: %v", err)
	}
}

// directoryRemover is one file that removes a directory with os.RemoveAll, the functions
// in it that do so, and — the answer this census exists to record — whether it applies
// the quarantine predicate to what it removes.
//
// "One owner of the pattern and two owners of the deletion" is the state that lets a
// later change delete the wrong thing with every test still green, so the count of
// quarantine-gated deleters is asserted rather than described. The scope is the whole
// module, not the packages that look relevant: a new deleter under the profiles base is
// exactly the thing a hand-listed scope cannot see.
type directoryRemover struct {
	path              string
	funcs             []string
	underProfilesBase bool
	quarantineGated   bool
	route             string
	why               string
}

var directoryRemovers = []directoryRemover{
	{
		path:              "internal/bridge/profile_lock.go",
		funcs:             []string{"removeQuarantinedProfiles"},
		underProfilesBase: true,
		quarantineGated:   true,
		route:             "POST /profiles/prune with confirm; and quarantineCorruptedProfile via PruneQuarantinedProfiles",
		why:               "the one deleter that applies IsQuarantinedProfileDir to the path it is about to remove, whichever caller selected it",
	},
	{
		path:              "internal/profiles/profiles_crud.go",
		funcs:             []string{"remove", "Import", "resetProfileDir"},
		underProfilesBase: true,
		quarantineGated:   false,
		route:             "DELETE /profiles/{id} (remove, via Delete and ForceDelete), POST /profiles/import (Import), POST /profiles/{id}/reset (resetProfileDir)",
		why:               "remove is id-addressed and in-use-gated but NOT eligibility-gated, so it can reach a live profile by design — that is why a reclaim must never route through it; Import only rolls back the destination it just created; resetProfileDir removes named caches inside one profile, never the profile directory",
	},
	{
		path:              "internal/orchestrator/instance_stop.go",
		funcs:             []string{"cleanupStoppedProfile", "markStopped"},
		underProfilesBase: true,
		quarantineGated:   false,
		route:             "instance stop",
		why:               "cleanupStoppedProfile is gated on the instance- name prefix, which quarantine never produces; markStopped removes the instance state dir, which is not a profile",
	},
	{
		path:              "internal/bridge/bridge_lifecycle.go",
		funcs:             []string{"Cleanup", "RestartBrowser"},
		underProfilesBase: false,
		route:             "bridge shutdown and browser restart",
		why:               "removes b.tempProfileDir, a temp-dir path this process created; never under the configured profiles base",
	},
	{
		path:              "internal/bridge/cleanup.go",
		funcs:             []string{"CleanupOrphanedChromeProcesses"},
		underProfilesBase: false,
		route:             "startup",
		why:               "sweeps os.TempDir() for pinchtab-profile-* leftovers; reads the temp dir, not the profiles base",
	},
	{
		path:              "internal/handlers/upload.go",
		funcs:             []string{"HandleUpload", "CleanupStaleUploads"},
		underProfilesBase: false,
		route:             "POST /upload and its stale sweep",
		why:               "scoped to the upload staging dir under the state dir",
	},
	{
		path:              "internal/handlers/record.go",
		funcs:             []string{"cleanup", "stop"},
		underProfilesBase: false,
		route:             "recording stop",
		why:               "removes the recorder's own temp dir",
	},
	{
		path:              "internal/browsers/chrome/cdp_probe.go",
		funcs:             []string{"LaunchAndProbe"},
		underProfilesBase: false,
		route:             "browser capability probe",
		why:               "removes the throwaway user-data-dir the probe itself created under os.MkdirTemp",
	},
	{
		path:              "internal/testbrowser/profile.go",
		funcs:             []string{"removeProfileDir"},
		underProfilesBase: false,
		route:             "test harness only",
		why:               "removes a t.TempDir-rooted browser profile built for one test",
	},
	{
		path:              "internal/cli/output/advisory.go",
		funcs:             []string{"ResetAdvisories"},
		underProfilesBase: false,
		route:             "ResetAdvisories only — no ordinary command reaches it; it is the deliberate 'let me hear the advice again' path and the tests' fresh-install helper",
		why:               "removes the advisories marker directory under the CLI state dir, never a browser profile, so the quarantine predicate does not apply. The join cannot widen: clistate.Dir() answers a non-empty path in every branch (XDG_STATE_HOME, the home fallback, os.TempDir), so the removal is always .../pinchtab/advisories and never the state dir itself or the filesystem root",
	},
	{
		path:              "tests/tools/runner/internal/e2e/provider.go",
		funcs:             []string{"cleanup", "prepareCloakOverrides", "prepareGhostChromeOverrides"},
		underProfilesBase: false,
		route:             "e2e runner",
		why:               "removes provider override dirs the runner created under os.MkdirTemp",
	},
}

func TestExactlyOneDeleterAppliesTheQuarantinePredicate(t *testing.T) {
	gated := []string{}
	for _, remover := range directoryRemovers {
		if remover.why == "" || remover.route == "" {
			t.Errorf("%s is censused with no reason and no route, so it records nothing", remover.path)
		}
		if remover.quarantineGated {
			gated = append(gated, remover.path)
			if !remover.underProfilesBase {
				t.Errorf("%s is marked quarantine-gated but not under the profiles base", remover.path)
			}
		}
	}
	if len(gated) != 1 {
		t.Fatalf("quarantine-gated deleters = %v, want exactly one owner of the deletion", gated)
	}
}

// The module-wide half: a new file that removes a directory anywhere in the module must
// be classified here before it can land. Scope is derived from the tree rather than from
// the table, so the table cannot narrow what is checked.
func TestEveryDirectoryRemoverInTheModuleIsCensused(t *testing.T) {
	censused := map[string]bool{}
	for _, remover := range directoryRemovers {
		censused[remover.path] = false
	}

	for _, file := range srccensus.Tree(t, "../..", 200) {
		if !strings.Contains(file.Text, "os.RemoveAll(") {
			continue
		}
		if _, listed := censused[file.Name]; !listed {
			t.Errorf("%s removes a directory with os.RemoveAll and is not censused — add it to directoryRemovers saying whether it applies the quarantine predicate and why that is safe", file.Name)
			continue
		}
		censused[file.Name] = true
	}

	for path, found := range censused {
		if !found {
			t.Errorf("%s is censused as a directory remover but no longer calls os.RemoveAll; drop the entry rather than leaving a stale reason", path)
		}
	}
}

// The function-level half, for the packages that CAN reach the profiles base: every
// os.RemoveAll site is attributed to a function the census names. A file-level table
// would keep passing while a new function inside a listed file grew its own deletion.
func TestEveryRemoverFunctionUnderTheProfilesBaseIsNamed(t *testing.T) {
	for _, pkg := range []struct{ dir, prefix string }{
		{".", "internal/bridge/"},
		{"../profiles", "internal/profiles/"},
		{"../orchestrator", "internal/orchestrator/"},
	} {
		loaded := srccensus.Load(t, pkg.dir, 3)
		sites := loaded.Calls(t, "os.RemoveAll")
		for _, site := range sites {
			named := false
			for _, remover := range directoryRemovers {
				if remover.path != pkg.prefix+site.File {
					continue
				}
				for _, fn := range remover.funcs {
					if fn == site.Func {
						named = true
					}
				}
			}
			if !named {
				t.Errorf("%s removes a directory at %s and is not named in the census entry for %s", site.Func, site, pkg.prefix+site.File)
			}
		}
	}
}
