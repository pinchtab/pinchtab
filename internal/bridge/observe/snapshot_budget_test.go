package observe

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/srccensus"
	"gopkg.in/yaml.v3"
)

// minObserveSourceFiles is the vacuity floor: the package already has more non-test
// sources than this, so a scan that silently stops seeing most of it fails instead of
// passing for the wrong reason.
const minObserveSourceFiles = 8

// budgetProbeNodes is a page's worth of realistic interactive nodes: the mix of roles,
// names, values and states a real form or app shell produces, at varying depth. The budget
// is only meaningful against content like this — a fixture of empty nodes would let almost
// any estimator look correct.
func budgetProbeNodes(count int) []A11yNode {
	roles := []string{"button", "textbox", "link", "checkbox", "combobox", "heading", "listitem", "tab"}
	names := []string{
		"Submit order",
		"Email address",
		"Continue to payment",
		"Remember this device",
		"Country or region",
		"Shipping information",
		"Standard delivery, 3-5 business days",
		"Account",
	}
	values := []string{"", "user@example.com", "", "", "United Kingdom", "", "", ""}

	nodes := make([]A11yNode, 0, count)
	for i := 0; i < count; i++ {
		nodes = append(nodes, A11yNode{
			Ref:      fmt.Sprintf("e%d", i+1),
			Role:     roles[i%len(roles)],
			Name:     names[i%len(names)],
			Value:    values[i%len(values)],
			Depth:    i % 5,
			Focused:  i%9 == 0,
			Disabled: i%11 == 0,
			Checked:  []CheckedState{"", CheckedTrue, CheckedFalse, CheckedMixed}[i%4],
		})
	}
	return nodes
}

// renderedTokens is the ground truth the card measured against: the tokens the caller
// actually receives for the nodes the truncator kept, counted from the bytes the matching
// formatter emits.
func renderedTokens(t *testing.T, nodes []A11yNode, format string) int {
	t.Helper()
	switch format {
	case "compact":
		return estimateTokens(len(FormatSnapshotCompact(nodes)))
	case "text":
		return estimateTokens(len(FormatSnapshotText(nodes)))
	case "yaml":
		out, err := yaml.Marshal(nodes)
		if err != nil {
			t.Fatalf("marshal yaml: %v", err)
		}
		return estimateTokens(len(out))
	default:
		out, err := json.Marshal(nodes)
		if err != nil {
			t.Fatalf("marshal json: %v", err)
		}
		return estimateTokens(len(out))
	}
}

// The contract maxTokens now carries, stated as the two properties that make a budget a
// budget rather than a hint:
//
//  1. the output never exceeds it, and
//  2. nothing is left on the table — no node that was dropped would have fit.
//
// Property 2 is what a percentage floor cannot express. A shortfall is only legitimate
// when the dropped node was too big for the remainder, and that is checkable exactly, so
// there is no tolerance here to quietly widen until the current constants pass.
//
// It used to be phrased against nodes[:len(kept)+1], which reads the property off the
// assumption that kept is a PREFIX of nodes. TruncateToTokens selects by budget tier
// now, so kept is a subset and that index names an unrelated node — the check would
// have gone on passing while testing nothing. Stated over the dropped set instead, it
// is both correct for a subset and strictly stronger: every omission has to be
// justified, not just the one at the boundary.
func TestTruncateToTokensDeliversTheBudgetItWasAsked(t *testing.T) {
	nodes := budgetProbeNodes(40)

	for _, format := range []string{"compact", "text", "json", "yaml"} {
		for _, budget := range []int{100, 300, 1000} {
			t.Run(fmt.Sprintf("%s/%d", format, budget), func(t *testing.T) {
				kept, truncated := TruncateToTokens(nodes, budget, format)
				actual := renderedTokens(t, kept, format)

				if actual > budget {
					t.Errorf("%s budget=%d kept %d/%d nodes but renders ~%d tokens (%+d%%): the budget is a ceiling and it was exceeded",
						format, budget, len(kept), len(nodes), actual, percentOff(actual, budget))
				}

				if truncated != (len(kept) < len(nodes)) {
					t.Errorf("%s budget=%d reported truncated=%v while keeping %d of %d nodes",
						format, budget, truncated, len(kept), len(nodes))
				}

				if truncated {
					keptRefs := make(map[string]bool, len(kept))
					for _, n := range kept {
						keptRefs[n.Ref] = true
					}
					cost := nodeCost(format)
					for _, n := range nodes {
						if keptRefs[n.Ref] {
							continue
						}
						if withNode := renderedTokens(t, append(append([]A11yNode{}, kept...), n), format); withNode <= budget {
							t.Errorf("%s budget=%d kept %d/%d nodes (~%d tokens) but dropped %s:%s, which costs ~%d tokens and would have fit in ~%d: the caller was short-changed",
								format, budget, len(kept), len(nodes), actual, n.Ref, n.Role, estimateTokens(cost(n)), withNode)
							break
						}
					}
				}
			})
		}
	}
}

