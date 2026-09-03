package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const clickDocPath = "docs/reference/click.md"

// clickContractSkills are the two shipped skills that describe the click contract.
// They are the surface agents actually read, and they drifted from the docs for a
// release because only the docs were ever listed in a contract-changing card's
// footprint. This guard is the answer to that: the skills are checked AGAINST the
// docs, so the next contract change reds here instead of shipping stale guidance.
// It reads the working tree, so it proves the shipped source and nothing about a
// copy installed in an agent home; that half is covered by the version stamp the
// npm package writes at staging and by `pinchtab skill status` (npm/scripts/sync-skills.js).
var clickContractSkills = []string{"skills/pinchtab/SKILL.md", "skills/pinchtab-mcp/SKILL.md"}

// clickContractBanned are statements the contract no longer produces. An agent
// taught to handle a 409 from a click is taught a branch that can never be taken.
var clickContractBanned = []string{"409", "navigation_changed", "unexpected page navigation"}

func readRepoFile(t *testing.T, rel string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(body)
}

// navigationSignalKeys reads the wire names of the navigation outcome from their
// DECLARATION SITE in internal/bridge — the one place that decides what a navigating
// action reports. Reading them rather than listing them here is what makes this a
// drift guard with no list to remember: a signal added to the contract must reach the
// docs and both skills or this reds, and one renamed cannot silently stop being
// checked.
func navigationSignalKeys(t *testing.T) []string {
	t.Helper()

	src := readRepoFile(t, "internal/bridge/action_request.go")
	pattern := regexp.MustCompile(`(?m)^\s*Result[A-Za-z]+\s*=\s*"([^"]+)"`)
	matches := pattern.FindAllStringSubmatch(src, -1)

	var keys []string
	for _, m := range matches {
		keys = append(keys, m[1])
	}
	if len(keys) < 4 {
		t.Fatalf("found %d navigation signal constants (%v) in internal/bridge/action_request.go; the contract publishes at least navigated, url, previousUrl and refsStale, so this guard is reading the wrong place and would pass vacuously", len(keys), keys)
	}
	return keys
}

// documentedClickResult parses the JSON example in docs/reference/click.md. A
// documented example is the one artefact no other test covers, so it is driven
// through a parser here rather than trusted to look right.
func documentedClickResult(t *testing.T, doc string) map[string]any {
	t.Helper()

	const fence = "```json"
	start := strings.Index(doc, fence)
	if start < 0 {
		t.Fatalf("%s has no json example of a navigating click; this guard reads the contract from that block and would pass vacuously without it", clickDocPath)
	}
	block := doc[start+len(fence):]
	end := strings.Index(block, "```")
	if end < 0 {
		t.Fatalf("%s has an unterminated json block", clickDocPath)
	}

	var payload struct {
		Result map[string]any `json:"result"`
	}
	if err := json.Unmarshal([]byte(block[:end]), &payload); err != nil {
		t.Fatalf("the json example in %s does not parse, so the contract cannot be read from it: %v", clickDocPath, err)
	}
	return payload.Result
}

// The skills must name every signal the documented response carries. An agent that
// cannot learn from the skill that the answer is already in the response either
// re-snapshots blindly or keeps using dead refs after a click it did not know
// navigated.
func TestTheSkillsNameEverySignalTheDocumentedClickResultCarries(t *testing.T) {
	keys := navigationSignalKeys(t)

	// The docs first: they are the description this guard measures the skills
	// against, so a signal missing from them would let the skills omit it too.
	documented := documentedClickResult(t, readRepoFile(t, clickDocPath))
	for _, key := range keys {
		if _, ok := documented[key]; !ok {
			t.Errorf("%s does not show %q in its navigating-click example, which internal/bridge reports", clickDocPath, key)
		}
	}

	for _, skill := range clickContractSkills {
		body := readRepoFile(t, skill)
		for _, key := range keys {
			if !strings.Contains(body, key) {
				t.Errorf("%s never mentions %q, which a navigating click returns — the skill is the surface an agent reads, so a signal missing here is a signal it will not use",
					skill, key)
			}
		}
	}
}

// The status the contract no longer produces, and the framing the docs explicitly
// correct. Both are checked against the docs rather than against a remembered rule:
// the banned strings must be absent from the docs too, so this cannot outlive the
// contract it guards.
func TestTheSkillsDoNotTeachTheContractThatWasReplaced(t *testing.T) {
	doc := readRepoFile(t, clickDocPath)

	for _, banned := range clickContractBanned {
		if strings.Contains(doc, banned) {
			t.Fatalf("%s itself contains %q, so this ban no longer describes the contract — re-derive it before trusting the skill assertions below", clickDocPath, banned)
		}
	}

	const correction = "not permission to navigate"
	if !strings.Contains(doc, correction) {
		t.Fatalf("%s no longer states that the wait flag is %q; this guard mirrors the docs, so it cannot pin a correction the docs have dropped", clickDocPath, correction)
	}

	for _, skill := range clickContractSkills {
		body := readRepoFile(t, skill)
		for _, banned := range clickContractBanned {
			if strings.Contains(body, banned) {
				t.Errorf("%s still mentions %q; the contract no longer produces it, so this teaches a branch that can never be taken", skill, banned)
			}
		}
		if !strings.Contains(body, correction) {
			t.Errorf("%s does not say the wait flag is %q — without it the skill reads as 'add the flag because the click navigates', which is the inverted framing %s exists to correct",
				skill, correction, clickDocPath)
		}
	}
}
