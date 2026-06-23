package bridge

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/pinchtab/pinchtab/internal/browserops"
	"github.com/pinchtab/pinchtab/internal/contentguard"
	"github.com/pinchtab/pinchtab/internal/idpi"
)

type TabEntry struct {
	Ctx                   context.Context
	Cancel                context.CancelFunc
	Accessed              bool
	CDPID                 string // raw CDP target ID
	CreatedAt             time.Time
	LastUsed              time.Time
	Policy                TabPolicyState
	Watching              bool
	ConsoleCaptureEnabled bool

	// Lifecycle auto-close timer. autoCloseGen is bumped on every (re)schedule
	// so a fire that races with a reset/cancel can detect itself and bail.
	autoCloseTimer *time.Timer
	autoCloseGen   uint64
}

type RefTarget struct {
	BackendNodeID  int64  `json:"backendNodeId"`
	FrameID        string `json:"frameId,omitempty"`
	FrameURL       string `json:"frameUrl,omitempty"`
	FrameName      string `json:"frameName,omitempty"`
	ChildFrameID   string `json:"childFrameId,omitempty"`
	ChildFrameURL  string `json:"childFrameUrl,omitempty"`
	ChildFrameName string `json:"childFrameName,omitempty"`
}

type RefCache struct {
	Refs     map[string]int64
	Targets  map[string]RefTarget
	Nodes    []A11yNode
	DomEpoch string
}

func (c *RefCache) Lookup(ref string) (RefTarget, bool) {
	if c == nil {
		return RefTarget{}, false
	}
	if c.Targets != nil {
		if target, ok := c.Targets[ref]; ok {
			return target, true
		}
	}
	if c.Refs != nil {
		if nid, ok := c.Refs[ref]; ok {
			return RefTarget{BackendNodeID: nid}, true
		}
	}
	return RefTarget{}, false
}

func RefTargetsFromNodes(nodes []A11yNode) map[string]RefTarget {
	targets := make(map[string]RefTarget, len(nodes))
	for _, node := range nodes {
		if node.Ref == "" || node.NodeID == 0 {
			continue
		}
		targets[node.Ref] = RefTarget{
			BackendNodeID:  node.NodeID,
			FrameID:        node.FrameID,
			FrameURL:       node.FrameURL,
			FrameName:      node.FrameName,
			ChildFrameID:   node.ChildFrameID,
			ChildFrameURL:  node.ChildFrameURL,
			ChildFrameName: node.ChildFrameName,
		}
	}
	return targets
}

type FrameScope struct {
	FrameID   string `json:"frameId,omitempty"`
	FrameURL  string `json:"frameUrl,omitempty"`
	FrameName string `json:"frameName,omitempty"`
	OwnerRef  string `json:"ownerRef,omitempty"`
}

func (s FrameScope) Active() bool {
	return s.FrameID != ""
}

type NavigateResult struct {
	TabID string
	URL   string
	Title string
	Route *browserops.RouteMetadata
}

// SnapshotResult is the bridge-level response from a snapshot operation.
// It carries the rich A11yNode tree (not the simpler browserops.SnapshotNode)
// together with ref-cache data so that handlers can store it via SetRefCache
// without needing to call CDP directly.
type SnapshotResult struct {
	Nodes       []A11yNode
	Refs        map[string]int64
	Targets     map[string]RefTarget
	URL         string
	Title       string
	Truncated   bool
	Hint        string
	IDPIWarning string
	Route       *browserops.RouteMetadata
}

type TextResult struct {
	Text        string
	URL         string
	Title       string
	Truncated   bool
	IDPIWarning string
	Route       *browserops.RouteMetadata
}

type NavigateParams struct {
	MaxRedirects       int
	AllowInternal      bool
	TrustedProxyCIDRs  []net.IPNet
	TrustedResolvedIPs []net.IP
	IDPIGuard          idpi.Guard
	// NoEscalate makes a static-first bridge return *StaticEscalateError
	// instead of internally escalating to Chrome. Handlers use it to defer
	// the Chrome launch until the static path has proven insufficient.
	NoEscalate bool
	// SkipStatic bypasses the static-first attempt entirely (the handler
	// already ran it via NoEscalate and is escalating).
	SkipStatic bool
}

// StaticEscalateError signals that the static-first path cannot serve a
// navigate (NavigateParams.NoEscalate mode) and the caller should launch
// Chrome and retry with SkipStatic. Route carries the static attempt's
// metadata so the caller can merge it with the Chrome attempt.
type StaticEscalateError struct {
	Quality int
	Reason  string
	Route   *browserops.RouteMetadata
}

func (e *StaticEscalateError) Error() string {
	return fmt.Sprintf("static path cannot serve this navigate (quality %d): %s", e.Quality, e.Reason)
}

type ContentParams struct {
	ContentGuard *contentguard.Scanner
	MaxDepth     int // max AX tree depth for snapshots; 0 or -1 means unlimited
}
