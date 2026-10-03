package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/bridge/observe"
)

func readSkillFile(t *testing.T, parts ...string) string {
	t.Helper()
	path := filepath.Join(append([]string{"..", "..", "skills", "pinchtab"}, parts...)...)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read %s, so this guard would cover nothing: %v", path, err)
	}
	return string(raw)
}

// documentsNamespace reports whether the CLI reference gives a namespace its own
// section. Headings carry the command's argument form ("### `pinchtab nav <url>`"), so
// the name is matched with either a closing backtick or a following space — a mention
// inside another command's prose does not count.
func documentsNamespace(reference, name string) bool {
	return strings.Contains(reference, namespaceHeading(name)) ||
		strings.Contains(reference, "### `pinchtab "+name+" ")
}

// namespaceHeading is what a reader has to add, so it is also what a failure message must
// print — spelled once here rather than escaped into the message.
func namespaceHeading(name string) string {
	return "### `pinchtab " + name + "`"
}

// namespaceSection returns one namespace's section alone, so an assertion about a command
// cannot be satisfied by an unrelated paragraph elsewhere in the file.
func namespaceSection(t *testing.T, reference, name string) string {
	t.Helper()

	start := strings.Index(reference, namespaceHeading(name))
	if start < 0 {
		start = strings.Index(reference, "### `pinchtab "+name+" ")
	}
	if start < 0 {
		t.Fatalf("the %s section is gone, so this guard is pinning nothing", name)
	}
	rest := reference[start+len("### "):]

	end := len(rest)
	for _, next := range []string{"\n### ", "\n## "} {
		if at := strings.Index(rest, next); at >= 0 && at < end {
			end = at
		}
	}
	return rest[:end]
}

// The defect this guard is derived from is a CROSS-SURFACE asymmetry, not incompleteness:
// an agent reading the skill could discover cookie setting over MCP and over raw HTTP,
// but not the CLI verb built for it — and for reading was sent to `state`, the command
// whose help no longer claims cookies as its job. The CLI is the token-cheap surface the
// skill steers agents toward, so a namespace present on the other two surfaces and absent
// here sends the agent to curl.
//
// Deliberately NOT "every namespace must be documented": measured against the tree, the
// CLI reference is a curated subset (roughly a third of the leaf verbs have no section),
// so that rule would assert a contract this repo does not have and would red for a dozen
// commands no card was filed about. Cross-surface parity is the narrower claim that is
// actually true and actually the defect.
//
// knownCrossSurfaceGaps is checked in BOTH directions, so it cannot go quiet: a NEW gap
// reds, and an entry whose namespace has since been documented reds as stale. Every entry
// carries what a reader needs to close it.
func TestNoNamespaceIsDocumentedOnAnotherSurfaceButMissingFromTheCLIReference(t *testing.T) {
	reference := readSkillFile(t, "references", "commands.md")
	mcp := readSkillFile(t, "references", "mcp.md")
	api := readSkillFile(t, "references", "api.md")

	knownCrossSurfaceGaps := map[string]string{
		"keyboard": "key-sequence verbs; documented on the MCP surface only — same class as the cookies gap, unfixed",
		"dialog":   "JS-dialog handling; documented on the MCP surface only — same class as the cookies gap, unfixed",
		"state":    "SKILL.md carries the state rules and the sensitivity warning inline, so the namespace is reachable there; a commands.md section is still owed",
	}

	namespaces := 0
	for _, cmd := range browserRootCommands() {
		if len(cmd.Commands()) == 0 {
			continue // a leaf verb is its own name; a namespace hides verbs an agent cannot guess
		}
		name := cmd.Name()
		namespaces++
		if documentsNamespace(reference, name) {
			if reason, stale := knownCrossSurfaceGaps[name]; stale {
				t.Errorf("`pinchtab %s` IS documented in the CLI reference now, but is still recorded as a known gap (%q); drop the entry so the guard keeps meaning what it says", name, reason)
			}
			continue
		}
		onAnotherSurface := strings.Contains(mcp, "pinchtab_"+name) || strings.Contains(api, "/"+name)
		if !onAnotherSurface {
			continue
		}
		if reason, ok := knownCrossSurfaceGaps[name]; ok {
			if reason == "" {
				t.Errorf("`pinchtab %s` is recorded as a known gap with no reason; every entry is a decision", name)
			}
			continue
		}
		t.Errorf("`pinchtab %s` is documented on the MCP or HTTP surface but has no %q section in skills/pinchtab/references/commands.md, so an agent can discover the operation everywhere except on the surface the skill steers it toward. Add the section in the file's house style, or record it in knownCrossSurfaceGaps with the reason",
			name, namespaceHeading(name))
	}
	if namespaces < 8 {
		t.Fatalf("only %d namespaces reached this guard; the command list is not being read and it would pass vacuously", namespaces)
	}
}

