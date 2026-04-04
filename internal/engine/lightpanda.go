package engine

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/chromedp/cdproto/accessibility"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/urls"
)

// LightpandaEngine implements Engine using a persistent CDP WebSocket
// connection to a Lightpanda browser instance.
type LightpandaEngine struct {
	wsURL string

	allocCtx    context.Context
	allocCancel context.CancelFunc
	ctx         context.Context
	ctxCancel   context.CancelFunc

	// refMap maps snapshot ref strings (e.g. "e0") to backend DOM node IDs.
	refMap map[string]cdp.BackendNodeID
	mu     sync.Mutex
}

// NewLightpandaEngine connects to a Lightpanda instance at the given
// WebSocket URL. The connection is persistent — Lightpanda drops the
// connection when new targets are created, so we reuse a single context.
func NewLightpandaEngine(wsURL string) (*LightpandaEngine, error) {
	if wsURL == "" {
		wsURL = "ws://127.0.0.1:19222"
	}

	// Use NoModifyURL so chromedp doesn't try to discover the WS URL
	// via an HTTP debug endpoint (Lightpanda exposes raw WS only).
	allocCtx, allocCancel := chromedp.NewRemoteAllocator(
		context.Background(),
		wsURL,
		chromedp.NoModifyURL,
	)

	ctx, ctxCancel := chromedp.NewContext(allocCtx)

	lp := &LightpandaEngine{
		wsURL:       wsURL,
		allocCtx:    allocCtx,
		allocCancel: allocCancel,
		ctx:         ctx,
		ctxCancel:   ctxCancel,
		refMap:      make(map[string]cdp.BackendNodeID),
	}

	slog.Info("lightpanda engine created", "ws", wsURL)
	return lp, nil
}

func (lp *LightpandaEngine) Name() string { return "lightpanda" }

func (lp *LightpandaEngine) Capabilities() []Capability {
	return []Capability{CapNavigate, CapSnapshot, CapText, CapClick, CapType}
}

func (lp *LightpandaEngine) Navigate(ctx context.Context, url string) (*NavigateResult, error) {
	lp.mu.Lock()
	defer lp.mu.Unlock()

	// Validate and sanitize URL to prevent SSRF (CodeQL go/request-forgery).
	safeURL, err := urls.Sanitize(url)
	if err != nil {
		return nil, fmt.Errorf("lightpanda navigate: %w", err)
	}

	if err := chromedp.Run(lp.ctx, chromedp.Navigate(safeURL)); err != nil {
		return nil, fmt.Errorf("lightpanda navigate: %w", err)
	}

	var title string
	_ = chromedp.Run(lp.ctx, chromedp.Title(&title))

	return &NavigateResult{
		TabID:  "lp-0",
		URL:    url,
		Title:  title,
		Engine: "lightpanda",
	}, nil
}

func (lp *LightpandaEngine) Snapshot(ctx context.Context, tabID, filter string) (*SnapshotResult, error) {
	lp.mu.Lock()
	defer lp.mu.Unlock()

	axNodes, err := accessibility.GetFullAXTree().WithDepth(-1).Do(lp.ctx)
	if err != nil {
		return nil, fmt.Errorf("lightpanda snapshot: %w", err)
	}

	lp.refMap = make(map[string]cdp.BackendNodeID)
	nodes := lp.buildSnapshotNodes(axNodes, filter)

	var title, pageURL string
	_ = chromedp.Run(lp.ctx, chromedp.Title(&title))
	_ = chromedp.Run(lp.ctx, chromedp.Location(&pageURL))

	return &SnapshotResult{
		Nodes:  nodes,
		URL:    pageURL,
		Title:  title,
		Engine: "lightpanda",
	}, nil
}

func (lp *LightpandaEngine) Text(ctx context.Context, tabID string) (*TextResult, error) {
	lp.mu.Lock()
	defer lp.mu.Unlock()

	var text string
	err := chromedp.Run(lp.ctx, chromedp.Evaluate(`document.body.innerText || ""`, &text))
	if err != nil {
		// Fallback: extract text from AX tree.
		axNodes, axErr := accessibility.GetFullAXTree().WithDepth(-1).Do(lp.ctx)
		if axErr != nil {
			return nil, fmt.Errorf("lightpanda text: %w", err)
		}
		text = extractTextFromAXNodes(axNodes)
	}

	var title, pageURL string
	_ = chromedp.Run(lp.ctx, chromedp.Title(&title))
	_ = chromedp.Run(lp.ctx, chromedp.Location(&pageURL))

	return &TextResult{
		Text:   text,
		URL:    pageURL,
		Title:  title,
		Engine: "lightpanda",
	}, nil
}

