package staticfetch

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
	"github.com/pinchtab/pinchtab/internal/browserops"
	"github.com/pinchtab/pinchtab/internal/urls"
	nethtml "golang.org/x/net/html"
)

var ErrStaticNotSupported = errors.New("operation not supported by static browser")

type liteTab struct {
	window html.Window
	url    string
	refMap map[string]dom.Element
}

type Browser struct {
	client  *http.Client
	tabs    map[string]*liteTab
	current string // active tab ID
	seq     int    // tab ID sequence counter
	mu      sync.Mutex
}

func NewBrowser() *Browser {
	return &Browser{
		client: &http.Client{Timeout: 30 * time.Second},
		tabs:   make(map[string]*liteTab),
	}
}

func (l *Browser) Name() string { return "static" }

func (l *Browser) Capabilities() []browserops.Capability {
	return []browserops.Capability{browserops.CapNavigate, browserops.CapSnapshot, browserops.CapText, browserops.CapClick, browserops.CapType}
}

func (l *Browser) Navigate(ctx context.Context, url string) (*browserops.NavigateResult, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// SSRF is enforced before this call (handler) and again at dial/redirect
	// time, so an inline nav guard here is intentionally omitted.
	safeURL, err := urls.Sanitize(url)
	if err != nil {
		return nil, fmt.Errorf("lite navigate: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, safeURL, nil)
	if err != nil {
		return nil, fmt.Errorf("lite navigate: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; PinchTab-StaticFetch/1.0)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,*/*")

	resp, err := l.clientForNavigate(ctx).Do(req)
	if err != nil {
		return nil, fmt.Errorf("lite navigate fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("lite navigate: HTTP %d from %s", resp.StatusCode, url)
	}

	ct := resp.Header.Get("Content-Type")
	if ct != "" && !strings.Contains(ct, "html") && !strings.Contains(ct, "xml") {
		return nil, fmt.Errorf("lite navigate: unsupported content type %q", ct)
	}

	// Strip <script> elements to prevent gost-dom panics (no JS runtime).
	cleanBody, err := stripScripts(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("lite navigate strip scripts: %w", err)
	}

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

	return &browserops.NavigateResult{
		TabID: tabID,
		URL:   url,
		Title: title,
	}, nil
}

func (l *Browser) Snapshot(_ context.Context, tabID, filter string) (*browserops.SnapshotResult, error) {
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

	title := l.getTitle(tab.window)

	return &browserops.SnapshotResult{
		Nodes: nodes,
		URL:   tab.url,
		Title: title,
	}, nil
}

func (l *Browser) Text(_ context.Context, tabID string) (*browserops.TextResult, error) {
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

	raw := body.TextContent()
	title := l.getTitle(tab.window)

	return &browserops.TextResult{
		Text:  normalizeWhitespace(raw),
		URL:   tab.url,
		Title: title,
	}, nil
}

func (l *Browser) Click(ctx context.Context, tabID, ref string) (retErr error) {
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
	// to a page with scripts, but no JS runtime is configured).
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

func (l *Browser) Type(_ context.Context, tabID, ref, text string) error {
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

func (l *Browser) TabURL(tabID string) (string, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	tab := l.tabs[tabID]
	if tab == nil {
		return "", false
	}
	return tab.url, true
}

// CloseTab releases a single tab's window and bookkeeping. Unknown IDs are a
// no-op so callers can close defensively; reports whether a tab was removed.
func (l *Browser) CloseTab(tabID string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	tab := l.tabs[tabID]
	if tab == nil {
		return false
	}
	if tab.window != nil {
		tab.window.Close()
	}
	delete(l.tabs, tabID)
	if l.current == tabID {
		l.current = ""
	}
	return true
}

func (l *Browser) Close() error {
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

func (l *Browser) resolveTab(tabID string) (*liteTab, error) {
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

// stripScripts removes <script> elements from HTML to prevent gost-dom
// from panicking when no JavaScript runtime is configured.
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
		case nethtml.SelfClosingTagToken:
			// `<script src="x"/>`: browsers don't honor self-closing script,
			// but gost-dom's reader may — skip it so no script element can
			// reach the JS-less DOM.
			tn, _ := z.TagName()
			if string(tn) == "script" {
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
