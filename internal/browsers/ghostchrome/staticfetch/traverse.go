package staticfetch

import (
	"fmt"
	"strings"

	"github.com/gost-dom/browser/dom"
	"github.com/gost-dom/browser/html"
	"github.com/pinchtab/pinchtab/internal/browserops"
)

// walkDOM produces the snapshot node list for a subtree. It allocates the
// result slice once and delegates to walkDOMInto, which appends in place
// instead of returning per-level slices (no recursive append fan-out).
func (l *Browser) walkDOM(tab *liteTab, node dom.Node, filter string, depth int) []browserops.SnapshotNode {
	var nodes []browserops.SnapshotNode
	l.walkDOMInto(&nodes, tab, node, filter, depth)
	return nodes
}

// walkDOMInto appends snapshot nodes for node (and its descendants) onto acc.
// Visit order is parent-before-children, siblings in document order. A
// non-interactive element under the "interactive" filter is skipped (no ref
// assigned) but its children are still visited at the same depth — matching the
// original recursive implementation exactly.
func (l *Browser) walkDOMInto(acc *[]browserops.SnapshotNode, tab *liteTab, node dom.Node, filter string, depth int) {
	el, isElement := node.(dom.Element)
	if !isElement {
		return
	}

	tag := strings.ToLower(el.TagName())

	if tag == "script" || tag == "style" || tag == "noscript" || tag == "link" || tag == "meta" {
		return
	}

	role := getRole(el)
	name := getAccessibleName(el)
	interactive := isInteractive(el)

	if filter == "interactive" && !interactive {
		for child := node.FirstChild(); child != nil; child = child.NextSibling() {
			l.walkDOMInto(acc, tab, child, filter, depth)
		}
		return
	}

	ref := fmt.Sprintf("e%d", len(tab.refMap))
	tab.refMap[ref] = el

	sn := browserops.SnapshotNode{
		Ref:         ref,
		Role:        role,
		Name:        name,
		Tag:         tag,
		Interactive: interactive,
		Depth:       depth,
	}

	if input, ok := el.(html.HTMLInputElement); ok {
		sn.Value = input.Value()
	}

	*acc = append(*acc, sn)

	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		l.walkDOMInto(acc, tab, child, filter, depth+1)
	}
}