func (lp *LightpandaEngine) Click(ctx context.Context, tabID, ref string) error {
	lp.mu.Lock()
	defer lp.mu.Unlock()

	backendID, ok := lp.refMap[ref]
	if !ok {
		return fmt.Errorf("ref %q not found (take a snapshot first)", ref)
	}

	executor := cdp.WithExecutor(lp.ctx, chromedp.FromContext(lp.ctx).Target)

	// Focus the node.
	if err := dom.Focus().WithBackendNodeID(backendID).Do(executor); err != nil {
		slog.Debug("lightpanda click focus failed, continuing", "err", err)
	}

	// Get box model for click coordinates.
	box, err := dom.GetBoxModel().WithBackendNodeID(backendID).Do(executor)
	if err != nil {
		// Fallback: click active element via JS.
		var ok bool
		jsErr := chromedp.Run(lp.ctx, chromedp.Evaluate(`(function(){ try { document.activeElement.click(); return true; } catch(e) { return false; } })()`, &ok))
		if jsErr != nil {
			return fmt.Errorf("lightpanda click: box model failed: %w, js fallback failed: %w", err, jsErr)
		}
		return nil
	}

	x, y := boxCenter(box)
	if err := input.DispatchMouseEvent(input.MousePressed, x, y).WithButton(input.Left).WithClickCount(1).Do(executor); err != nil {
		return fmt.Errorf("lightpanda click press: %w", err)
	}
	if err := input.DispatchMouseEvent(input.MouseReleased, x, y).WithButton(input.Left).WithClickCount(1).Do(executor); err != nil {
		return fmt.Errorf("lightpanda click release: %w", err)
	}

	return nil
}

func (lp *LightpandaEngine) Type(ctx context.Context, tabID, ref, text string) error {
	lp.mu.Lock()
	defer lp.mu.Unlock()

	backendID, ok := lp.refMap[ref]
	if !ok {
		return fmt.Errorf("ref %q not found (take a snapshot first)", ref)
	}

	executor := cdp.WithExecutor(lp.ctx, chromedp.FromContext(lp.ctx).Target)
	if err := dom.Focus().WithBackendNodeID(backendID).Do(executor); err != nil {
		return fmt.Errorf("lightpanda type focus: %w", err)
	}

	for _, ch := range text {
		s := string(ch)
		if err := input.DispatchKeyEvent(input.KeyDown).WithText(s).Do(executor); err != nil {
			return fmt.Errorf("lightpanda type key: %w", err)
		}
		if err := input.DispatchKeyEvent(input.KeyUp).Do(executor); err != nil {
			return fmt.Errorf("lightpanda type keyup: %w", err)
		}
	}

	return nil
}

func (lp *LightpandaEngine) Close() error {
	lp.mu.Lock()
	defer lp.mu.Unlock()

	if lp.ctxCancel != nil {
		lp.ctxCancel()
	}
	if lp.allocCancel != nil {
		lp.allocCancel()
	}
	return nil
}

// ---------- snapshot helpers ----------

// buildSnapshotNodes converts CDP accessibility nodes into SnapshotNode slices.
func (lp *LightpandaEngine) buildSnapshotNodes(axNodes []*accessibility.Node, filter string) []SnapshotNode {
	// Build depth map via parent→child relationships.
	depths := make(map[accessibility.NodeID]int, len(axNodes))
	if len(axNodes) > 0 {
		depths[axNodes[0].NodeID] = 0
		for _, n := range axNodes {
			d := depths[n.NodeID]
			for _, cid := range n.ChildIDs {
				depths[cid] = d + 1
			}
		}
	}

	var nodes []SnapshotNode
	for _, n := range axNodes {
		if n.Ignored {
			continue
		}

		role := axValueStr(n.Role)
		name := axValueStr(n.Name)
		value := axValueStr(n.Value)

		if (role == "none" || role == "generic" || role == "") && name == "" {
			continue
		}

		interactive := isInteractiveRole(role)
		if filter == "interactive" && !interactive {
			continue
		}

		ref := fmt.Sprintf("e%d", len(lp.refMap))
		lp.refMap[ref] = n.BackendDOMNodeID

		nodes = append(nodes, SnapshotNode{
			Ref:         ref,
			Role:        role,
			Name:        name,
			Value:       value,
			Depth:       depths[n.NodeID],
			Interactive: interactive,
		})
	}

	return nodes
}

// axValueStr safely extracts the string from an accessibility.Value.
func axValueStr(v *accessibility.Value) string {
	if v == nil || len(v.Value) == 0 {
		return ""
	}
	// v.Value is jsontext.Value ([]byte). Try to unquote a JSON string;
	// fall back to raw representation.
	raw := string(v.Value)
	if len(raw) >= 2 && raw[0] == '"' && raw[len(raw)-1] == '"' {
		return raw[1 : len(raw)-1]
	}
	return raw
}

// extractTextFromAXNodes extracts readable text from accessibility nodes.
func extractTextFromAXNodes(axNodes []*accessibility.Node) string {
	var sb strings.Builder
	for _, n := range axNodes {
		if n.Ignored {
			continue
		}
		name := axValueStr(n.Name)
		if name != "" {
			if sb.Len() > 0 {
				sb.WriteByte(' ')
			}
			sb.WriteString(name)
		}
	}
	return sb.String()
}

// isInteractiveRole returns true for ARIA roles that represent interactive elements.
func isInteractiveRole(role string) bool {
	switch role {
	case "button", "link", "textbox", "combobox", "checkbox", "radio",
		"tab", "menuitem", "switch", "searchbox", "slider", "spinbutton":
		return true
	}
	return false
}

// boxCenter returns the center coordinates of a DOM box model's content quad.
func boxCenter(box *dom.BoxModel) (float64, float64) {
	if box == nil || len(box.Content) < 8 {
		return 0, 0
	}
	x := (box.Content[0] + box.Content[2] + box.Content[4] + box.Content[6]) / 4
	y := (box.Content[1] + box.Content[3] + box.Content[5] + box.Content[7]) / 4
	return x, y
}
