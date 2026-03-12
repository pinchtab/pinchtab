package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// LightpandaEngine implements Engine using a persistent CDP connection to a
// Lightpanda browser instance. Unlike the prior prototype that reconnected
// per-operation (freshContext), this keeps a single WebSocket and browser
// context alive across calls, eliminating the re-navigation overhead that
// made the old approach 1.9x slower than Chrome.
//
// Lightpanda supports only one BrowserContext at a time, so we must NOT
// call chromedp.NewContext per-navigation (that sends Target.createTarget
// which tears down the previous session). Instead we create one context
// eagerly and reuse it for all operations.
type LightpandaEngine struct {
	wsURL string // e.g. "ws://127.0.0.1:9222"

	mu       sync.Mutex
	allocCtx context.Context    // remote allocator context
	allocCan context.CancelFunc // cancel for allocator
	browCtx  context.Context    // single browser context (reused)
	browCan  context.CancelFunc // cancel for browser context
	url      string             // currently loaded URL
	refMap   map[string]int64   // ref → backendNodeId
	ready    bool               // true after first Navigate
}

// NewLightpandaEngine creates an engine that connects to a running Lightpanda
// CDP server at the given WebSocket URL. The URL should be the base endpoint,
// e.g. "ws://127.0.0.1:9222".
func NewLightpandaEngine(wsURL string) (*LightpandaEngine, error) {
	if wsURL == "" {
		return nil, errors.New("lightpanda: wsURL required")
	}

	// Use NoModifyURL so chromedp connects directly to the given WebSocket
	// URL instead of querying /json/version. Lightpanda in Docker advertises
	// the container-internal address (ws://0.0.0.0:9222/) which is
	// unreachable from the host.
	allocCtx, allocCancel := chromedp.NewRemoteAllocator(context.Background(), wsURL, chromedp.NoModifyURL)

	// Create a single browser context — Lightpanda only supports one.
	browCtx, browCancel := chromedp.NewContext(allocCtx)

	return &LightpandaEngine{
		wsURL:    wsURL,
		allocCtx: allocCtx,
		allocCan: allocCancel,
		browCtx:  browCtx,
		browCan:  browCancel,
		refMap:   make(map[string]int64),
	}, nil
}

func (lp *LightpandaEngine) Name() string { return "lightpanda" }

func (lp *LightpandaEngine) Capabilities() []Capability {
	return []Capability{CapNavigate, CapSnapshot, CapText, CapClick, CapType, CapEvaluate}
}

// Navigate opens a URL in the Lightpanda browser context and waits for load.
func (lp *LightpandaEngine) Navigate(ctx context.Context, url string) (*NavigateResult, error) {
	lp.mu.Lock()
	defer lp.mu.Unlock()

	if err := chromedp.Run(lp.browCtx, chromedp.Navigate(url)); err != nil {
		return nil, fmt.Errorf("lightpanda navigate: %w", err)
	}

	var title string

	lp.url = url
	lp.refMap = make(map[string]int64)
	lp.ready = true

	return &NavigateResult{
		TabID: "lp-0",
		URL:   url,
		Title: title,
	}, nil
}

// Snapshot returns the accessibility tree from Lightpanda's CDP endpoint.
func (lp *LightpandaEngine) Snapshot(ctx context.Context, filter string) ([]SnapshotNode, error) {
	lp.mu.Lock()
	defer lp.mu.Unlock()

	if !lp.ready {
		return nil, errors.New("no page loaded")
	}

	tCtx, tCancel := context.WithTimeout(lp.browCtx, 10*time.Second)
	defer tCancel()

	// Fetch the full accessibility tree via CDP.
	// Lightpanda requires explicit depth param (-1 = full tree);
	// Chrome accepts nil params but also handles depth=-1 fine.
	var result json.RawMessage
	axParams := map[string]interface{}{"depth": -1}
	if err := chromedp.Run(tCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx,
				"Accessibility.getFullAXTree", axParams, &result)
		}),
	); err != nil {
		return nil, fmt.Errorf("lightpanda a11y tree: %w", err)
	}

	// Parse with a lenient struct — Lightpanda returns property names
	// (e.g. "uninteresting") that the strict cdproto types reject.
	var treeResp struct {
		Nodes []lpAXNode `json:"nodes"`
	}
	if err := json.Unmarshal(result, &treeResp); err != nil {
		return nil, fmt.Errorf("lightpanda parse a11y: %w", err)
	}

	// Build ref map and snapshot nodes.
	lp.refMap = make(map[string]int64)
	nodes := buildLPSnapshot(lp, treeResp.Nodes, filter)
	return nodes, nil
}

