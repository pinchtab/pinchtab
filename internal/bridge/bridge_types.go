package bridge

import (
	"context"
	"fmt"
	"maps"
	"net"
	"strconv"
	"sync"
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
	CreatorScope          string
	UsedByOtherScope      bool

	idleTimer   *time.Timer
	idleGen     uint64
	frozen      bool
	awakeHolds  int
	lifecycleMu sync.Mutex
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
	Book     *RefBook
}

// RefBook gives each backend DOM node a stable ref within a DOM epoch: a node
// keeps the ref it was first assigned, and an unseen node gets the next number.
// This is what makes a ref denote a node rather than a row index, so the same
// node keeps its ref across a change of filter, depth, selector, token budget or
// republish. Refs are therefore sparse in a filtered view. A node with a zero
// backend id carries no identity: alongside resolvable nodes it gets a fresh ref
// every read and is never resolvable, so its instability is harmless; a snapshot
// made entirely of such nodes is a statically fetched page whose refs are kept
// stable per element by the static browser itself and are preserved untouched.
type RefBook struct {
	byNode map[int64]string
	next   int
}

func NewRefBook() *RefBook {
	return &RefBook{byNode: make(map[int64]string)}
}

func (b *RefBook) assign(backendNodeID int64) string {
	if backendNodeID != 0 {
		if ref, ok := b.byNode[backendNodeID]; ok {
			return ref
		}
	}
	ref := fmt.Sprintf("e%d", b.next)
	b.next++
	if backendNodeID != 0 {
		b.byNode[backendNodeID] = ref
	}
	return ref
}

// Rebind stamps each node with its stable ref and returns the ref→backend-node
// map for the resolvable nodes. A snapshot carrying no resolvable node is a
// statically fetched page: the static browser already keeps each ref stable per
// element, so its refs are left untouched — renumbering them against this book's
// counter would put them out of step with the static browser's own ref→element
// map and resolve a later action to the wrong element.
func (b *RefBook) Rebind(nodes []A11yNode) map[string]int64 {
	if !anyResolvable(nodes) {
		return map[string]int64{}
	}
	refs := make(map[string]int64, len(nodes))
	for i := range nodes {
		ref := b.assign(nodes[i].NodeID)
		nodes[i].Ref = ref
		if nodes[i].NodeID != 0 {
			refs[ref] = nodes[i].NodeID
		}
	}
	return refs
}

func anyResolvable(nodes []A11yNode) bool {
	for i := range nodes {
		if nodes[i].NodeID != 0 {
			return true
		}
	}
	return false
}

func (b *RefBook) sharesNode(nodes []A11yNode) bool {
	for i := range nodes {
		if nodes[i].NodeID == 0 {
			continue
		}
		if _, ok := b.byNode[nodes[i].NodeID]; ok {
			return true
		}
	}
	return false
}

// bookFromCache returns a fresh copy of the epoch's ref book, so each publish
// grows its own copy rather than mutating one shared across concurrent reads. A
// cache published before the book existed — the ghost-chrome escalation seam
// carries only Refs — is seeded from those refs, so the refs already handed out
// survive the transition to Chrome.
func (c *RefCache) bookFromCache() *RefBook {
	book := NewRefBook()
	if c == nil {
		return book
	}
	if c.Book != nil {
		maps.Copy(book.byNode, c.Book.byNode)
		book.next = c.Book.next
		return book
	}
	for ref, id := range c.Refs {
		if id == 0 {
			continue
		}
		book.byNode[id] = ref
		if n, ok := refNumber(ref); ok && n >= book.next {
			book.next = n + 1
		}
	}
	return book
}

func refNumber(ref string) (int, bool) {
	if len(ref) < 2 || ref[0] != 'e' {
		return 0, false
	}
	n, err := strconv.Atoi(ref[1:])
	if err != nil {
		return 0, false
	}
	return n, true
}

// EpochRefs rebinds nodes against the tab's DOM-epoch ref book and returns the
// RefCache to publish. When the nodes share a backend node with the prior epoch
// it carries that epoch's book and vocab token, so a node keeps its ref across
// filter, depth, selector and republish reads and the superseded-vocabulary
// guard does not fire. When they share none — a fresh document — it starts a new
// epoch: a book numbering from e0 and a fresh vocab token.
func EpochRefs(prev *RefCache, nodes []A11yNode) *RefCache {
	book := prev.bookFromCache()
	vocab := ""
	if prev != nil {
		vocab = prev.DomEpoch
	}
	if !book.sharesNode(nodes) {
		book = NewRefBook()
		vocab = ""
	}
	if vocab == "" {
		vocab = MintVocabToken()
	}
	refs := book.Rebind(nodes)
	return &RefCache{
		Refs:     refs,
		Targets:  RefTargetsFromNodes(nodes),
		Nodes:    nodes,
		DomEpoch: vocab,
		Book:     book,
	}
}

// Lookup answers whether a ref names a node that can be acted on. A zero backend
// node id is not such a node, so an entry carrying one is reported as absent —
// this is the single owner of that invariant, which is why neither resolveRef
// restates it.
//
// Both maps need the check, and for different reasons. RefTargetsFromNodes
// already skips zero-id nodes, but the ghost-chrome static-snapshot route writes
// Targets directly and deliberately stores a zero alongside a zero in Refs: those
// refs exist so an agent can read a statically fetched page, before anything has
// escalated to Chrome and given them real node ids. Zero there means "a known ref
// with no live node yet", which is a third state, not a defect — the snapshot
// keeps returning those refs, and only resolution refuses them.
func (c *RefCache) Lookup(ref string) (RefTarget, bool) {
	if c == nil {
		return RefTarget{}, false
	}
	if c.Targets != nil {
		if target, ok := c.Targets[ref]; ok && target.BackendNodeID != 0 {
			return target, true
		}
	}
	if c.Refs != nil {
		if nid, ok := c.Refs[ref]; ok && nid != 0 {
			return RefTarget{BackendNodeID: nid}, true
		}
	}
	return RefTarget{}, false
}

func MintVocabToken() string {
	return mintDomEpoch()
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
	TabID  string
	URL    string
	Title  string
	Status int
	Route  *browserops.RouteMetadata
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
	MaxRedirects int
	// DispatchOnly sends Page.navigate and returns after Chrome accepts it,
	// without waiting for load/ready state. The caller must poll the explicit
	// tab for readiness. This is used for headed background tabs whose SPA load
	// event can remain pending indefinitely without ever activating the window.
	DispatchOnly       bool
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
	SkipMetadata bool
}
