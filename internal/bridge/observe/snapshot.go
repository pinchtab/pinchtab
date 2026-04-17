package observe

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/chromedp"
)

type A11yNode struct {
	Ref            string `json:"ref"`
	Role           string `json:"role"`
	Name           string `json:"name"`
	Depth          int    `json:"depth"`
	Value          string `json:"value,omitempty"`
	Disabled       bool   `json:"disabled,omitempty"`
	Focused        bool   `json:"focused,omitempty"`
	Hidden         bool   `json:"hidden,omitempty"`
	NodeID         int64  `json:"nodeId,omitempty"`
	FrameID        string `json:"frameId,omitempty"`
	FrameURL       string `json:"frameUrl,omitempty"`
	FrameName      string `json:"frameName,omitempty"`
	ChildFrameID   string `json:"childFrameId,omitempty"`
	ChildFrameURL  string `json:"childFrameUrl,omitempty"`
	ChildFrameName string `json:"childFrameName,omitempty"`
}

type RawAXNode struct {
	NodeID           string      `json:"nodeId"`
	Ignored          bool        `json:"ignored"`
	Role             *RawAXValue `json:"role"`
	Name             *RawAXValue `json:"name"`
	Value            *RawAXValue `json:"value"`
	Properties       []RawAXProp `json:"properties"`
	ChildIDs         []string    `json:"childIds"`
	BackendDOMNodeID int64       `json:"backendDOMNodeId"`
	FrameID          string      `json:"-"`
	FrameURL         string      `json:"-"`
	FrameName        string      `json:"-"`
	FrameOwnerNodeID int64       `json:"-"`
}

type RawAXTreeResponse struct {
	Nodes []RawAXNode `json:"nodes"`
}

type RawFrame struct {
	ID   string `json:"id"`
	URL  string `json:"url,omitempty"`
	Name string `json:"name,omitempty"`
}

type RawFrameTree struct {
	Frame       RawFrame       `json:"frame"`
	ChildFrames []RawFrameTree `json:"childFrames"`
}

// FrameIDs returns every frame id in a frame tree, including descendants.
func FrameIDs(tree RawFrameTree) []string {
	ids := make([]string, 0, 1+len(tree.ChildFrames))
	var walk func(RawFrameTree)
	walk = func(t RawFrameTree) {
		if t.Frame.ID != "" {
			ids = append(ids, t.Frame.ID)
		}
		for _, child := range t.ChildFrames {
			walk(child)
		}
	}
	walk(tree)
	return ids
}

// FrameMap returns frame metadata keyed by frame id.
func FrameMap(tree RawFrameTree) map[string]RawFrame {
	frames := make(map[string]RawFrame, 1+len(tree.ChildFrames))
	var walk func(RawFrameTree)
	walk = func(t RawFrameTree) {
		if t.Frame.ID != "" {
			frames[t.Frame.ID] = t.Frame
		}
		for _, child := range t.ChildFrames {
			walk(child)
		}
	}
	walk(tree)
	return frames
}

// FrameOwnerMap returns iframe owner backend node IDs keyed by child frame id.
func FrameOwnerMap(ctx context.Context, tree RawFrameTree) map[string]int64 {
	owners := make(map[string]int64, len(tree.ChildFrames))
	var walk func(RawFrameTree)
	walk = func(t RawFrameTree) {
		for _, child := range t.ChildFrames {
			if child.Frame.ID != "" {
				backendNodeID, _, err := dom.GetFrameOwner(cdp.FrameID(child.Frame.ID)).Do(ctx)
				if err == nil && backendNodeID != 0 {
					owners[child.Frame.ID] = int64(backendNodeID)
				}
			}
			walk(child)
		}
	}
	walk(tree)
	return owners
}

func FetchFrameTree(ctx context.Context) (RawFrameTree, error) {
	var frameTreeResult json.RawMessage
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "Page.getFrameTree", nil, &frameTreeResult)
	})); err != nil {
		return RawFrameTree{}, err
	}

	var frameResp struct {
		FrameTree RawFrameTree `json:"frameTree"`
	}
	if err := json.Unmarshal(frameTreeResult, &frameResp); err != nil {
		return RawFrameTree{}, err
	}
	return frameResp.FrameTree, nil
}