// The band the two properties above actually produce, recorded so the number in
// docs/reference/snapshot.md is measured rather than asserted. A format whose nodes are
// large relative to the budget lands lower in the band; that is the one-node gap, not
// slack in the estimator.
func TestBudgetDeliveryBandIsWorthDocumenting(t *testing.T) {
	nodes := budgetProbeNodes(40)
	worst := 100
	for _, format := range []string{"compact", "text", "json", "yaml"} {
		for _, budget := range []int{100, 300, 1000} {
			kept, truncated := TruncateToTokens(nodes, budget, format)
			got := renderedTokens(t, kept, format) * 100 / budget
			if !truncated {
				// Everything fit, so the budget was never the constraint and the
				// percentage says nothing about the allocator.
				t.Logf("%-8s budget=%4d -> kept all %d nodes (~%d%% of budget, not constrained)", format, budget, len(kept), got)
				continue
			}
			t.Logf("%-8s budget=%4d -> kept %2d/%d, ~%d%% of budget", format, budget, len(kept), len(nodes), got)
			if got < worst {
				worst = got
			}
		}
	}
	if worst < 80 {
		t.Errorf("worst delivery was %d%% of budget; docs claim at least 80%%", worst)
	}
}

func percentOff(actual, budget int) int {
	if budget == 0 {
		return 0
	}
	return (actual - budget) * 100 / budget
}

// The estimate cannot drift from the output because it IS the output: what the truncator
// charges for a node must be the bytes that node costs once rendered. Change a formatter
// without changing the charge and this reds — which is the link the old estimator did not
// have, and the reason maxTokens had already drifted from all four formats.
func TestWhatTheTruncatorChargesIsWhatTheFormatterEmits(t *testing.T) {
	nodes := budgetProbeNodes(40)

	for _, tc := range []struct {
		format string
		// slack is the array or document framing that belongs to the collection rather
		// than to any node: the bytes no per-node charge can be attributed.
		slack int
	}{
		{format: "compact", slack: 0},
		{format: "text", slack: 0},
		{format: "yaml", slack: 0},
		{format: "json", slack: 1},
	} {
		t.Run(tc.format, func(t *testing.T) {
			cost := nodeCost(tc.format)
			charged := 0
			for _, n := range nodes {
				charged += cost(n)
			}
			emitted := renderedBytes(t, nodes, tc.format)

			if diff := emitted - charged; diff != tc.slack {
				t.Errorf("%s: charged %d bytes for %d nodes but the formatter emits %d (off by %d, want %d): the cost model has drifted from the layout",
					tc.format, charged, len(nodes), emitted, diff, tc.slack)
			}
		})
	}
}

