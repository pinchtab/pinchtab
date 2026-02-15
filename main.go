package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
)

// ── Config ─────────────────────────────────────────────────

var (
	port       = envOr("BRIDGE_PORT", "18800")
	cdpURL     = os.Getenv("CDP_URL") // empty = launch Chrome ourselves
	token      = os.Getenv("BRIDGE_TOKEN")
	stateDir   = envOr("BRIDGE_STATE_DIR", filepath.Join(homeDir(), ".browser-bridge"))
	headless   = os.Getenv("BRIDGE_HEADLESS") == "true"
	profileDir = envOr("BRIDGE_PROFILE", filepath.Join(homeDir(), ".browser-bridge", "chrome-profile"))
	actionTimeout = 15 * time.Second
)

// ── State ──────────────────────────────────────────────────

type TabState struct {
	ID    string `json:"id"`
	URL   string `json:"url"`
	Title string `json:"title"`
}

type SessionState struct {
	Tabs    []TabState `json:"tabs"`
	SavedAt string     `json:"savedAt"`
}

type TabEntry struct {
	ctx    context.Context
	cancel context.CancelFunc
}

// refCache stores the ref→backendNodeID mapping from the last snapshot per tab.
// This avoids re-fetching the a11y tree on every action — refs stay stable
// until the next snapshot call.
type refCache struct {
	refs map[string]int64 // "e0" → backendNodeID
}

type Bridge struct {
	allocCtx   context.Context
	browserCtx context.Context // persistent browser context
	tabs       map[string]*TabEntry
	snapshots  map[string]*refCache // tabID → last snapshot's ref mapping
	mu         sync.RWMutex
}

var bridge Bridge

// ── A11y Node (flat) ───────────────────────────────────────

type A11yNode struct {
	Ref      string `json:"ref"`
	Role     string `json:"role"`
	Name     string `json:"name"`
	Depth    int    `json:"depth"`
	Value    string `json:"value,omitempty"`
	Disabled bool   `json:"disabled,omitempty"`
	Focused  bool   `json:"focused,omitempty"`
	NodeID   int64  `json:"nodeId,omitempty"` // backend DOM node ID for action resolution
}

// ── Main ───────────────────────────────────────────────────

