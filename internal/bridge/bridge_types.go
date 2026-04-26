package bridge

import (
	"context"
	"time"
)

type TabEntry struct {
	Ctx                   context.Context
	Cancel                context.CancelFunc
	Accessed              bool
	CDPID                 string    // raw CDP target ID
	CreatedAt             time.Time // when the tab was first created/registered
	LastUsed              time.Time // last time the tab was accessed via TabContext
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
	Refs    map[string]int64
	Targets map[string]RefTarget
	Nodes   []A11yNode
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
