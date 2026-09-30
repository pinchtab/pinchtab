package bridge

import (
	"context"

	bridgeobserve "github.com/pinchtab/pinchtab/internal/bridge/observe"
)

const (
	DefaultNetworkBufferSize = bridgeobserve.DefaultNetworkBufferSize
	FilterInteractive        = bridgeobserve.FilterInteractive
)

var (
	InteractiveRoles = bridgeobserve.InteractiveRoles
	ContextRoles     = bridgeobserve.ContextRoles
)

type A11yNode = bridgeobserve.A11yNode
type RawAXNode = bridgeobserve.RawAXNode
type RawAXValue = bridgeobserve.RawAXValue
type RawAXProp = bridgeobserve.RawAXProp
type RawFrame = bridgeobserve.RawFrame
type RawFrameTree = bridgeobserve.RawFrameTree
type rawFrameTree = bridgeobserve.RawFrameTree

type NetworkEntry = bridgeobserve.NetworkEntry
type NetworkBuffer = bridgeobserve.NetworkBuffer
type NetworkFilter = bridgeobserve.NetworkFilter
type NetworkMonitor = bridgeobserve.NetworkMonitor
type MemoryMetrics = bridgeobserve.MemoryMetrics

type PageMetrics = bridgeobserve.PageMetrics

type FrameContext = bridgeobserve.FrameContext

func FetchFrameContext(ctx context.Context) (FrameContext, error) {
	return bridgeobserve.FetchFrameContext(ctx)
}

type BoundingBox = bridgeobserve.BoundingBox
type ViewportInfo = bridgeobserve.ViewportInfo

func FetchLayout(ctx context.Context) (ViewportInfo, error) {
	return bridgeobserve.FetchLayout(ctx)
}

func ElementBorderBox(ctx context.Context, backendNodeID int64) (BoundingBox, bool) {
	return bridgeobserve.ElementBorderBox(ctx, backendNodeID)
}

func IsOnScreen(box BoundingBox, vp ViewportInfo) bool {
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

func TruncateToTokens(nodes []A11yNode, maxTokens int, format string) ([]A11yNode, bool) {
	return bridgeobserve.TruncateToTokens(nodes, maxTokens, format)
}

func GetResponseBody(ctx context.Context, requestID string) (string, bool, error) {
	return bridgeobserve.GetResponseBody(ctx, requestID)
}

func (b *Bridge) GetAggregatedMemoryMetrics() (*MemoryMetrics, error) {
	var targets map[string]context.Context
	if tm, err := b.tabManager(); err == nil {
		targets = tm.LiveTabContexts()
	}
	return bridgeobserve.GetAggregatedMemoryMetrics(b.BrowserCtx, targets)
}