// A node's charge must also track its own shape, not just aggregate correctly: an empty
// name, a value, a focus marker and a checked state each change the bytes emitted, and a
// charge that ignores any of them under-bills that node shape specifically.
func TestEveryRenderedFieldIsChargedForCompactAndText(t *testing.T) {
	base := A11yNode{Ref: "e1", Role: "button"}
	for _, tc := range []struct {
		name string
		node A11yNode
	}{
		{name: "bare", node: base},
		{name: "named", node: A11yNode{Ref: "e1", Role: "button", Name: "Submit order"}},
		{name: "valued", node: A11yNode{Ref: "e1", Role: "textbox", Value: "user@example.com"}},
		{name: "focused", node: A11yNode{Ref: "e1", Role: "button", Focused: true}},
		{name: "disabled", node: A11yNode{Ref: "e1", Role: "button", Disabled: true}},
		{name: "hidden", node: A11yNode{Ref: "e1", Role: "button", Hidden: true}},
		{name: "checked", node: A11yNode{Ref: "e1", Role: "checkbox", Checked: CheckedTrue}},
		{name: "unchecked", node: A11yNode{Ref: "e1", Role: "checkbox", Checked: CheckedFalse}},
		{name: "mixed", node: A11yNode{Ref: "e1", Role: "checkbox", Checked: CheckedMixed}},
		{name: "deep", node: A11yNode{Ref: "e1", Role: "listitem", Depth: 4}},
		{name: "everything", node: A11yNode{Ref: "e12", Role: "checkbox", Name: "Remember me", Value: "on", Depth: 3, Focused: true, Disabled: true, Hidden: true, Checked: CheckedMixed}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			one := []A11yNode{tc.node}
			if got, want := nodeCost("compact")(tc.node), len(FormatSnapshotCompact(one)); got != want {
				t.Errorf("compact charged %d bytes, formatter emits %d", got, want)
			}
			if got, want := nodeCost("text")(tc.node), len(FormatSnapshotText(one)); got != want {
				t.Errorf("text charged %d bytes, formatter emits %d", got, want)
			}
		})
	}
}

func renderedBytes(t *testing.T, nodes []A11yNode, format string) int {
	t.Helper()
	switch format {
	case "compact":
		return len(FormatSnapshotCompact(nodes))
	case "text":
		return len(FormatSnapshotText(nodes))
	case "yaml":
		out, err := yaml.Marshal(nodes)
		if err != nil {
			t.Fatalf("marshal yaml: %v", err)
		}
		return len(out)
	default:
		out, err := json.Marshal(nodes)
		if err != nil {
			t.Fatalf("marshal json: %v", err)
		}
		return len(out)
	}
}

// The behavioural guards above compare a charge to an output, which cannot catch a FIFTH
// renderer that emits the layout inline and is never costed at all. This one can: the
// distinctive layout literals belong to appendNode and nowhere else.
func TestNodeLayoutIsWrittenDownExactlyOnce(t *testing.T) {
	pkg := srccensus.Load(t, ".", minObserveSourceFiles)

	for _, literal := range []string{` val="`, ` [focused]`, ` [disabled]`} {
		t.Run(strings.TrimSpace(literal), func(t *testing.T) {
			var sites []string
			for _, name := range pkg.Files() {
				body, err := os.ReadFile(filepath.Join(pkg.Dir(), name))
				if err != nil {
					t.Fatal(err)
				}
				for i, line := range strings.Split(string(body), "\n") {
					if strings.Contains(line, literal) {
						sites = append(sites, fmt.Sprintf("%s:%d: %s", name, i+1, strings.TrimSpace(line)))
					}
				}
			}
			if len(sites) != 1 {
				t.Errorf("%q appears %d times, want exactly 1 — a node's layout written down twice is what let maxTokens drift from every format; emit it through appendNode, or if the layout genuinely moved, re-point this guard at its new single home rather than deleting it:\n%s",
					literal, len(sites), strings.Join(sites, "\n"))
			}
		})
	}
}

// pageShapedNodes is the layout the prefix allocator could not serve: a long run of
// copy, then the controls. Terms above a signup form, a nav blurb above a search box,
// results above pagination — the actionable part of a page is routinely the last part
// of it.
func pageShapedNodes(prose int) []A11yNode {
	nodes := make([]A11yNode, 0, prose+6)
	for i := 0; i < prose; i++ {
		nodes = append(nodes,
			A11yNode{Ref: fmt.Sprintf("h%d", i), Role: "heading", Name: fmt.Sprintf("Section %d", i)},
			A11yNode{Ref: fmt.Sprintf("p%d", i), Role: "paragraph", Name: "Body copy the agent cannot act on, at some length."},
		)
	}
	return append(nodes,
		A11yNode{Ref: "c0", Role: "textbox", Name: "Email"},
		A11yNode{Ref: "c1", Role: "textbox", Name: "Password"},
		A11yNode{Ref: "c2", Role: "combobox", Name: "Plan"},
		A11yNode{Ref: "c3", Role: "checkbox", Name: "I accept the terms"},
		A11yNode{Ref: "c4", Role: "button", Name: "Create account"},
		A11yNode{Ref: "c5", Role: "link", Name: "Need help?"},
	)
}