func main() {
	os.MkdirAll(stateDir, 0755)

	var allocCancel context.CancelFunc

	if cdpURL != "" {
		log.Printf("Connecting to Chrome at %s", cdpURL)
		bridge.allocCtx, allocCancel = chromedp.NewRemoteAllocator(context.Background(), cdpURL)
	} else {
		os.MkdirAll(profileDir, 0755)
		log.Printf("Launching Chrome (profile: %s, headless: %v)", profileDir, headless)

		opts := []chromedp.ExecAllocatorOption{
			chromedp.UserDataDir(profileDir),
			chromedp.NoFirstRun,
			chromedp.NoDefaultBrowserCheck,

			// Stealth: disable automation indicators
			chromedp.Flag("disable-blink-features", "AutomationControlled"),
			chromedp.Flag("exclude-switches", "enable-automation"),
			chromedp.Flag("disable-infobars", true),
			chromedp.Flag("disable-background-networking", false),
			chromedp.Flag("enable-features", "NetworkService,NetworkServiceInProcess"),
			chromedp.Flag("disable-popup-blocking", true),
			chromedp.Flag("disable-default-apps", false),
			chromedp.Flag("no-first-run", true),

			chromedp.UserAgent("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/144.0.0.0 Safari/537.36"),
			chromedp.WindowSize(1440, 900),
		}

		if headless {
			opts = append(opts, chromedp.Headless)
		} else {
			opts = append(opts, chromedp.Flag("headless", false))
		}

		bridge.allocCtx, allocCancel = chromedp.NewExecAllocator(context.Background(), opts...)
	}
	defer allocCancel()

	browserCtx, browserCancel := chromedp.NewContext(bridge.allocCtx)
	defer browserCancel()

	// Initialize and inject stealth scripts
	if err := chromedp.Run(browserCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, err := page.AddScriptToEvaluateOnNewDocument(`
				Object.defineProperty(navigator, 'webdriver', { get: () => undefined });
				if (!window.chrome) { window.chrome = {}; }
				if (!window.chrome.runtime) { window.chrome.runtime = {}; }
				const originalQuery = window.navigator.permissions.query;
				window.navigator.permissions.query = (parameters) => (
					parameters.name === 'notifications' ?
						Promise.resolve({ state: Notification.permission }) :
						originalQuery(parameters)
				);
				Object.defineProperty(navigator, 'plugins', {
					get: () => [1, 2, 3, 4, 5],
				});
				Object.defineProperty(navigator, 'languages', {
					get: () => ['en-GB', 'en-US', 'en'],
				});
			`).Do(ctx)
			return err
		}),
	); err != nil {
		log.Fatalf("Cannot start Chrome: %v", err)
	}
	bridge.browserCtx = browserCtx
	bridge.tabs = make(map[string]*TabEntry)
	bridge.snapshots = make(map[string]*refCache)

	// Register the initial tab
	initTargetID := chromedp.FromContext(browserCtx).Target.TargetID
	bridge.tabs[string(initTargetID)] = &TabEntry{ctx: browserCtx}
	log.Printf("Initial tab: %s", initTargetID)

	// Restore tabs from last session
	restoreState()

	// Start background tab cleanup
	go bridge.cleanStaleTabs(30 * time.Second)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handleHealth)
	mux.HandleFunc("GET /tabs", handleTabs)
	mux.HandleFunc("GET /snapshot", handleSnapshot)
	mux.HandleFunc("GET /screenshot", handleScreenshot)
	mux.HandleFunc("GET /text", handleText)
	mux.HandleFunc("POST /navigate", handleNavigate)
	mux.HandleFunc("POST /action", handleAction)
	mux.HandleFunc("POST /evaluate", handleEvaluate)
	mux.HandleFunc("POST /tab", handleTab)

	handler := corsMiddleware(authMiddleware(mux))

	srv := &http.Server{Addr: ":" + port, Handler: handler}
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		log.Println("Shutting down, saving state...")
		saveState()
		srv.Shutdown(context.Background())
	}()

	log.Printf("🌉 Browser Bridge running on http://localhost:%s", port)
	log.Printf("   CDP target: %s", cdpURL)
	if token != "" {
		log.Println("   Auth: Bearer token required")
	} else {
		log.Println("   Auth: none (set BRIDGE_TOKEN to enable)")
	}

	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

// ── GET /health ────────────────────────────────────────────

func handleHealth(w http.ResponseWriter, r *http.Request) {
	targets, err := listTargets()
	if err != nil {
		jsonResp(w, 200, map[string]any{"status": "disconnected", "error": err.Error(), "cdp": cdpURL})
		return
	}
	jsonResp(w, 200, map[string]any{"status": "ok", "tabs": len(targets), "cdp": cdpURL})
}

// ── GET /tabs ──────────────────────────────────────────────

func handleTabs(w http.ResponseWriter, r *http.Request) {
	targets, err := listTargets()
	if err != nil {
		jsonErr(w, 500, err)
		return
	}

	tabs := make([]map[string]any, 0, len(targets))
	for _, t := range targets {
		tabs = append(tabs, map[string]any{
			"id":    string(t.TargetID),
			"url":   t.URL,
			"title": t.Title,
			"type":  t.Type,
		})
	}
	jsonResp(w, 200, map[string]any{"tabs": tabs})
}

// ── Raw a11y tree types (to avoid cdproto deserialization issues) ──

