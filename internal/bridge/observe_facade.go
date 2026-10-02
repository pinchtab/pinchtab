package bridge

import (
	"context"

	bridgeobserve "github.com/pinchtab/pinchtab/internal/bridge/observe"
)

const FilterInteractive = bridgeobserve.FilterInteractive

type A11yNode = bridgeobserve.A11yNode
type RawAXNode = bridgeobserve.RawAXNode
type RawFrame = bridgeobserve.RawFrame

type NetworkEntry = bridgeobserve.NetworkEntry
type NetworkBuffer = bridgeobserve.NetworkBuffer
type NetworkFilter = bridgeobserve.NetworkFilter
type NetworkMonitor = bridgeobserve.NetworkMonitor

type PageMetrics = bridgeobserve.PageMetrics

func FetchFrameContext(ctx context.Context) (bridgeobserve.FrameContext, error) {
	return bridgeobserve.FetchFrameContext(ctx)
}

func FetchLayout(ctx context.Context) (bridgeobserve.ViewportInfo, error) {
	return bridgeobserve.FetchLayout(ctx)
}

func ElementBorderBox(ctx context.Context, backendNodeID int64) (bridgeobserve.BoundingBox, bool) {
	return bridgeobserve.ElementBorderBox(ctx, backendNodeID)
}

func IsOnScreen(box bridgeobserve.BoundingBox, vp bridgeobserve.ViewportInfo) bool {
	return bridgeobserve.IsOnScreen(box, vp)
}

func FetchAXTree(ctx context.Context) ([]RawAXNode, error) {
	return bridgeobserve.FetchAXTree(ctx)
}

func BuildSnapshot(nodes []RawAXNode, filter string, maxDepth int) ([]A11yNode, map[string]int64) {
	return bridgeobserve.BuildSnapshot(nodes, filter, maxDepth)
}

func FilterSubtree(nodes []RawAXNode, scopeBackendID int64) []RawAXNode {
	return bridgeobserve.FilterSubtree(nodes, scopeBackendID)
}

func DiffSnapshot(prev, curr []A11yNode) (added, changed, removed []A11yNode) {
	return bridgeobserve.DiffSnapshot(prev, curr)
}

func FormatSnapshotText(nodes []A11yNode) string {
	return bridgeobserve.FormatSnapshotText(nodes)
}

func FormatSnapshotCompact(nodes []A11yNode) string {
	return bridgeobserve.FormatSnapshotCompact(nodes)
}

func FormatSnapshotCompactDiff(nodes, added, changed, removed []A11yNode) string {
	return bridgeobserve.FormatSnapshotCompactDiff(nodes, added, changed, removed)
}

func FormatSnapshotTextDiff(nodes, added, changed, removed []A11yNode) string {
	return bridgeobserve.FormatSnapshotTextDiff(nodes, added, changed, removed)
}

func TruncateToTokens(nodes []A11yNode, maxTokens int, format string) ([]A11yNode, bool) {
	return bridgeobserve.TruncateToTokens(nodes, maxTokens, format)
}

func GetResponseBody(ctx context.Context, requestID string) (string, bool, error) {
	return bridgeobserve.GetResponseBody(ctx, requestID)
}

func (b *Bridge) GetAggregatedMemoryMetrics() (*bridgeobserve.MemoryMetrics, error) {
	var targets map[string]context.Context
	if tm, err := b.tabManager(); err == nil {
		targets = tm.LiveTabContexts()
	}
	return bridgeobserve.GetAggregatedMemoryMetrics(b.BrowserCtx, targets)
}
