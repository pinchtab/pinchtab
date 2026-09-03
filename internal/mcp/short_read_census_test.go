package mcp

import (
	"sort"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/srccensus"
)

// One rule, module-wide: a short read must never become a successful body. Every
// capped read is classified here, because the discriminator is not whether the
// call writes limit+1 — it is whether the truncated value is then consumed as if
// it were whole. This client capped at exactly the limit and handed a model a
// truncated base64 image as a success; the activity middleware capped the INBOUND
// body and substituted the peek for it, so an oversize action reached the handler
// mangled and PinchTab blamed the client for the JSON it had cut.
//
// The census is the record of the two permanent exclusions as well as the guard,
// so a later sweep meets their reasons where it would look rather than re-opening
// them from the outside.
const (
	refusesOverTheCap  = "reads limit+1 and refuses above it"
	peeksWithoutTaking = "peeks a prefix and leaves the stream whole for its consumer"
	unreachableCap     = "the cap cannot be reached by anything that passed the guard in front of it"
	loudOnTruncation   = "a truncated value cannot parse, so the request is refused rather than accepted"
)

var cappedReads = map[string]string{
	"internal/mcp/client.go":               refusesOverTheCap,
	"internal/handlers/download_fetch.go":  refusesOverTheCap,
	"internal/orchestrator/tab_extract.go": refusesOverTheCap,
	"internal/bridge/runtime/cdp_url.go":   refusesOverTheCap,
	"internal/activity/context.go":         peeksWithoutTaking,
	// Permanent: isSmallJSON gates the read on Content-Length >= 0 && <= 64 KB
	// against the same 64 KB cap, so the limit is unreachable. That is a property
	// of the input, not a promise about future behaviour.
	"internal/proxy/proxy.go": unreachableCap,
	// Loud but misattributed, and left as it is: a truncated config PUT does not
	// parse, so it answers 400 rather than writing a partial config. The only cost
	// is that bad_config_json blames the client for a truncation PinchTab performed.
	"internal/dashboard/config_api.go": loudOnTruncation,
}

func TestEveryCappedReadIsClassified(t *testing.T) {
	var found []string
	for _, file := range srccensus.Tree(t, "../..", 200) {
		if strings.Contains(file.Text, "io.LimitReader(") {
			found = append(found, file.Name)
		}
	}
	sort.Strings(found)
	if len(found) < len(cappedReads) {
		t.Fatalf("found %d capped reads for %d classified; the census is matching almost nothing and would pass vacuously", len(found), len(cappedReads))
	}

	for _, name := range found {
		if _, ok := cappedReads[name]; !ok {
			t.Errorf("%s caps a read and is not classified; say which it is — %q, %q, or an exclusion with the reason the truncated value is never consumed as whole",
				name, refusesOverTheCap, peeksWithoutTaking)
		}
	}
	for name := range cappedReads {
		if !containsName(found, name) {
			t.Errorf("%s is classified here and no longer caps a read; drop the entry rather than leaving the census guarding one site fewer than it claims", name)
		}
	}

	// The two sites this card fixed carry the rule's own idiom, so a revert is
	// visible here as well as in the behaviour tests.
	for name, want := range map[string]string{
		"internal/mcp/client.go":       "MaxResponseBytes+1",
		"internal/activity/context.go": "io.MultiReader",
	} {
		if !fileText(t, name).Contains(want) {
			t.Errorf("%s no longer carries %q; the classification above says it refuses or replays, and it does neither", name, want)
		}
	}
}

func containsName(names []string, want string) bool {
	for _, name := range names {
		if name == want {
			return true
		}
	}
	return false
}

type censusText string

func (c censusText) Contains(want string) bool { return strings.Contains(string(c), want) }

func fileText(t *testing.T, name string) censusText {
	t.Helper()
	for _, file := range srccensus.Tree(t, "../..", 200) {
		if file.Name == name {
			return censusText(file.Text)
		}
	}
	t.Fatalf("%s is not in the module source any more", name)
	return ""
}