type rawAXNode struct {
	NodeID           string        `json:"nodeId"`
	Ignored          bool          `json:"ignored"`
	Role             *rawAXValue   `json:"role"`
	Name             *rawAXValue   `json:"name"`
	Value            *rawAXValue   `json:"value"`
	Properties       []rawAXProp   `json:"properties"`
	ChildIDs         []string      `json:"childIds"`
	BackendDOMNodeID int64         `json:"backendDOMNodeId"`
}

type rawAXValue struct {
	Type  string          `json:"type"`
	Value json.RawMessage `json:"value"`
}

type rawAXProp struct {
	Name  string      `json:"name"`
	Value *rawAXValue `json:"value"`
}

func (v *rawAXValue) String() string {
	if v == nil || v.Value == nil {
		return ""
	}
	var s string
	if err := json.Unmarshal(v.Value, &s); err == nil {
		return s
	}
	return strings.Trim(string(v.Value), `"`)
}

// ── GET /snapshot ──────────────────────────────────────────

func handleSnapshot(w http.ResponseWriter, r *http.Request) {
	tabID := r.URL.Query().Get("tabId")
	filter := r.URL.Query().Get("filter")
	maxDepthStr := r.URL.Query().Get("depth")
	maxDepth := -1
	if maxDepthStr != "" {
		if d, err := strconv.Atoi(maxDepthStr); err == nil {
			maxDepth = d
		}
	}

	ctx, resolvedTabID, err := tabContext(tabID)
	if err != nil {
		jsonErr(w, 404, err)
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, actionTimeout)
	defer tCancel()

	// Use raw CDP call to avoid cdproto deserialization issues
	var rawResult json.RawMessage
	if err := chromedp.Run(tCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx,
				"Accessibility.getFullAXTree", nil, &rawResult)
		}),
	); err != nil {
		jsonErr(w, 500, err)
		return
	}

	var treeResp struct {
		Nodes []rawAXNode `json:"nodes"`
	}
	if err := json.Unmarshal(rawResult, &treeResp); err != nil {
		jsonErr(w, 500, fmt.Errorf("parse a11y tree: %v", err))
		return
	}

	interactiveRoles := map[string]bool{
		"button": true, "link": true, "textbox": true, "searchbox": true,
		"combobox": true, "listbox": true, "option": true, "checkbox": true,
		"radio": true, "switch": true, "slider": true, "spinbutton": true,
		"menuitem": true, "menuitemcheckbox": true, "menuitemradio": true,
		"tab": true, "treeitem": true,
	}

	// Build parent map for depth
	parentMap := make(map[string]string)
	for _, n := range treeResp.Nodes {
		for _, childID := range n.ChildIDs {
			parentMap[childID] = n.NodeID
		}
	}
	depthOf := func(nodeID string) int {
		d := 0
		cur := nodeID
		for {
			p, ok := parentMap[cur]
			if !ok {
				break
			}
			d++
			cur = p
		}
		return d
	}

	flat := make([]A11yNode, 0)
	refs := make(map[string]int64) // ref→backendNodeID cache
	refID := 0

	for _, n := range treeResp.Nodes {
		if n.Ignored {
			continue
		}

		role := n.Role.String()
		name := n.Name.String()

		if role == "none" || role == "generic" || role == "InlineTextBox" {
			continue
		}
		if name == "" && role == "StaticText" {
			continue
		}

		depth := depthOf(n.NodeID)
		if maxDepth >= 0 && depth > maxDepth {
			continue
		}
		if filter == "interactive" && !interactiveRoles[role] {
			continue
		}

		ref := fmt.Sprintf("e%d", refID)
		entry := A11yNode{
			Ref:   ref,
			Role:  role,
			Name:  name,
			Depth: depth,
		}

		if v := n.Value.String(); v != "" {
			entry.Value = v
		}
		if n.BackendDOMNodeID != 0 {
			entry.NodeID = n.BackendDOMNodeID
			refs[ref] = n.BackendDOMNodeID
		}

		for _, prop := range n.Properties {
			if prop.Name == "disabled" && prop.Value.String() == "true" {
				entry.Disabled = true
			}
			if prop.Name == "focused" && prop.Value.String() == "true" {
				entry.Focused = true
			}
		}

		flat = append(flat, entry)
		refID++
	}

	// Cache the ref→nodeID mapping for this tab (#1 ref stability fix)
	bridge.mu.Lock()
	bridge.snapshots[resolvedTabID] = &refCache{refs: refs}
	bridge.mu.Unlock()

	var url, title string
	chromedp.Run(tCtx,
		chromedp.Location(&url),
		chromedp.Title(&title),
	)

	jsonResp(w, 200, map[string]any{
		"url":   url,
		"title": title,
		"nodes": flat,
		"count": len(flat),
	})
}

