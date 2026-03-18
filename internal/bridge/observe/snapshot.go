package observe

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chromedp/chromedp"
)

type A11yNode struct {
	Ref      string `json:"ref"`
	Role     string `json:"role"`
	Name     string `json:"name"`
	Depth    int    `json:"depth"`
	Value    string `json:"value,omitempty"`
	Disabled bool   `json:"disabled,omitempty"`
	Focused  bool   `json:"focused,omitempty"`
	NodeID   int64  `json:"nodeId,omitempty"`
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
}

type RawAXTreeResponse struct {
	Nodes []RawAXNode `json:"nodes"`
}

type RawFrameTree struct {
	Frame struct {
		ID string `json:"id"`
	} `json:"frame"`
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

// FetchAXTree returns the merged accessibility tree for the current page and any child frames.
func FetchAXTree(ctx context.Context) ([]RawAXNode, error) {
	var frameTreeResult json.RawMessage
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return chromedp.FromContext(ctx).Target.Execute(ctx, "Page.getFrameTree", nil, &frameTreeResult)
	})); err != nil {
		return fetchAXTreeForFrame(ctx, "")
	}

	var frameResp struct {
		FrameTree RawFrameTree `json:"frameTree"`
	}
	if err := json.Unmarshal(frameTreeResult, &frameResp); err != nil {
		return fetchAXTreeForFrame(ctx, "")
	}

	ids := FrameIDs(frameResp.FrameTree)
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
		for _, n := range nodes {
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
	params := map[string]any{}
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
	"tab": true, "treeitem": true,
}

const FilterInteractive = "interactive"

func BuildSnapshot(nodes []RawAXNode, filter string, maxDepth int) ([]A11yNode, map[string]int64) {
	parentMap := make(map[string]string)
	for _, n := range nodes {
		for _, childID := range n.ChildIDs {
			parentMap[childID] = n.NodeID
		}
	}
	depthOf := func(nodeID string) int {
		d := 0
		cur := nodeID
		for {
			p, ok := parentMap[cur]
			if !ok {
				break
			}
			d++
			cur = p
		}
		return d
	}

	flat := make([]A11yNode, 0)
	refs := make(map[string]int64)
	refID := 0

	for _, n := range nodes {
		if n.Ignored {
			continue
		}

		role := n.Role.String()
		name := n.Name.String()

		if role == "none" || role == "generic" || role == "InlineTextBox" {
			continue
		}
		if name == "" && role == "StaticText" {
			continue
		}

		depth := depthOf(n.NodeID)
		if maxDepth >= 0 && depth > maxDepth {
			continue
		}
		if filter == FilterInteractive && !InteractiveRoles[role] {
			continue
		}

		ref := fmt.Sprintf("e%d", refID)
		entry := A11yNode{
			Ref:   ref,
			Role:  role,
			Name:  name,
			Depth: depth,
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

		flat = append(flat, entry)
		refID++
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
