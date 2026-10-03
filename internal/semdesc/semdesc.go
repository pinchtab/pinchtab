// Package semdesc converts accessibility snapshot nodes into semantic element
// descriptors. It is the single source of the node-to-descriptor mapping so the
// /find handler and the model-free extractor score against identical inputs and
// cannot drift.
package semdesc

import (
	"strings"

	"github.com/pinchtab/pinchtab/internal/bridge/observe"
	"github.com/pinchtab/semantic"
)

// FromNode builds a descriptor from a single node's direct fields, without the
// cross-node positional/parent/section context that Build computes over the
// whole tree. Used where only the element's own attributes are needed.
func FromNode(node observe.A11yNode) semantic.ElementDescriptor {
	return semantic.ElementDescriptor{
		Ref:         node.Ref,
		Role:        node.Role,
		Name:        node.Name,
		Value:       node.Value,
		Label:       node.Label,
		Placeholder: node.Placeholder,
		Alt:         node.Alt,
		Title:       node.Title,
		TestID:      node.TestID,
		Text:        node.Text,
		Tag:         node.Tag,
		Interactive: observe.InteractiveRoles[node.Role],
	}
}

// Build maps a flat pre-order node list to descriptors, computing tree context
// (parent, sibling index/count, nearest section, LabelledBy) from Depth.
func Build(nodes []observe.A11yNode) []semantic.ElementDescriptor {
	descs := make([]semantic.ElementDescriptor, len(nodes))
	if len(nodes) == 0 {
		return descs
	}

	parentIdx := make([]int, len(nodes))
	for i := range parentIdx {
		parentIdx[i] = -1
	}

	stackByDepth := make(map[int]int)
	for i, node := range nodes {
		depth := max(node.Depth, 0)
		if depth > 0 {
			if parent, ok := stackByDepth[depth-1]; ok {
				parentIdx[i] = parent
			}
		}
		stackByDepth[depth] = i
		for knownDepth := range stackByDepth {
			if knownDepth > depth {
				delete(stackByDepth, knownDepth)
			}
		}
	}

	type siblingKey struct {
		parent int
		depth  int
	}
	siblingIndex := make([]int, len(nodes))
	siblingCounts := make(map[siblingKey]int)
	keys := make([]siblingKey, len(nodes))
	for i, node := range nodes {
		key := siblingKey{parent: parentIdx[i], depth: node.Depth}
		keys[i] = key
		siblingIndex[i] = siblingCounts[key]
		siblingCounts[key]++
	}

	for i, node := range nodes {
		desc := semantic.ElementDescriptor{
			Ref:         node.Ref,
			Role:        node.Role,
			Name:        node.Name,
			Value:       node.Value,
			Label:       node.Label,
			Placeholder: node.Placeholder,
			Alt:         node.Alt,
			Title:       node.Title,
			TestID:      node.TestID,
			Text:        node.Text,
			Tag:         node.Tag,
			Interactive: observe.InteractiveRoles[node.Role],
			DocumentIdx: i,
			Positional: semantic.PositionalHints{
				Depth:        node.Depth,
				SiblingIndex: siblingIndex[i],
				SiblingCount: siblingCounts[keys[i]],
			},
		}

		if parent := parentIdx[i]; parent >= 0 {
			desc.Parent = contextLabel(nodes[parent])
			if isLabelledContainer(nodes[parent].Role) {
				desc.Positional.LabelledBy = nodes[parent].Name
			}
		}
		if node.RowContext != "" {
			desc.Positional.LabelledBy = node.RowContext
			desc.Text = strings.TrimSpace(desc.Text + " " + node.RowContext)
		}
		if section := nearestSection(nodes, parentIdx, i); section != "" {
			desc.Section = section
		}

		descs[i] = desc
	}

	return descs
}

func contextLabel(node observe.A11yNode) string {
	role := strings.TrimSpace(node.Role)
	name := strings.TrimSpace(node.Name)
	switch {
	case role != "" && name != "":
		return role + ": " + name
	case name != "":
		return name
	default:
		return role
	}
}

func nearestSection(nodes []observe.A11yNode, parentIdx []int, idx int) string {
	for parent := parentIdx[idx]; parent >= 0; parent = parentIdx[parent] {
		if isSectionRole(nodes[parent].Role) {
			return contextLabel(nodes[parent])
		}
	}
	return ""
}

func isLabelledContainer(role string) bool {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "form", "group", "region", "dialog", "tabpanel":
		return true
	default:
		return false
	}
}

func isSectionRole(role string) bool {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "form", "region", "dialog", "navigation", "main", "banner", "contentinfo", "group", "tabpanel":
		return true
	default:
		return false
	}
}
