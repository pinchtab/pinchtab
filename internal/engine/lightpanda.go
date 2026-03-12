package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/chromedp/cdproto/accessibility"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/input"
	cdppage "github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

// LightpandaEngine implements Engine using a persistent CDP connection to a
// Lightpanda browser instance. Unlike the prior prototype that reconnected
// per-operation (freshContext), this keeps the WebSocket alive across calls,
// eliminating the re-navigation overhead that made the old approach 1.9x
// slower than Chrome.
type LightpandaEngine struct {
	wsURL string // e.g. "ws://127.0.0.1:9222"

	mu       sync.Mutex
	allocCtx context.Context    // remote allocator context
	allocCan context.CancelFunc // cancel for allocator
	tabs     map[string]*lpTab
	current  string
	seq      int
}

type lpTab struct {
	ctx    context.Context
	cancel context.CancelFunc
	url    string
	refMap map[string]int64 // ref → backendNodeId
}

// NewLightpandaEngine creates an engine that connects to a running Lightpanda
// CDP server at the given WebSocket URL. The URL should be the base endpoint,
// e.g. "ws://127.0.0.1:9222".
func NewLightpandaEngine(wsURL string) (*LightpandaEngine, error) {
	if wsURL == "" {
		return nil, errors.New("lightpanda: wsURL required")
	}

	// Probe the /json/version endpoint to get the actual devtools WS URL.
	// chromedp.NewRemoteAllocator handles this for us.
	allocCtx, allocCancel := chromedp.NewRemoteAllocator(context.Background(), wsURL)

	return &LightpandaEngine{
		wsURL:    wsURL,
		allocCtx: allocCtx,
		allocCan: allocCancel,
		tabs:     make(map[string]*lpTab),
	}, nil
}

func (lp *LightpandaEngine) Name() string { return "lightpanda" }

func (lp *LightpandaEngine) Capabilities() []Capability {
	return []Capability{CapNavigate, CapSnapshot, CapText, CapClick, CapType, CapEvaluate}
}

// Navigate opens a URL in a new Lightpanda target and waits for load.
func (lp *LightpandaEngine) Navigate(ctx context.Context, url string) (*NavigateResult, error) {
	lp.mu.Lock()
	defer lp.mu.Unlock()

	// Create a new browser context + target through CDP.
	tabCtx, tabCancel := chromedp.NewContext(lp.allocCtx)

	// Navigate with a timeout.
	navCtx, navCancel := context.WithTimeout(tabCtx, 30*time.Second)
	defer navCancel()

	if err := chromedp.Run(navCtx, chromedp.Navigate(url)); err != nil {
		tabCancel()
		return nil, fmt.Errorf("lightpanda navigate: %w", err)
	}

	// Wait for page load event.
	if err := chromedp.Run(navCtx, cdppage.Enable()); err != nil {
		slog.Debug("lightpanda: page.enable", "err", err)
	}

	// Get title.
	var title string
	_ = chromedp.Run(navCtx, chromedp.Title(&title))

	lp.seq++
	tabID := fmt.Sprintf("lp-%d", lp.seq)
	lp.tabs[tabID] = &lpTab{
		ctx:    tabCtx,
		cancel: tabCancel,
		url:    url,
		refMap: make(map[string]int64),
	}
	lp.current = tabID

	return &NavigateResult{
		TabID: tabID,
		URL:   url,
		Title: title,
	}, nil
}

// Snapshot returns the accessibility tree from Lightpanda's CDP endpoint.
func (lp *LightpandaEngine) Snapshot(ctx context.Context, filter string) ([]SnapshotNode, error) {
	lp.mu.Lock()
	defer lp.mu.Unlock()

	tab := lp.tabs[lp.current]
	if tab == nil {
		return nil, errors.New("no page loaded")
	}

	tCtx, tCancel := context.WithTimeout(tab.ctx, 10*time.Second)
	defer tCancel()

	// Fetch the full accessibility tree via CDP.
	var result json.RawMessage
	if err := chromedp.Run(tCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx,
				"Accessibility.getFullAXTree", nil, &result)
		}),
	); err != nil {
		return nil, fmt.Errorf("lightpanda a11y tree: %w", err)
	}

	var treeResp struct {
		Nodes []*accessibility.Node `json:"nodes"`
	}
	if err := json.Unmarshal(result, &treeResp); err != nil {
		return nil, fmt.Errorf("lightpanda parse a11y: %w", err)
	}

	// Build ref map and snapshot nodes.
	tab.refMap = make(map[string]int64)
	nodes := buildLPSnapshot(tab, treeResp.Nodes, filter)
	return nodes, nil
}

// Text returns the visible text content via JS evaluation.
func (lp *LightpandaEngine) Text(ctx context.Context) (string, error) {
	lp.mu.Lock()
	defer lp.mu.Unlock()

	tab := lp.tabs[lp.current]
	if tab == nil {
		return "", errors.New("no page loaded")
	}

	tCtx, tCancel := context.WithTimeout(tab.ctx, 10*time.Second)
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

	tab := lp.tabs[lp.current]
	if tab == nil {
		return errors.New("no page loaded")
	}

	backendID, ok := tab.refMap[ref]
	if !ok {
		return fmt.Errorf("ref %q not found (take a snapshot first)", ref)
	}

	tCtx, tCancel := context.WithTimeout(tab.ctx, 10*time.Second)
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

	tab := lp.tabs[lp.current]
	if tab == nil {
		return errors.New("no page loaded")
	}

	backendID, ok := tab.refMap[ref]
	if !ok {
		return fmt.Errorf("ref %q not found (take a snapshot first)", ref)
	}

	tCtx, tCancel := context.WithTimeout(tab.ctx, 10*time.Second)
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

// Close releases all tabs and the allocator connection.
func (lp *LightpandaEngine) Close() error {
	lp.mu.Lock()
	defer lp.mu.Unlock()

	for _, tab := range lp.tabs {
		// Close the target gracefully.
		_ = chromedp.Run(tab.ctx,
			chromedp.ActionFunc(func(ctx context.Context) error {
				tid := chromedp.FromContext(ctx).Target.TargetID
				return target.CloseTarget(tid).Do(ctx)
			}),
		)
		tab.cancel()
	}
	lp.tabs = make(map[string]*lpTab)

	if lp.allocCan != nil {
		lp.allocCan()
	}
	return nil
}

// --- accessibility tree helpers ---

func buildLPSnapshot(tab *lpTab, nodes []*accessibility.Node, filter string) []SnapshotNode {
	var out []SnapshotNode
	refSeq := 0

	for _, n := range nodes {
		if n.Ignored {
			continue
		}

		role := axValueStr(n.Role)
		name := axValueStr(n.Name)

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
			tab.refMap[ref] = int64(n.BackendDOMNodeID)
		}

		sn := SnapshotNode{
			Ref:         ref,
			Role:        role,
			Name:        name,
			Interactive: interactive,
		}

		// Extract value from the node's value field or properties.
		if n.Value != nil {
			sn.Value = axValueStr(n.Value)
		}
		for _, p := range n.Properties {
			if p.Name == "value" || p.Name == "valuetext" {
				sn.Value = axValueStr(p.Value)
			}
		}

		out = append(out, sn)
	}
	return out
}

func axValueStr(v *accessibility.Value) string {
	if v == nil {
		return ""
	}
	// Value.Value is jsontext.Value ([]byte) — try to unmarshal as string.
	var s string
	if err := json.Unmarshal(v.Value, &s); err == nil {
		return s
	}
	return string(v.Type)
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