// The routing half of the same defect: the skill sent an agent to `state` to read a
// cookie, which is precisely the misfiling the cookies namespace removed from the CLI
// help. Both surfaces must name the command that actually reads one.
func TestTheSkillRoutesCookieReadsAtTheCookiesCommand(t *testing.T) {
	for _, parts := range [][]string{{"SKILL.md"}, {"references", "commands.md"}} {
		name := filepath.Join(parts...)
		text := readSkillFile(t, parts...)
		if !strings.Contains(text, "pinchtab cookies get") {
			t.Errorf("%s never names `pinchtab cookies get`, so an agent reading it cannot find the command that reads a cookie", name)
		}
	}
}

// The browser-wide blast radius of `cookies clear` is pinned on the CLI side, and a doc
// that omits it teaches an agent to reach for a command that will log the user out of
// every site. The reference must not contradict the help it mirrors.
func TestTheCookiesReferenceStatesTheBrowserWideClearEffect(t *testing.T) {
	reference := readSkillFile(t, "references", "commands.md")

	// storage is pinned DIRECTLY rather than by the cross-surface rule above, and the
	// reason is measured: it has no MCP tool and no /storage HTTP route, so nothing on
	// another surface documents it and the derived rule cannot reach it. Removing its
	// section leaves that guard green — these assertions are what notice.
	cookies := namespaceSection(t, reference, "cookies")
	storage := namespaceSection(t, reference, "storage")

	for _, want := range []string{"all origins", "cannot be scoped"} {
		if !strings.Contains(cookies, want) {
			t.Errorf("the cookies section never says %q about `cookies clear`; its own --help does, and the doc must not read as narrower than the command is", want)
		}
	}

	for _, want := range []string{"pinchtab storage delete token", "bare `storage delete` is refused", "only verb that wipes a store", "pinchtab storage clear"} {
		if !strings.Contains(storage, want) {
			t.Errorf("the storage section never says %q; `storage delete` needs a key and `storage clear` is the one wipe verb, and the doc must teach both", want)
		}
	}
	for _, stale := range []string{"no `--key`", "omit it and the whole store is cleared", "pinchtab storage delete  "} {
		if strings.Contains(storage, stale) {
			t.Errorf("the storage section still says %q, teaching a bare `storage delete` wipe the CLI now refuses", stale)
		}
	}
}

// Either anchor alone finds a copy, so a one-word edit to the phrase keeps the file in
// the set and the failure names it instead of silently shrinking the set.
func compactDiffDocCopies(t *testing.T) []string {
	t.Helper()
	out, err := exec.Command("git", "-C", filepath.Join("..", ".."), "grep", "-l", "-F",
		"-e", "[~] changed and removed refs", "-e", "[+] added", "-e", "`[+]` = added", "--", ":!*.go").Output()
	if err != nil {
		t.Fatalf("git grep for compact-diff doc copies: %v", err)
	}
	return strings.Fields(string(out))
}

func TestCompactDiffDocCopiesMatchTheSummary(t *testing.T) {
	copies := compactDiffDocCopies(t)
	const floor = 10
	if len(copies) < floor {
		t.Fatalf("found %d compact-diff doc copies %v, want at least %d", len(copies), copies, floor)
	}
	for _, required := range []string{"skills/pinchtab/SKILL.md", "skills/pinchtab/references/agent-optimization.md", "skills/pinchtab/references/api.md", "skills/pinchtab/references/commands.md"} {
		if !slices.Contains(copies, required) {
			t.Errorf("%s no longer describes the compact diff; discovered %v", required, copies)
		}
	}
	for _, path := range copies {
		raw, err := os.ReadFile(filepath.Join("..", "..", path))
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if !strings.Contains(string(raw), observe.CompactDiffSummary) {
			t.Errorf("%s drifted from observe.CompactDiffSummary %q", path, observe.CompactDiffSummary)
		}
	}
}
