package output

import (
	"fmt"
	"sort"
	"testing"

	"github.com/pinchtab/pinchtab/internal/srccensus"
)

// Every hint is one of two things, and which one decides how often it may print.
// An advisory describes a steady state the caller may have chosen: its wording is
// the same on every invocation, its remedy is a decision rather than a reaction,
// and repeating it trains the reader to skip hints. An occurrence reports what
// just happened on this call, so the second one is news too and must print.
//
// The classification is not a comment: a site's class here is the function it is
// required to call, so a site that changes its mind about which it is fails until
// someone records the decision, and a new site fails until it makes one.
const (
	advisory   = "output.Advisory"
	occurrence = "output.Hint"
)

// hintCallers are the packages that print hints. The floors are vacuity guards:
// a census that silently stopped seeing most of a package would otherwise pass.
var hintCallers = map[string]int{
	"../actions":            40,
	"../../../cmd/pinchtab": 45,
}

var hintSites = map[string]struct {
	class string
	calls int
	why   string
}{
	"../actions/actions_navigate.go:Navigate":                          {advisory, 1, "no agent session: a steady state, and on a bridge one the caller cannot leave"},
	"../actions/actions_navigate.go:reportFallbackNewTab":              {occurrence, 2, "this navigation's tab was gone and a new one was opened"},
	"../actions/actions_element.go:printActionResult":                  {occurrence, 2, "this action's refs went stale, or this submit is still pending"},
	"../actions/actions_element.go:printStaleRefsHint":                 {occurrence, 1, "this action invalidated the snapshot the caller is holding"},
	"../actions/actions_capture.go:Capture":                            {occurrence, 1, "this page tripped the IDPI guard"},
	"../actions/actions_text.go:Text":                                  {occurrence, 1, "readability collapsed on this page"},
	"../actions/actions_tabs.go:TabHandoff":                            {occurrence, 1, "the reason this handoff carried"},
	"../actions/actions_tabs.go:TabHandoffStatus":                      {occurrence, 1, "the reason this handoff carried"},
	"../../../cmd/pinchtab/cmd_cli_runtime.go:resolveCLIBase":          {advisory, 2, "the caller's own --server/PINCHTAB_SERVER is redundant: their setting, not an event"},
	"../../../cmd/pinchtab/cmd_config_actions.go:hintRestartIfRunning": {occurrence, 1, "this edit needs a restart to reach the running server"},
	"../../../cmd/pinchtab/cmd_session.go:printSessionCreated":         {occurrence, 1, "the id of the session just created"},
	"../../../cmd/pinchtab/cmd_session.go:init":                        {occurrence, 2, "attached to a failure that exits"},
	"../../../cmd/pinchtab/cmd_session.go:exitSessionUnavailable":      {occurrence, 2, "attached to a failure that exits"},
}

func TestEveryHintCallSiteIsClassified(t *testing.T) {
	found := map[string]struct {
		class string
		calls int
	}{}
	for dir, floor := range hintCallers {
		pkg := srccensus.Load(t, dir, floor)
		for _, class := range []string{advisory, occurrence} {
			for _, site := range pkg.CallsAllowingNone(class) {
				key := fmt.Sprintf("%s/%s:%s", dir, site.File, site.Func)
				entry := found[key]
				if entry.class != "" && entry.class != class {
					t.Errorf("%s prints both an advisory and an occurrence hint; split it so each line's class is readable at the call site", key)
				}
				entry.class = class
				entry.calls++
				found[key] = entry
			}
		}
	}
	if len(found) < len(hintSites) {
		t.Fatalf("the census found %d hint sites for %d recorded; it is matching almost nothing and would pass vacuously", len(found), len(hintSites))
	}

	for _, key := range sortedSiteKeys(found) {
		site := found[key]
		want, recorded := hintSites[key]
		if !recorded {
			t.Errorf("%s prints a hint and is not classified; record it here as %s or %s with the reason, since how often it may print follows from that", key, advisory, occurrence)
			continue
		}
		if want.class != site.class {
			t.Errorf("%s is recorded as %s but calls %s", key, want.class, site.class)
		}
		if want.calls != site.calls {
			t.Errorf("%s makes %d hint call(s), recorded %d; a new line in a classified function still has to choose its class", key, site.calls, want.calls)
		}
	}
	for _, key := range sortedRecordedKeys() {
		if _, ok := found[key]; !ok {
			t.Errorf("%s is recorded here and prints no hint any more; drop the entry rather than leaving the census guarding one site fewer than it claims", key)
		}
	}
}

func sortedSiteKeys(found map[string]struct {
	class string
	calls int
}) []string {
	keys := make([]string, 0, len(found))
	for key := range found {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedRecordedKeys() []string {
	keys := make([]string, 0, len(hintSites))
	for key := range hintSites {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