// Text returns the visible text content via JS evaluation.
func (lp *LightpandaEngine) Text(ctx context.Context) (string, error) {
	lp.mu.Lock()
	defer lp.mu.Unlock()

	if !lp.ready {
		return "", errors.New("no page loaded")
	}

	tCtx, tCancel := context.WithTimeout(lp.browCtx, 10*time.Second)
	defer tCancel()

	var text string
	if err := chromedp.Run(tCtx,
		chromedp.Evaluate(`document.body.innerText`, &text),
	); err != nil {
		return "", fmt.Errorf("lightpanda text: %w", err)
	}
	return normalizeWhitespace(text), nil
}

// Click dispatches a mouse click to the element identified by ref.
func (lp *LightpandaEngine) Click(ctx context.Context, ref string) error {
	lp.mu.Lock()
	defer lp.mu.Unlock()

	if !lp.ready {
		return errors.New("no page loaded")
	}

	backendID, ok := lp.refMap[ref]
	if !ok {
		return fmt.Errorf("ref %q not found (take a snapshot first)", ref)
	}

	tCtx, tCancel := context.WithTimeout(lp.browCtx, 10*time.Second)
	defer tCancel()

	// Resolve the backendNodeId to get content quads for click coordinates.
	// If getContentQuads works, use its coordinates; otherwise fall back to
	// JS click via callFunctionOn.
	var quads []dom.Quad
	err := chromedp.Run(tCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			quads, err = dom.GetContentQuads().WithBackendNodeID(cdp.BackendNodeID(backendID)).Do(ctx)
			return err
		}),
	)
	if err == nil && len(quads) > 0 && len(quads[0]) >= 2 {
		// Use the center of the first quad.
		x, y := quadCenter(quads[0])
		return chromedp.Run(tCtx,
			input.DispatchMouseEvent(input.MousePressed, x, y).WithButton(input.Left).WithClickCount(1),
			input.DispatchMouseEvent(input.MouseReleased, x, y).WithButton(input.Left).WithClickCount(1),
		)
	}

	// Fallback: click via JS using the backend node ID.
	return chromedp.Run(tCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			node, err := dom.ResolveNode().WithBackendNodeID(cdp.BackendNodeID(backendID)).Do(ctx)
			if err != nil {
				return fmt.Errorf("resolve node: %w", err)
			}
			_, _, err = runtime.CallFunctionOn(`function() { this.click(); }`).
				WithObjectID(node.ObjectID).Do(ctx)
			return err
		}),
	)
}

// Type enters text into an element identified by ref.
func (lp *LightpandaEngine) Type(ctx context.Context, ref, text string) error {
	lp.mu.Lock()
	defer lp.mu.Unlock()

	if !lp.ready {
		return errors.New("no page loaded")
	}

	backendID, ok := lp.refMap[ref]
	if !ok {
		return fmt.Errorf("ref %q not found (take a snapshot first)", ref)
	}

	tCtx, tCancel := context.WithTimeout(lp.browCtx, 10*time.Second)
	defer tCancel()

	// Focus the element then insert text.
	return chromedp.Run(tCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			node, err := dom.ResolveNode().WithBackendNodeID(cdp.BackendNodeID(backendID)).Do(ctx)
			if err != nil {
				return fmt.Errorf("resolve node: %w", err)
			}
			// Focus via JS.
			_, _, err = runtime.CallFunctionOn(`function() { this.focus(); }`).
				WithObjectID(node.ObjectID).Do(ctx)
			if err != nil {
				return fmt.Errorf("focus: %w", err)
			}
			return nil
		}),
		input.InsertText(text),
	)
}

