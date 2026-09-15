package observe

import (
	"encoding/json"
	"strings"

	"gopkg.in/yaml.v3"
)

// All THREE states are annotated, not just the checked one: absent means the control
// has no checkedness and false means it is off, so rendering only [checked] would make
// an unchecked option look like a node the field does not apply to.
var checkedAnnotations = map[CheckedState]string{
	CheckedTrue:  " [checked]",
	CheckedFalse: " [unchecked]",
	CheckedMixed: " [mixed]",
}

// [~] is NOT available here: the compact diff renderer already uses it for
// "changed", and the same token meaning two things on one line is unreadable.
var checkedCompactAnnotations = map[CheckedState]string{
	CheckedTrue:  " [x]",
	CheckedFalse: " [ ]",
	CheckedMixed: " [/]",
}

// nodeStyle is everything that differs between the text and compact layouts. The field
// ORDER and the quoting are not in here because they do not differ — they live once, in
// appendNode. A style that could reorder fields would be a second layout model, which is
// the thing this file used to have three of.
type nodeStyle struct {
	indent   string
	refSep   string
	focused  string
	disabled string
	hidden   string
	checked  map[CheckedState]string
}

var textNodeStyle = nodeStyle{
	indent:   "  ",
	refSep:   " ",
	focused:  " [focused]",
	disabled: " [disabled]",
	hidden:   " [hidden]",
	checked:  checkedAnnotations,
}

var compactNodeStyle = nodeStyle{
	refSep:   ":",
	focused:  " *",
	disabled: " -",
	hidden:   " [hidden]",
	checked:  checkedCompactAnnotations,
}

// appendNode writes one node exactly as the caller's format emits it. It is the single
// source of truth for what a node costs, because the truncator charges the caller for
// what this writes rather than for a second hand-maintained model of it.
func appendNode(b *strings.Builder, n A11yNode, style nodeStyle, marker string) {
	for i := 0; i < n.Depth; i++ {
		b.WriteString(style.indent)
	}
	b.WriteString(n.Ref)
	b.WriteString(style.refSep)
	b.WriteString(n.Role)
	if n.Name != "" {
		b.WriteString(` "`)
		b.WriteString(n.Name)
		b.WriteByte('"')
	}
	if n.Value != "" {
		b.WriteString(` val="`)
		b.WriteString(n.Value)
		b.WriteByte('"')
	}
	if n.Focused {
		b.WriteString(style.focused)
	}
	if annotation, ok := style.checked[n.Checked]; ok {
		b.WriteString(annotation)
	}
	if n.Disabled {
		b.WriteString(style.disabled)
	}
	if n.Hidden {
		b.WriteString(style.hidden)
	}
	b.WriteString(marker)
	b.WriteByte('\n')
}

func formatNodes(nodes []A11yNode, style nodeStyle, marker func(A11yNode) string) string {
	var b strings.Builder
	for _, n := range nodes {
		suffix := ""
		if marker != nil {
			suffix = marker(n)
		}
		appendNode(&b, n, style, suffix)
	}
	return b.String()
}

func FormatSnapshotText(nodes []A11yNode) string {
	return formatNodes(nodes, textNodeStyle, nil)
}

func FormatSnapshotCompact(nodes []A11yNode) string {
	return formatNodes(nodes, compactNodeStyle, nil)
}

// FormatSnapshotCompactDiff outputs all current nodes in compact format with
// change markers: [+] for added, [~] for changed. Removed refs are listed at
// the end as [- ref]. This gives agents the full valid ref set plus change info.
func FormatSnapshotCompactDiff(nodes []A11yNode, added, changed, removed []A11yNode) string {
	addedRefs := make(map[string]bool, len(added))
	for _, n := range added {
		addedRefs[n.Ref] = true
	}
	changedRefs := make(map[string]bool, len(changed))
	for _, n := range changed {
		changedRefs[n.Ref] = true
	}

	var b strings.Builder
	b.WriteString(formatNodes(nodes, compactNodeStyle, func(n A11yNode) string {
		switch {
		case addedRefs[n.Ref]:
			return " [+]"
		case changedRefs[n.Ref]:
			return " [~]"
		default:
			return ""
		}
	}))

	if len(removed) > 0 {
		b.WriteString("# removed:")
		for _, n := range removed {
			b.WriteByte(' ')
			b.WriteString(n.Ref)
		}
		b.WriteByte('\n')
	}

	return b.String()
}