// FetchAXTree returns the merged accessibility tree for the current page and any child frames.
func FetchAXTree(ctx context.Context) ([]RawAXNode, error) {
	frameTree, err := FetchFrameTree(ctx)
	if err != nil {
		return fetchAXTreeForFrame(ctx, "")
	}

	frameMap := FrameMap(frameTree)
	ownerMap := FrameOwnerMap(ctx, frameTree)
	ids := FrameIDs(frameTree)
	if len(ids) == 0 {
		return fetchAXTreeForFrame(ctx, "")
	}

	merged := make([]RawAXNode, 0, 256)
	seen := make(map[string]bool, 256)
	for _, id := range ids {
		nodes, err := fetchAXTreeForFrame(ctx, id)
		if err != nil {
			continue
		}
		frameMeta := frameMap[id]
		for _, n := range nodes {
			n.FrameID = id
			n.FrameURL = frameMeta.URL
			n.FrameName = frameMeta.Name
			n.FrameOwnerNodeID = ownerMap[id]
			key := n.NodeID
			if key == "" {
				key = fmt.Sprintf("backend:%d:%s:%s", n.BackendDOMNodeID, n.Role.String(), n.Name.String())
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			merged = append(merged, n)
		}
	}
	if len(merged) > 0 {
		return merged, nil
	}
	return fetchAXTreeForFrame(ctx, "")
}

func fetchAXTreeForFrame(ctx context.Context, frameID string) ([]RawAXNode, error) {
	params := map[string]any{
		// pierce:true makes the accessibility tree traverse Shadow DOM boundaries,
		// exposing content inside shadow roots (issue #381).
		"pierce": true,
	}
	if frameID != "" {
		params["frameId"] = frameID
	}
	var rawResult json.RawMessage
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "Accessibility.getFullAXTree", params, &rawResult)
	})); err != nil {
		return nil, err
	}
	var treeResp RawAXTreeResponse
	if err := json.Unmarshal(rawResult, &treeResp); err != nil {
		return nil, err
	}
	return treeResp.Nodes, nil
}

type RawAXValue struct {
	Type  string          `json:"type"`
	Value json.RawMessage `json:"value"`
}

type RawAXProp struct {
	Name  string      `json:"name"`
	Value *RawAXValue `json:"value"`
}

func (v *RawAXValue) String() string {
	if v == nil || v.Value == nil {
		return ""
	}
	var s string
	if err := json.Unmarshal(v.Value, &s); err == nil {
		return s
	}
	return strings.Trim(string(v.Value), `"`)
}

var InteractiveRoles = map[string]bool{
	"button": true, "link": true, "textbox": true, "searchbox": true,
	"combobox": true, "listbox": true, "option": true, "checkbox": true,
	"radio": true, "switch": true, "slider": true, "spinbutton": true,
	"menuitem": true, "menuitemcheckbox": true, "menuitemradio": true,
	"tab": true, "treeitem": true, "iframe": true, "Iframe": true,
}

const FilterInteractive = "interactive"

// isAXNodeHidden checks whether a raw accessibility node has properties
// indicating it is hidden from the user (aria-hidden, display:none, etc.).
// Chrome's accessibility tree marks these via the "hidden" boolean property.
func isAXNodeHidden(n RawAXNode) bool {
	for _, prop := range n.Properties {
		if prop.Name == "hidden" && prop.Value.String() == "true" {
			return true
		}
	}
	return false
}

