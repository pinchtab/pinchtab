package server

import (
	"testing"

	"github.com/pinchtab/pinchtab/internal/bridge"
)

// This closes the gap carried on the card: the profile guard's safety depends on WHICH
// instance states count as holding a directory, and that set lived inside a closure in
// RunDashboard where no test could reach it. RunDashboard binds a listener and launches a
// browser, so it is not drivable — extracting the derivation is what makes it assertable at
// all, and this is the composition the running server actually installs.
func TestProfileInstanceHolderCountsEveryStateThatHoldsTheDirectory(t *testing.T) {
	const profileID = "prof_held"

	for _, tc := range []struct {
		status   bridge.InstanceStatus
		wantHeld bool
		why      string
	}{
		{"running", true, "the obvious case"},
		{"starting", true, "the profile is claimed before the process reports running, so deleting here races the launch"},
		{"stopping", true, "the browser still has the directory open while it winds down; deleting is the same loss as deleting while it runs"},
		{"stopped", false, "nothing holds the directory"},
		{"failed", false, "nothing holds the directory"},
		{"", false, "an unset status must not read as held, or every idle profile becomes undeletable"},
	} {
		t.Run(string(tc.status), func(t *testing.T) {
			instances := []bridge.Instance{{ID: "inst_1", ProfileID: profileID, Status: tc.status}}

			holder, held := profileInstanceHolder(instances, profileID)
			if held != tc.wantHeld {
				t.Errorf("status %q: held = %v, want %v — %s", tc.status, held, tc.wantHeld, tc.why)
			}
			if held && holder != "inst_1" {
				t.Errorf("status %q: holder = %q, want the instance id so the 409 can name it", tc.status, holder)
			}
		})
	}
}

// The match is on ProfileID. bridge.Instance also carries ProfileName, which its own comment
// marks display-only, and matching on that would make the guard miss a rename and collide
// two profiles resolving to one display string.
func TestProfileInstanceHolderMatchesOnIDNotDisplayName(t *testing.T) {
	instances := []bridge.Instance{
		{ID: "inst_other", ProfileID: "prof_other", ProfileName: "default", Status: "running"},
	}

	if holder, held := profileInstanceHolder(instances, "prof_default"); held {
		t.Errorf("a different profile with the same display name reads as the holder (%q); the guard must key on the id the delete route resolves", holder)
	}
	if _, held := profileInstanceHolder(instances, "prof_other"); !held {
		t.Error("the instance's own profile id is not matched, so the lookup finds nothing at all")
	}
}

// An empty instance list is the fresh-server case and must not refuse everything.
func TestProfileInstanceHolderReportsNothingHeldWithNoInstances(t *testing.T) {
	if holder, held := profileInstanceHolder(nil, "prof_any"); held {
		t.Errorf("held = true with no instances running (holder %q)", holder)
	}
}