// ── GET /screenshot ────────────────────────────────────────

func handleScreenshot(w http.ResponseWriter, r *http.Request) {
	tabID := r.URL.Query().Get("tabId")

	ctx, _, err := tabContext(tabID)
	if err != nil {
		jsonErr(w, 404, err)
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, actionTimeout)
	defer tCancel()

	var buf []byte
	quality := 80
	if q := r.URL.Query().Get("quality"); q != "" {
		if qn, err := strconv.Atoi(q); err == nil {
			quality = qn
		}
	}

	if err := chromedp.Run(tCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			buf, err = page.CaptureScreenshot().
				WithFormat(page.CaptureScreenshotFormatJpeg).
				WithQuality(int64(quality)).
				Do(ctx)
			return err
		}),
	); err != nil {
		jsonErr(w, 500, err)
		return
	}

	if r.URL.Query().Get("raw") == "true" {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(buf)
		return
	}

	jsonResp(w, 200, map[string]any{
		"format": "jpeg",
		"base64": buf,
	})
}

// ── GET /text ──────────────────────────────────────────────

func handleText(w http.ResponseWriter, r *http.Request) {
	tabID := r.URL.Query().Get("tabId")

	ctx, _, err := tabContext(tabID)
	if err != nil {
		jsonErr(w, 404, err)
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, actionTimeout)
	defer tCancel()

	var text string
	if err := chromedp.Run(tCtx,
		chromedp.Evaluate(`document.body.innerText`, &text),
	); err != nil {
		jsonErr(w, 500, err)
		return
	}

	var url, title string
	chromedp.Run(tCtx,
		chromedp.Location(&url),
		chromedp.Title(&title),
	)

	jsonResp(w, 200, map[string]any{
		"url":   url,
		"title": title,
		"text":  text,
	})
}

// ── POST /navigate ─────────────────────────────────────────

func handleNavigate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TabID string `json:"tabId"`
		URL   string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, err)
		return
	}
	if req.URL == "" {
		jsonResp(w, 400, map[string]string{"error": "url required"})
		return
	}

	ctx, resolvedTabID, err := tabContext(req.TabID)
	if err != nil {
		jsonErr(w, 404, err)
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, 30*time.Second)
	defer tCancel()

	if err := chromedp.Run(tCtx, chromedp.Navigate(req.URL)); err != nil {
		jsonErr(w, 500, err)
		return
	}

	// Invalidate snapshot cache — page changed
	bridge.mu.Lock()
	delete(bridge.snapshots, resolvedTabID)
	bridge.mu.Unlock()

	var url, title string
	chromedp.Run(tCtx,
		chromedp.Location(&url),
		chromedp.Title(&title),
	)

	jsonResp(w, 200, map[string]any{"url": url, "title": title})
}

// ── POST /action ───────────────────────────────────────────

