package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gost-dom/browser/dom"
	"github.com/gost-dom/browser/html"
	gosturl "github.com/gost-dom/browser/url"
	"github.com/pinchtab/pinchtab/internal/urls"
	nethtml "golang.org/x/net/html"
)

var ErrLiteNotSupported = errors.New("operation not supported in lite mode")

// liteTab tracks one Gost-DOM window.
type liteTab struct {
	window html.Window
	url    string
	refMap map[string]dom.Element
}

// LiteEngine implements Engine using Gost-DOM.
type LiteEngine struct {
	client  *http.Client
	tabs    map[string]*liteTab
	current string // active tab ID
	seq     int    // tab ID sequence counter
	mu      sync.Mutex
}

// NewLiteEngine creates a Gost-DOM based engine.
func NewLiteEngine() *LiteEngine {
	return &LiteEngine{
		client: &http.Client{Timeout: 30 * time.Second},
		tabs:   make(map[string]*liteTab),
	}
}

func (l *LiteEngine) Name() string { return "lite" }

func (l *LiteEngine) Capabilities() []Capability {
	return []Capability{CapNavigate, CapSnapshot, CapText, CapClick, CapType}
}

// Navigate opens a URL in the lite engine and returns the result.
func (l *LiteEngine) Navigate(ctx context.Context, url string) (*NavigateResult, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Validate and sanitize URL to prevent SSRF (CodeQL go/request-forgery).
	safeURL, err := urls.Sanitize(url)
	if err != nil {
		return nil, fmt.Errorf("lite navigate: %w", err)
	}

	// Fetch HTML via HTTP.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, safeURL, nil)
	if err != nil {
		return nil, fmt.Errorf("lite navigate: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; PinchTab-Lite/1.0)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,*/*")

	resp, err := l.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("lite navigate fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("lite navigate: HTTP %d from %s", resp.StatusCode, url)
	}

	// Detect content type — only process HTML.
	ct := resp.Header.Get("Content-Type")
	if ct != "" && !strings.Contains(ct, "html") && !strings.Contains(ct, "xml") {
		return nil, fmt.Errorf("lite navigate: unsupported content type %q", ct)
	}

	// Strip <script> elements to prevent gost-dom panics (no JS engine).
	cleanBody, err := stripScripts(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("lite navigate strip scripts: %w", err)
	}

	// Parse the cleaned HTML directly using gost-dom's reader API,
	// avoiding a second HTTP fetch.
	parsedURL := gosturl.ParseURL(url)
	win, err := html.NewWindowReader(cleanBody, parsedURL)
	if err != nil {
		return nil, fmt.Errorf("lite navigate open: %w", err)
	}

	l.seq++
	tabID := fmt.Sprintf("lite-%d", l.seq)
	l.tabs[tabID] = &liteTab{
		window: win,
		url:    url,
		refMap: make(map[string]dom.Element),
	}
	l.current = tabID

	title := l.getTitle(win)

	return &NavigateResult{
		TabID: tabID,
		URL:   url,
		Title: title,
	}, nil
}

// Snapshot returns the DOM tree as snapshot nodes.
func (l *LiteEngine) Snapshot(_ context.Context, tabID, filter string) ([]SnapshotNode, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	tab, err := l.resolveTab(tabID)
	if err != nil {
		return nil, err
	}

	doc := tab.window.Document()
	if doc == nil {
		return nil, errors.New("no document")
	}

	body := doc.Body()
	if body == nil {
		return nil, errors.New("no body element")
	}

	tab.refMap = make(map[string]dom.Element)
	nodes := l.walkDOM(tab, body, filter, 0)
	return nodes, nil
}

// Text returns the visible text content of the page.
func (l *LiteEngine) Text(_ context.Context, tabID string) (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	tab, err := l.resolveTab(tabID)
	if err != nil {
		return "", err
	}

	doc := tab.window.Document()
	if doc == nil {
		return "", errors.New("no document")
	}

	body := doc.Body()
	if body == nil {
		return "", errors.New("no body element")
	}

	raw := body.TextContent()
	return normalizeWhitespace(raw), nil
}

// Click clicks an element identified by ref.
func (l *LiteEngine) Click(ctx context.Context, tabID, ref string) (retErr error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	tab, err := l.resolveTab(tabID)
	if err != nil {
		return err
	}

	el, ok := tab.refMap[ref]
	if !ok {
		return fmt.Errorf("ref %q not found (take a snapshot first)", ref)
	}

	// Recover from gost-dom panics (e.g., anchor click triggers navigation
	// to a page with scripts, but no JS engine is configured).
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("click recovered from panic: %v", r)
		}
	}()

	if htmlEl, ok := el.(html.HTMLElement); ok {
		htmlEl.Click()
		return nil
	}
	return errors.New("element does not support click")
}

