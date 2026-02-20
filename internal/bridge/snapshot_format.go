package bridge

import (
	"strings"
)

func FormatSnapshotText(nodes []A11yNode) string {
	var b strings.Builder
	for _, n := range nodes {
		for i := 0; i < n.Depth; i++ {
			b.WriteString("  ")
		}
		b.WriteString(n.Ref)
		b.WriteByte(' ')
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
			b.WriteString(" [focused]")
		}
		if n.Disabled {
			b.WriteString(" [disabled]")
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func FormatSnapshotCompact(nodes []A11yNode) string {
	var b strings.Builder
	for _, n := range nodes {
		b.WriteString(n.Ref)
		b.WriteByte(':')
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
			b.WriteString(" *")
		}
		if n.Disabled {
			b.WriteString(" -")
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func TruncateToTokens(nodes []A11yNode, maxTokens int, format string) ([]A11yNode, bool) {
	tokensUsed := 0
	for i, n := range nodes {
		var nodeTokens int
		switch format {
		case "compact":
			size := len(n.Ref) + 1 + len(n.Role) + len(n.Name) + len(n.Value) + 8
			nodeTokens = size / 4
		case "text":
			size := n.Depth*2 + len(n.Ref) + 1 + len(n.Role) + len(n.Name) + len(n.Value) + 8
			nodeTokens = size / 4
		default:
			size := len(n.Ref) + len(n.Role) + len(n.Name) + len(n.Value) + 60
			nodeTokens = size / 3
		}
		if nodeTokens < 1 {
			nodeTokens = 1
		}
		tokensUsed += nodeTokens
		if tokensUsed > maxTokens {
			return nodes[:i], true
		}
	}
	return nodes, false
}