func handleAction(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TabID    string `json:"tabId"`
		Kind     string `json:"kind"`
		Ref      string `json:"ref"`
		Selector string `json:"selector"`
		Text     string `json:"text"`
		Key      string `json:"key"`
		NodeID   int64  `json:"nodeId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, err)
		return
	}

	ctx, resolvedTabID, err := tabContext(req.TabID)
	if err != nil {
		jsonErr(w, 404, err)
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, actionTimeout)
	defer tCancel()

	// Resolve ref to backendNodeID from cached snapshot
	if req.Ref != "" && req.NodeID == 0 && req.Selector == "" {
		bridge.mu.RLock()
		cache := bridge.snapshots[resolvedTabID]
		bridge.mu.RUnlock()
		if cache != nil {
			if nid, ok := cache.refs[req.Ref]; ok {
				req.NodeID = nid
			}
		}
		if req.NodeID == 0 {
			jsonResp(w, 400, map[string]string{
				"error": fmt.Sprintf("ref %s not found — take a /snapshot first", req.Ref),
			})
			return
		}
	}

	var sel string
	if req.Selector != "" {
		sel = req.Selector
	}

	var result map[string]any

	switch req.Kind {
	case "click":
		if sel != "" {
			err = chromedp.Run(tCtx, chromedp.Click(sel, chromedp.ByQuery))
		} else if req.NodeID > 0 {
			err = clickByNodeID(tCtx, req.NodeID)
		} else {
			jsonResp(w, 400, map[string]string{"error": "need selector, ref, or nodeId"})
			return
		}
		result = map[string]any{"clicked": true}

	case "type":
		if req.Text == "" {
			jsonResp(w, 400, map[string]string{"error": "text required for type"})
			return
		}
		if sel != "" {
			err = chromedp.Run(tCtx,
				chromedp.Click(sel, chromedp.ByQuery),
				chromedp.SendKeys(sel, req.Text, chromedp.ByQuery),
			)
		} else if req.NodeID > 0 {
			err = typeByNodeID(tCtx, req.NodeID, req.Text)
		} else {
			jsonResp(w, 400, map[string]string{"error": "need selector or ref"})
			return
		}
		result = map[string]any{"typed": req.Text}

	case "fill":
		if sel != "" {
			err = chromedp.Run(tCtx,
				chromedp.SetValue(sel, req.Text, chromedp.ByQuery),
			)
		}
		result = map[string]any{"filled": req.Text}

	case "press":
		if req.Key == "" {
			jsonResp(w, 400, map[string]string{"error": "key required for press"})
			return
		}
		err = chromedp.Run(tCtx, chromedp.KeyEvent(req.Key))
		result = map[string]any{"pressed": req.Key}

	case "focus":
		if sel != "" {
			err = chromedp.Run(tCtx, chromedp.Focus(sel, chromedp.ByQuery))
		} else if req.NodeID > 0 {
			err = chromedp.Run(tCtx,
				chromedp.ActionFunc(func(ctx context.Context) error {
					p := map[string]any{"backendNodeId": req.NodeID}
					return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.focus", p, nil)
				}),
			)
		}
		result = map[string]any{"focused": true}

	default:
		jsonResp(w, 400, map[string]string{"error": fmt.Sprintf("unknown action: %s", req.Kind)})
		return
	}

	if err != nil {
		jsonErr(w, 500, err)
		return
	}

	jsonResp(w, 200, result)
}

// ── POST /evaluate ─────────────────────────────────────────

func handleEvaluate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TabID      string `json:"tabId"`
		Expression string `json:"expression"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, err)
		return
	}
	if req.Expression == "" {
		jsonResp(w, 400, map[string]string{"error": "expression required"})
		return
	}

	ctx, _, err := tabContext(req.TabID)
	if err != nil {
		jsonErr(w, 404, err)
		return
	}

	tCtx, tCancel := context.WithTimeout(ctx, actionTimeout)
	defer tCancel()

	var result any
	if err := chromedp.Run(tCtx,
		chromedp.Evaluate(req.Expression, &result),
	); err != nil {
		jsonErr(w, 500, err)
		return
	}

	jsonResp(w, 200, map[string]any{"result": result})
}

// ── POST /tab ──────────────────────────────────────────────