// prefixToTokens is the allocator this replaced: the longest run of nodes from the
// start that fits. Kept here so the improvement is measured against the real previous
// behaviour rather than described.
func prefixToTokens(nodes []A11yNode, maxTokens int, format string) []A11yNode {
	cost := nodeCost(format)
	used := 0
	for i, n := range nodes {
		used += cost(n)
		if estimateTokens(used) > maxTokens {
			return nodes[:i]
		}
	}
	return nodes
}

func actionableCount(nodes []A11yNode) int {
	n := 0
	for _, node := range nodes {
		if InteractiveRoles[node.Role] {
			n++
		}
	}
	return n
}

// The property the budget exists to protect: a snapshot the agent can still act on.
//
// A reply that spends its whole budget on prose is not a smaller answer, it is a
// useless one — there is nothing in it to click. That is what the prefix allocator
// returned on this shape, and on the real 71-node page it was measured against: 0 of 9
// controls at budgets of 120, 200 and 300 tokens.
//
// Two things are asserted, both robust across formats. The allocator is never worse
// than the prefix it replaced, and it never returns an unusable snapshot while a
// control would have fit. How MANY controls survive is a function of the budget and
// the format's per-node cost — yaml spends ~100 tokens on a single node — so that is
// logged rather than asserted.
func TestBudgetKeepsWhatTheAgentCanActOn(t *testing.T) {
	nodes := pageShapedNodes(60)
	total := actionableCount(nodes)

	for _, format := range []string{"compact", "text", "json", "yaml"} {
		for _, budget := range []int{120, 200, 300} {
			t.Run(fmt.Sprintf("%s/%d", format, budget), func(t *testing.T) {
				kept, truncated := TruncateToTokens(nodes, budget, format)
				if !truncated {
					t.Fatalf("budget=%d did not constrain a %d-node page; the test proves nothing", budget, len(nodes))
				}

				got := actionableCount(kept)
				was := actionableCount(prefixToTokens(nodes, budget, format))
				t.Logf("%-8s budget=%3d -> controls %d/%d (prefix allocator: %d/%d)", format, budget, got, total, was, total)

				if got < was {
					t.Errorf("%s budget=%d kept %d controls, fewer than the %d the prefix allocator kept: this must never be a regression",
						format, budget, got, was)
				}

				// Does even one control fit on its own? Then returning none is a
				// snapshot with nothing to act on, which is the whole defect.
				cost := nodeCost(format)
				fits := false
				for _, n := range nodes {
					if InteractiveRoles[n.Role] && estimateTokens(cost(n)) <= budget {
						fits = true
						break
					}
				}
				if fits && got == 0 {
					t.Errorf("%s budget=%d returned %d nodes and not one control, though a control fits in the budget: the reply cannot be acted on",
						format, budget, len(kept))
				}
			})
		}
	}
}

// Selection reorders nothing. The reply has to read like the page, so an agent can
// still reason about what sits above what.
func TestBudgetKeepsDocumentOrder(t *testing.T) {
	nodes := pageShapedNodes(40)
	kept, truncated := TruncateToTokens(nodes, 200, "compact")
	if !truncated {
		t.Fatal("expected the budget to constrain this page")
	}

	position := make(map[string]int, len(nodes))
	for i, n := range nodes {
		position[n.Ref] = i
	}
	for i := 1; i < len(kept); i++ {
		if position[kept[i-1].Ref] >= position[kept[i].Ref] {
			t.Fatalf("kept[%d]=%s came after kept[%d]=%s in the page but before it in the reply",
				i-1, kept[i-1].Ref, i, kept[i].Ref)
		}
	}
}

// A snapshot the budget never constrained must come back exactly as it was: same
// nodes, same order, and truncated=false. Prioritising only ever decides between nodes
// that were going to be dropped anyway.
func TestBudgetLeavesAnUnconstrainedSnapshotAlone(t *testing.T) {
	nodes := pageShapedNodes(5)
	kept, truncated := TruncateToTokens(nodes, 100000, "compact")
	if truncated {
		t.Fatal("reported truncation for a snapshot that fit")
	}
	if len(kept) != len(nodes) {
		t.Fatalf("kept %d of %d nodes although everything fit", len(kept), len(nodes))
	}
	for i := range nodes {
		if kept[i].Ref != nodes[i].Ref {
			t.Fatalf("node %d changed from %s to %s", i, nodes[i].Ref, kept[i].Ref)
		}
	}
}