// Close releases the browser context and allocator connection.
func (lp *LightpandaEngine) Close() error {
	lp.mu.Lock()
	defer lp.mu.Unlock()

	if lp.browCan != nil {
		lp.browCan()
	}
	if lp.allocCan != nil {
		lp.allocCan()
	}
	return nil
}

// --- accessibility tree helpers ---

// lpAXNode is a lenient representation of a CDP accessibility node.
// We use this instead of cdproto's accessibility.Node because Lightpanda
// returns property names (e.g. "uninteresting") that the strict cdproto
// enum types reject during JSON unmarshalling.
type lpAXNode struct {
	NodeID           json.RawMessage `json:"nodeId"`
	BackendDOMNodeID int64           `json:"backendDOMNodeId"`
	Ignored          bool            `json:"ignored"`
	Role             *lpAXValue      `json:"role"`
	Name             *lpAXValue      `json:"name"`
	Value            *lpAXValue      `json:"value"`
	Properties       []lpAXProperty  `json:"properties"`
}

type lpAXValue struct {
	Type  string          `json:"type"`
	Value json.RawMessage `json:"value"`
}

type lpAXProperty struct {
	Name  string     `json:"name"`
	Value *lpAXValue `json:"value"`
}

func lpAXValueStr(v *lpAXValue) string {
	if v == nil {
		return ""
	}
	var s string
	if err := json.Unmarshal(v.Value, &s); err == nil {
		return s
	}
	return string(v.Type)
}

func buildLPSnapshot(lp *LightpandaEngine, nodes []lpAXNode, filter string) []SnapshotNode {
	var out []SnapshotNode
	refSeq := 0

	for _, n := range nodes {
		if n.Ignored {
			continue
		}

		role := lpAXValueStr(n.Role)
		name := lpAXValueStr(n.Name)

		// Skip the root document node and other non-semantic nodes.
		if role == "none" || role == "RootWebArea" || role == "GenericContainer" || role == "" {
			continue
		}

		interactive := isInteractiveRole(role)
		if filter == "interactive" && !interactive {
			continue
		}

		ref := fmt.Sprintf("e%d", refSeq)
		refSeq++

		if n.BackendDOMNodeID > 0 {
			lp.refMap[ref] = n.BackendDOMNodeID
		}

		sn := SnapshotNode{
			Ref:         ref,
			Role:        role,
			Name:        name,
			Interactive: interactive,
		}

		// Extract value from the node's value field or properties.
		if n.Value != nil {
			sn.Value = lpAXValueStr(n.Value)
		}
		for _, p := range n.Properties {
			if p.Name == "value" || p.Name == "valuetext" {
				sn.Value = lpAXValueStr(p.Value)
			}
		}

		out = append(out, sn)
	}
	return out
}

func isInteractiveRole(role string) bool {
	switch role {
	case "button", "link", "textbox", "checkbox", "radio", "combobox",
		"menuitem", "tab", "switch", "slider", "spinbutton",
		"searchbox", "option", "menuitemcheckbox", "menuitemradio":
		return true
	}
	return false
}

func quadCenter(q dom.Quad) (float64, float64) {
	if len(q) < 8 {
		return 0, 0
	}
	// Quad is [x1,y1, x2,y2, x3,y3, x4,y4]
	x := (q[0] + q[2] + q[4] + q[6]) / 4
	y := (q[1] + q[3] + q[5] + q[7]) / 4
	return x, y
}