func handleTab(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action string `json:"action"` // "new" or "close"
		TabID  string `json:"tabId"`
		URL    string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, 400, err)
		return
	}

	switch req.Action {
	case "new":
		ctx, cancel := chromedp.NewContext(bridge.browserCtx)

		url := "about:blank"
		if req.URL != "" {
			url = req.URL
		}
		if err := chromedp.Run(ctx, chromedp.Navigate(url)); err != nil {
			cancel()
			jsonErr(w, 500, err)
			return
		}

		newTargetID := string(chromedp.FromContext(ctx).Target.TargetID)
		bridge.mu.Lock()
		bridge.tabs[newTargetID] = &TabEntry{ctx: ctx, cancel: cancel}
		bridge.mu.Unlock()

		var curURL, title string
		chromedp.Run(ctx, chromedp.Location(&curURL), chromedp.Title(&title))
		jsonResp(w, 200, map[string]any{"tabId": newTargetID, "url": curURL, "title": title})

	case "close":
		if req.TabID == "" {
			jsonResp(w, 400, map[string]string{"error": "tabId required"})
			return
		}

		// Clean up from registry
		bridge.mu.Lock()
		if entry, ok := bridge.tabs[req.TabID]; ok {
			if entry.cancel != nil {
				entry.cancel()
			}
			delete(bridge.tabs, req.TabID)
			delete(bridge.snapshots, req.TabID)
		}
		bridge.mu.Unlock()

		ctx, cancel := chromedp.NewContext(bridge.browserCtx,
			chromedp.WithTargetID(target.ID(req.TabID)),
		)
		defer cancel()
		if err := chromedp.Run(ctx, page.Close()); err != nil {
			jsonErr(w, 500, err)
			return
		}
		jsonResp(w, 200, map[string]any{"closed": true})

	default:
		jsonResp(w, 400, map[string]string{"error": "action must be 'new' or 'close'"})
	}
}

// ── Node-based actions (use backendNodeID directly) ────────

func clickByNodeID(ctx context.Context, backendNodeID int64) error {
	return chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			p := map[string]any{"backendNodeId": backendNodeID}
			var result json.RawMessage
			if err := chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.resolveNode", p, &result); err != nil {
				return fmt.Errorf("DOM.resolveNode: %v", err)
			}
			var resp struct {
				Object struct {
					ObjectID string `json:"objectId"`
				} `json:"object"`
			}
			if err := json.Unmarshal(result, &resp); err != nil {
				return err
			}
			if resp.Object.ObjectID == "" {
				return fmt.Errorf("no objectId for node %d", backendNodeID)
			}
			callP := map[string]any{
				"objectId":            resp.Object.ObjectID,
				"functionDeclaration": "function() { this.click(); }",
				"arguments":           []any{},
			}
			return chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.callFunctionOn", callP, nil)
		}),
	)
}

func typeByNodeID(ctx context.Context, backendNodeID int64, text string) error {
	return chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			p := map[string]any{"backendNodeId": backendNodeID}
			return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.focus", p, nil)
		}),
		chromedp.KeyEvent(text),
	)
}

// ── Tab context helper ─────────────────────────────────────

// tabContext returns the chromedp context for a tab and the resolved tabID.
// If tabID is empty, uses the first page target.
func tabContext(tabID string) (context.Context, string, error) {
	bridge.mu.Lock()
	defer bridge.mu.Unlock()

	if tabID == "" {
		targets, err := listTargets()
		if err != nil {
			return nil, "", err
		}
		if len(targets) == 0 {
			return nil, "", fmt.Errorf("no tabs open")
		}
		tabID = string(targets[0].TargetID)
	}

	// Check registry first
	if entry, ok := bridge.tabs[tabID]; ok {
		return entry.ctx, tabID, nil
	}

	// Create new context for this target
	ctx, cancel := chromedp.NewContext(bridge.browserCtx,
		chromedp.WithTargetID(target.ID(tabID)),
	)
	if err := chromedp.Run(ctx); err != nil {
		cancel()
		return nil, "", fmt.Errorf("tab %s not found: %v", tabID, err)
	}

	bridge.tabs[tabID] = &TabEntry{ctx: ctx, cancel: cancel}
	return ctx, tabID, nil
}