// estimateTokens is the one place bytes become tokens. Four bytes per token is the
// approximation the whole budget rests on, so it lives here rather than being spelled
// out per format — a format that divides by its own constant is a second model of the
// same thing, which is how the old estimator drifted.
func estimateTokens(bytes int) int {
	return bytes / 4
}

// nodeCost reports what one node costs the caller in the format it asked for. Every
// branch MEASURES: the text layouts by rendering through appendNode, the structured ones
// by marshalling the node with the same encoder the handler uses. Nothing here models a
// layout, so nothing here can drift away from one.
func nodeCost(format string) func(A11yNode) int {
	switch format {
	case "compact":
		return renderedNodeCost(compactNodeStyle)
	case "text":
		return renderedNodeCost(textNodeStyle)
	case "yaml":
		return func(n A11yNode) int {
			out, err := yaml.Marshal([]A11yNode{n})
			if err != nil {
				return 0
			}
			return len(out)
		}
	default:
		return func(n A11yNode) int {
			out, err := json.Marshal(n)
			if err != nil {
				return 0
			}
			// +1 for the comma or closing bracket this node brings with it once it is
			// an element of the nodes array rather than a value on its own.
			return len(out) + 1
		}
	}
}

func renderedNodeCost(style nodeStyle) func(A11yNode) int {
	var b strings.Builder
	return func(n A11yNode) int {
		b.Reset()
		appendNode(&b, n, style, "")
		return b.Len()
	}
}

// Budget tiers, ordered by what an agent loses when the node is dropped.
//
// An agent reads a snapshot to decide what to DO next, and the only nodes it can
// act on are the ones carrying an interactive role. Static text is orientation;
// a button is the whole reason the snapshot was requested.
const (
	tierActionable = iota // roles the agent can click, type into or select
	tierContext           // headings, media, table cells: where it is on the page
	tierRemainder         // prose and containers
)

func budgetTier(n A11yNode) int {
	switch {
	case InteractiveRoles[n.Role]:
		return tierActionable
	case ContextRoles[n.Role]:
		return tierContext
	default:
		return tierRemainder
	}
}

// TruncateToTokens fits nodes into maxTokens, spending the budget on what the page
// can be acted on with before what it can be read with.
//
// It used to keep the longest PREFIX that fit, which spends the budget in document
// order. Document order is not value order. On any page whose controls sit below its
// copy — a form under terms, a search box under a nav blurb, pagination under results
// — the prefix is entirely prose and the reply contains no refs at all. Measured on a
// 71-node page with 9 controls at the end, budgets of 120, 200 and 300 tokens each
// returned 0 of the 9: a snapshot the agent cannot act on, produced precisely when the
// budget made every token count.
//
// Nodes are selected by tier and emitted in document order, so the reply reads like
// the page and not like a ranking. Both properties the prefix form guaranteed are
// kept, and the second one strengthens:
//
//  1. the output never exceeds the budget, and
//  2. nothing is left on the table — every node that was dropped was too big for
//     what remained, which now holds over the whole set rather than just the next
//     node along.
//
// An unconstrained snapshot is returned untouched, so this only ever decides between
// nodes that were going to be lost anyway.
func TruncateToTokens(nodes []A11yNode, maxTokens int, format string) ([]A11yNode, bool) {
	cost := nodeCost(format)

	costs := make([]int, len(nodes))
	total := 0
	for i, n := range nodes {
		costs[i] = cost(n)
		total += costs[i]
	}
	if estimateTokens(total) <= maxTokens {
		return nodes, false
	}

	keep := make([]bool, len(nodes))
	bytesUsed := 0
	for tier := tierActionable; tier <= tierRemainder; tier++ {
		for i, n := range nodes {
			if keep[i] || budgetTier(n) != tier {
				continue
			}
			// Skip rather than stop: one oversized node must not evict every
			// smaller one behind it, which is how a single long label used to
			// cost an agent the rest of the form.
			if estimateTokens(bytesUsed+costs[i]) > maxTokens {
				continue
			}
			keep[i] = true
			bytesUsed += costs[i]
		}
	}

	kept := make([]A11yNode, 0, len(nodes))
	for i, n := range nodes {
		if keep[i] {
			kept = append(kept, n)
		}
	}
	return kept, true
}