// Type enters text into an element identified by ref.
func (l *LiteEngine) Type(_ context.Context, tabID, ref, text string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	tab, err := l.resolveTab(tabID)
	if err != nil {
		return err
	}

	el, ok := tab.refMap[ref]
	if !ok {
		return fmt.Errorf("ref %q not found (take a snapshot first)", ref)
	}

	if input, ok := el.(html.HTMLInputElement); ok {
		input.SetValue(text)
		return nil
	}

	el.SetAttribute("value", text)
	return nil
}

// Close shuts down the lite engine and releases resources.
func (l *LiteEngine) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	for _, tab := range l.tabs {
		if tab.window != nil {
			tab.window.Close()
		}
	}
	l.tabs = make(map[string]*liteTab)
	return nil
}

func (l *LiteEngine) resolveTab(tabID string) (*liteTab, error) {
	if tabID == "" {
		tabID = l.current
	}
	if tabID == "" {
		return nil, errors.New("no page loaded")
	}
	tab := l.tabs[tabID]
	if tab == nil || tab.window == nil {
		return nil, fmt.Errorf("tab %q not found", tabID)
	}
	l.current = tabID
	return tab, nil
}

// ---------- helpers ----------

func (l *LiteEngine) walkDOM(tab *liteTab, node dom.Node, filter string, depth int) []SnapshotNode {
	var nodes []SnapshotNode

	el, isElement := node.(dom.Element)
	if !isElement {
		return nodes
	}

	tag := strings.ToLower(el.TagName())

	// Skip non-visible and script/style elements.
	if tag == "script" || tag == "style" || tag == "noscript" || tag == "link" || tag == "meta" {
		return nodes
	}

	role := getRole(el)
	name := getAccessibleName(el)
	interactive := isInteractive(el)

	if filter == "interactive" && !interactive {
		for child := node.FirstChild(); child != nil; child = child.NextSibling() {
			nodes = append(nodes, l.walkDOM(tab, child, filter, depth)...)
		}
		return nodes
	}

	ref := fmt.Sprintf("e%d", len(tab.refMap))
	tab.refMap[ref] = el

	sn := SnapshotNode{
		Ref:         ref,
		Role:        role,
		Name:        name,
		Tag:         tag,
		Interactive: interactive,
		Depth:       depth,
	}

	if input, ok := el.(html.HTMLInputElement); ok {
		sn.Value = input.Value()
	}

	nodes = append(nodes, sn)

	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		nodes = append(nodes, l.walkDOM(tab, child, filter, depth+1)...)
	}
	return nodes
}

func (l *LiteEngine) getTitle(win html.Window) string {
	if win == nil {
		return ""
	}
	doc := win.Document()
	if doc == nil {
		return ""
	}
	titleEl, err := doc.QuerySelector("title")
	if err != nil || titleEl == nil {
		return ""
	}
	return strings.TrimSpace(titleEl.TextContent())
}