// cleanStaleTabs periodically removes tab entries whose targets no longer exist.
func (b *Bridge) cleanStaleTabs(interval time.Duration) {
	for {
		time.Sleep(interval)

		targets, err := listTargets()
		if err != nil {
			continue
		}

		alive := make(map[string]bool, len(targets))
		for _, t := range targets {
			alive[string(t.TargetID)] = true
		}

		b.mu.Lock()
		for id, entry := range b.tabs {
			if !alive[id] {
				if entry.cancel != nil {
					entry.cancel()
				}
				delete(b.tabs, id)
				delete(b.snapshots, id)
				log.Printf("Cleaned stale tab: %s", id)
			}
		}
		b.mu.Unlock()
	}
}

func listTargets() ([]*target.Info, error) {
	var targets []*target.Info
	if err := chromedp.Run(bridge.browserCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			targets, err = target.GetTargets().Do(ctx)
			return err
		}),
	); err != nil {
		return nil, err
	}

	pages := make([]*target.Info, 0)
	for _, t := range targets {
		if t.Type == "page" {
			pages = append(pages, t)
		}
	}
	return pages, nil
}

// ── Session persistence ────────────────────────────────────

func saveState() {
	targets, err := listTargets()
	if err != nil {
		log.Printf("Failed to save state: %v", err)
		return
	}

	tabs := make([]TabState, 0, len(targets))
	for _, t := range targets {
		if t.URL != "" && t.URL != "about:blank" && t.URL != "chrome://newtab/" {
			tabs = append(tabs, TabState{
				ID:    string(t.TargetID),
				URL:   t.URL,
				Title: t.Title,
			})
		}
	}

	state := SessionState{
		Tabs:    tabs,
		SavedAt: time.Now().UTC().Format(time.RFC3339),
	}

	data, _ := json.MarshalIndent(state, "", "  ")
	path := filepath.Join(stateDir, "sessions.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		log.Printf("Failed to write state: %v", err)
	} else {
		log.Printf("Saved %d tabs to %s", len(tabs), path)
	}
}

func restoreState() {
	path := filepath.Join(stateDir, "sessions.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var state SessionState
	if err := json.Unmarshal(data, &state); err != nil {
		return
	}

	restored := 0
	for _, tab := range state.Tabs {
		// Skip problematic URLs
		if strings.Contains(tab.URL, "/sorry/") || strings.Contains(tab.URL, "about:blank") {
			continue
		}
		ctx, cancel := chromedp.NewContext(bridge.browserCtx)
		// Timeout per tab — don't let one hung page block startup
		tCtx, tCancel := context.WithTimeout(ctx, 10*time.Second)
		if err := chromedp.Run(tCtx, chromedp.Navigate(tab.URL)); err != nil {
			tCancel()
			cancel()
			log.Printf("Failed to restore tab %s: %v", tab.URL, err)
			continue
		}
		tCancel()
		newID := string(chromedp.FromContext(ctx).Target.TargetID)
		bridge.tabs[newID] = &TabEntry{ctx: ctx, cancel: cancel}
		restored++
	}
	if restored > 0 {
		log.Printf("Restored %d tabs from previous session", restored)
	}
}

// ── Middleware ──────────────────────────────────────────────

func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token != "" {
			auth := r.Header.Get("Authorization")
			if auth != "Bearer "+token {
				jsonResp(w, 401, map[string]string{"error": "unauthorized"})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ── Helpers ────────────────────────────────────────────────

func jsonResp(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
}

func jsonErr(w http.ResponseWriter, code int, err error) {
	jsonResp(w, code, map[string]string{"error": err.Error()})
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func homeDir() string {
	h, _ := os.UserHomeDir()
	return h
}
