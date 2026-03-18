package bridge

import (
	"context"

	bridgeobserve "github.com/pinchtab/pinchtab/internal/bridge/observe"
)

const (
	DefaultNetworkBufferSize = bridgeobserve.DefaultNetworkBufferSize
	FilterInteractive        = bridgeobserve.FilterInteractive
)

var InteractiveRoles = bridgeobserve.InteractiveRoles

type A11yNode = bridgeobserve.A11yNode
type RawAXNode = bridgeobserve.RawAXNode
type RawAXValue = bridgeobserve.RawAXValue
type RawAXProp = bridgeobserve.RawAXProp
type rawFrameTree = bridgeobserve.RawFrameTree

type NetworkEntry = bridgeobserve.NetworkEntry
type NetworkBuffer = bridgeobserve.NetworkBuffer
type NetworkFilter = bridgeobserve.NetworkFilter
type NetworkMonitor = bridgeobserve.NetworkMonitor
type MemoryMetrics = bridgeobserve.MemoryMetrics

func frameIDs(tree rawFrameTree) []string {
	return bridgeobserve.FrameIDs(tree)
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

func TruncateToTokens(nodes []A11yNode, maxTokens int, format string) ([]A11yNode, bool) {
	return bridgeobserve.TruncateToTokens(nodes, maxTokens, format)
}

func NewNetworkBuffer(size int) *NetworkBuffer {
	return bridgeobserve.NewNetworkBuffer(size)
}

func NewNetworkMonitor(bufferSize int) *NetworkMonitor {
	return bridgeobserve.NewNetworkMonitor(bufferSize)
}

func matchStatusRange(status int, pattern string) bool {
	return bridgeobserve.MatchStatusRange(status, pattern)
}

func GetResponseBodyDirect(ctx context.Context, requestID string) (string, bool, error) {
	return bridgeobserve.GetResponseBodyDirect(ctx, requestID)
}

func (b *Bridge) GetMemoryMetrics(tabID string) (*MemoryMetrics, error) {
	return b.GetAggregatedMemoryMetrics()
}

func (b *Bridge) GetBrowserMemoryMetrics() (*MemoryMetrics, error) {
	return b.GetAggregatedMemoryMetrics()
}

func (b *Bridge) GetAggregatedMemoryMetrics() (*MemoryMetrics, error) {
	return bridgeobserve.GetAggregatedMemoryMetrics(b.BrowserCtx)
}