// getRole maps an element to its implicit ARIA role.
func getRole(el dom.Element) string {
	if role, ok := el.GetAttribute("role"); ok {
		return role
	}

	switch strings.ToLower(el.TagName()) {
	case "a":
		if _, has := el.GetAttribute("href"); has {
			return "link"
		}
	case "button":
		return "button"
	case "input":
		t, _ := el.GetAttribute("type")
		switch t {
		case "submit", "button":
			return "button"
		case "checkbox":
			return "checkbox"
		case "radio":
			return "radio"
		default:
			return "textbox"
		}
	case "textarea":
		return "textbox"
	case "select":
		return "combobox"
	case "img":
		return "img"
	case "nav":
		return "navigation"
	case "main":
		return "main"
	case "header":
		return "banner"
	case "footer":
		return "contentinfo"
	case "aside":
		return "complementary"
	case "form":
		return "form"
	case "h1", "h2", "h3", "h4", "h5", "h6":
		return "heading"
	case "ul", "ol":
		return "list"
	case "li":
		return "listitem"
	case "table":
		return "table"
	case "tr":
		return "row"
	case "td":
		return "cell"
	case "th":
		return "columnheader"
	case "section":
		if _, has := el.GetAttribute("aria-label"); has {
			return "region"
		}
		if _, has := el.GetAttribute("aria-labelledby"); has {
			return "region"
		}
	case "details":
		return "group"
	case "summary":
		return "button"
	case "dialog":
		return "dialog"
	case "article":
		return "article"
	case "p", "div", "span":
		return "generic"
	}
	return "generic"
}

// getAccessibleName resolves an element's accessible name.
func getAccessibleName(el dom.Element) string {
	if label, ok := el.GetAttribute("aria-label"); ok {
		return label
	}
	if title, ok := el.GetAttribute("title"); ok {
		return title
	}
	tag := strings.ToLower(el.TagName())
	if tag == "img" {
		if alt, ok := el.GetAttribute("alt"); ok {
			return alt
		}
	}
	if tag == "input" || tag == "textarea" {
		if ph, ok := el.GetAttribute("placeholder"); ok {
			return ph
		}
	}
	if isInteractive(el) {
		text := strings.TrimSpace(el.TextContent())
		if len(text) > 100 {
			text = text[:100] + "..."
		}
		return text
	}
	return ""
}

// isInteractive returns true for elements a user can interact with.
func isInteractive(el dom.Element) bool {
	switch strings.ToLower(el.TagName()) {
	case "a":
		_, has := el.GetAttribute("href")
		return has
	case "button", "input", "textarea", "select", "summary":
		return true
	}
	if _, ok := el.GetAttribute("onclick"); ok {
		return true
	}
	if idx, ok := el.GetAttribute("tabindex"); ok && idx != "-1" {
		return true
	}
	if role, ok := el.GetAttribute("role"); ok {
		switch role {
		case "button", "link", "tab", "menuitem", "switch", "checkbox", "radio":
			return true
		}
	}
	return false
}

// stripScripts removes <script> elements from HTML to prevent gost-dom
// from panicking when no JavaScript engine is configured.
func stripScripts(r io.Reader) (io.Reader, error) {
	z := nethtml.NewTokenizer(r)
	var buf bytes.Buffer
	inScript := false
	for {
		tt := z.Next()
		switch tt {
		case nethtml.ErrorToken:
			if z.Err() == io.EOF {
				return &buf, nil
			}
			return nil, z.Err()
		case nethtml.StartTagToken:
			tn, _ := z.TagName()
			if string(tn) == "script" {
				inScript = true
				continue
			}
			buf.Write(z.Raw())
		case nethtml.EndTagToken:
			tn, _ := z.TagName()
			if string(tn) == "script" {
				inScript = false
				continue
			}
			buf.Write(z.Raw())
		default:
			if !inScript {
				buf.Write(z.Raw())
			}
		}
	}
}

// normalizeWhitespace collapses runs of whitespace (including blank lines)
// into single spaces while trimming leading/trailing space.
func normalizeWhitespace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prev := true // treat start as whitespace
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if !prev {
				b.WriteByte(' ')
				prev = true
			}
			continue
		}
		b.WriteRune(r)
		prev = false
	}
	return strings.TrimSpace(b.String())
}