func BuildSnapshot(nodes []RawAXNode, filter string, maxDepth int) ([]A11yNode, map[string]int64) {
	nodeByID := make(map[string]RawAXNode, len(nodes))
	parentMap := make(map[string]string)
	childMap := make(map[string][]string, len(nodes))
	backendToAX := make(map[int64]string, len(nodes))
	frameRoots := make(map[string][]string, 4)
	frameOwners := make(map[string]int64, 4)
	ownerToChildFrame := make(map[int64]RawFrame, 4)
	frameOrder := make([]string, 0, 4)
	seenFrames := make(map[string]bool, 4)

	for _, n := range nodes {
		nodeByID[n.NodeID] = n
		childMap[n.NodeID] = append(childMap[n.NodeID], n.ChildIDs...)
		if n.BackendDOMNodeID != 0 {
			backendToAX[n.BackendDOMNodeID] = n.NodeID
		}
		if !seenFrames[n.FrameID] {
			frameOrder = append(frameOrder, n.FrameID)
			seenFrames[n.FrameID] = true
		}
		if n.FrameOwnerNodeID != 0 && frameOwners[n.FrameID] == 0 {
			frameOwners[n.FrameID] = n.FrameOwnerNodeID
			ownerToChildFrame[n.FrameOwnerNodeID] = RawFrame{
				ID:   n.FrameID,
				URL:  n.FrameURL,
				Name: n.FrameName,
			}
		}
		for _, childID := range n.ChildIDs {
			parentMap[childID] = n.NodeID
		}
	}
	maxAncestorWalk := max(len(parentMap)+1, 1)

	// Build a set of AX node IDs that are hidden, including inherited hidden
	// status from ancestors. A child of a hidden node is also hidden.
	hiddenNodes := make(map[string]bool, len(nodes)/4)
	for _, n := range nodes {
		if isAXNodeHidden(n) {
			hiddenNodes[n.NodeID] = true
		}
	}
	// Propagate: if a parent is hidden, all descendants inherit hidden status.
	isHidden := func(nodeID string) bool {
		cur := nodeID
		for range maxAncestorWalk {
			if hiddenNodes[cur] {
				return true
			}
			p, ok := parentMap[cur]
			if !ok {
				break
			}
			cur = p
		}
		return false
	}

	for _, n := range nodes {
		parentID, ok := parentMap[n.NodeID]
		if !ok {
			frameRoots[n.FrameID] = append(frameRoots[n.FrameID], n.NodeID)
			continue
		}
		parentNode, ok := nodeByID[parentID]
		if !ok || parentNode.FrameID != n.FrameID {
			frameRoots[n.FrameID] = append(frameRoots[n.FrameID], n.NodeID)
		}
	}

	rootFrameID := ""
	for _, frameID := range frameOrder {
		if frameOwners[frameID] == 0 {
			rootFrameID = frameID
			break
		}
	}
	if rootFrameID == "" && len(frameOrder) > 0 {
		rootFrameID = frameOrder[0]
	}

	topRoots := make([]string, 0, len(nodes))
	frameChildRoots := make(map[string][]string)
	for _, frameID := range frameOrder {
		roots := frameRoots[frameID]
		if len(roots) == 0 {
			continue
		}
		ownerBackendID := frameOwners[frameID]
		ownerAXID := backendToAX[ownerBackendID]
		if frameID == rootFrameID || ownerAXID == "" {
			topRoots = append(topRoots, roots...)
			continue
		}
		frameChildRoots[ownerAXID] = append(frameChildRoots[ownerAXID], roots...)
	}
	if len(topRoots) == 0 {
		for _, n := range nodes {
			topRoots = append(topRoots, n.NodeID)
		}
	}

	flat := make([]A11yNode, 0)
	refs := make(map[string]int64)
	refID := 0
	appendNode := func(n RawAXNode, depth int) {
		role := n.Role.String()
		name := n.Name.String()
		ref := fmt.Sprintf("e%d", refID)
		entry := A11yNode{
			Ref:       ref,
			Role:      role,
			Name:      name,
			Depth:     depth,
			FrameID:   n.FrameID,
			FrameURL:  n.FrameURL,
			FrameName: n.FrameName,
		}
		if childFrame, ok := ownerToChildFrame[n.BackendDOMNodeID]; ok {
			entry.ChildFrameID = childFrame.ID
			entry.ChildFrameURL = childFrame.URL
			entry.ChildFrameName = childFrame.Name
		}

		if v := n.Value.String(); v != "" {
			entry.Value = v
		}
		if n.BackendDOMNodeID != 0 {
			entry.NodeID = n.BackendDOMNodeID
			refs[ref] = n.BackendDOMNodeID
		}

		for _, prop := range n.Properties {
			if prop.Name == "disabled" && prop.Value.String() == "true" {
				entry.Disabled = true
			}
			if prop.Name == "focused" && prop.Value.String() == "true" {
				entry.Focused = true
			}
		}

		// Tag nodes that are visually hidden but still present in the a11y tree
		// (e.g. display:none with explicit ARIA attributes). This lets consumers
		// (AI agents) know the content is not visible to the user.
		if isHidden(n.NodeID) {
			entry.Hidden = true
		}

		flat = append(flat, entry)
		refID++
	}

	isSkippableNode := func(n RawAXNode) bool {
		role := n.Role.String()
		name := n.Name.String()
		if n.Ignored {
			return true
		}
		if role == "none" || role == "generic" || role == "InlineTextBox" {
			return true
		}
		if name == "" && role == "StaticText" {
			return true
		}
		return false
	}

	isFlattenableFrameRoot := func(n RawAXNode) bool {
		role := n.Role.String()
		return n.FrameOwnerNodeID != 0 && (role == "WebArea" || role == "RootWebArea")
	}

	visited := make(map[string]bool, len(nodes))
	var visitFrameRoot func(string, int)
	var visit func(string, int)

	visit = func(nodeID string, depth int) {
		if nodeID == "" || visited[nodeID] {
			return
		}
		n, ok := nodeByID[nodeID]
		if !ok {
			return
		}
		visited[nodeID] = true
		if maxDepth >= 0 && depth > maxDepth {
			return
		}

		if !isSkippableNode(n) && (filter != FilterInteractive || InteractiveRoles[n.Role.String()]) {
			appendNode(n, depth)
		}

		for _, childID := range childMap[nodeID] {
			visit(childID, depth+1)
		}
		for _, frameRootID := range frameChildRoots[nodeID] {
			visitFrameRoot(frameRootID, depth+1)
		}
	}

	visitFrameRoot = func(nodeID string, depth int) {
		n, ok := nodeByID[nodeID]
		if !ok {
			return
		}
		if isFlattenableFrameRoot(n) {
			for _, childID := range childMap[nodeID] {
				visit(childID, depth)
			}
			return
		}
		visit(nodeID, depth)
	}

	for _, rootID := range topRoots {
		visit(rootID, 0)
	}

	return flat, refs
}

func FilterSubtree(nodes []RawAXNode, scopeBackendID int64) []RawAXNode {
	scopeAXID := ""
	for _, n := range nodes {
		if n.BackendDOMNodeID == scopeBackendID {
			scopeAXID = n.NodeID
			break
		}
	}
	if scopeAXID == "" {
		return nodes
	}

	childMap := make(map[string][]string, len(nodes))
	for _, n := range nodes {
		childMap[n.NodeID] = append(childMap[n.NodeID], n.ChildIDs...)
	}

	include := make(map[string]bool)
	include[scopeAXID] = true
	queue := []string{scopeAXID}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, cid := range childMap[cur] {
			if !include[cid] {
				include[cid] = true
				queue = append(queue, cid)
			}
		}
	}

	result := make([]RawAXNode, 0, len(include))
	for _, n := range nodes {
		if include[n.NodeID] {
			result = append(result, n)
		}
	}
	return result
}

func DiffSnapshot(prev, curr []A11yNode) (added, changed, removed []A11yNode) {
	prevMap := make(map[string]A11yNode, len(prev))
	for _, n := range prev {
		key := fmt.Sprintf("%s:%s:%d", n.Role, n.Name, n.NodeID)
		prevMap[key] = n
	}

	currMap := make(map[string]bool, len(curr))
	for _, n := range curr {
		key := fmt.Sprintf("%s:%s:%d", n.Role, n.Name, n.NodeID)
		currMap[key] = true
		old, existed := prevMap[key]
		if !existed {
			added = append(added, n)
		} else if old.Value != n.Value || old.Focused != n.Focused || old.Disabled != n.Disabled {
			changed = append(changed, n)
		}
	}

	for _, n := range prev {
		key := fmt.Sprintf("%s:%s:%d", n.Role, n.Name, n.NodeID)
		if !currMap[key] {
			removed = append(removed, n)
		}
	}

	return
}
