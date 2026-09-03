package instance

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

// BridgeClient makes HTTP calls to a bridge instance.
// Each method targets a specific bridge endpoint.
type BridgeClient struct {
	client *http.Client

	// authorize stamps the credential a bridge instance requires. A spawned
	// instance runs the same auth middleware as its parent, so an unauthenticated
	// call to it is answered 401 — and the one caller of FetchTabs swallows that
	// at debug level, so the tab→instance discovery it feeds simply found nothing
	// and said so nowhere. Nil means "no credential", which is what a test stub
	// and an unauthenticated bridge want.
	authorize func(*http.Request)
}

// NewBridgeClient creates a BridgeClient that sends no credential.
func NewBridgeClient() *BridgeClient {
	return NewBridgeClientWithAuth(nil)
}

// NewBridgeClientWithAuth creates a BridgeClient that runs authorize over every
// request it builds. The orchestrator supplies one that resolves the target
// instance and applies that instance's own token, since an attached external
// bridge does not share the server's.
func NewBridgeClientWithAuth(authorize func(*http.Request)) *BridgeClient {
	return &BridgeClient{
		client:    &http.Client{Timeout: httpx.MaxNavigationHTTPDuration},
		authorize: authorize,
	}
}

// authorized applies the configured credential to a request the client built.
func (bc *BridgeClient) authorized(req *http.Request) *http.Request {
	if bc.authorize != nil {
		bc.authorize(req)
	}
	return req
}

// FetchTabs implements TabFetcher by querying a bridge's /tabs endpoint.
func (bc *BridgeClient) FetchTabs(instanceURL string) ([]bridge.InstanceTab, error) {
	req, err := http.NewRequest(http.MethodGet, instanceURL+"/tabs", nil)
	if err != nil {
		return nil, fmt.Errorf("fetch tabs request: %w", err)
	}
	resp, err := bc.client.Do(bc.authorized(req))
	if err != nil {
		return nil, fmt.Errorf("fetch tabs: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if err := statusError(resp, "fetch tabs"); err != nil {
		return nil, err
	}

	// Bridge returns {"tabs": [...]}
	var wrapper struct {
		Tabs []bridge.InstanceTab `json:"tabs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return nil, fmt.Errorf("decode tabs: %w", err)
	}
	return wrapper.Tabs, nil
}

// CreateTab creates a new tab on a bridge instance. Returns the tab ID.
func (bc *BridgeClient) CreateTab(ctx context.Context, port, url string) (string, error) {
	// Create blank tab first to avoid waitFor issues
	body := `{"action":"new","url":"about:blank"}`
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, bridgeURL(port, "/tab"), strings.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create tab request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := bc.client.Do(bc.authorized(req))
	if err != nil {
		return "", fmt.Errorf("create tab: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if err := statusError(resp, "create tab"); err != nil {
		return "", err
	}

	var result struct {
		TabID string `json:"tabId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode create tab response: %w", err)
	}

	if url != "" && url != "about:blank" {
		if err := bc.NavigateTab(ctx, port, result.TabID, url); err != nil {
			return "", fmt.Errorf("navigate after create: %w", err)
		}
	}

	return result.TabID, nil
}

// NavigateTab navigates an existing tab to a URL
func (bc *BridgeClient) NavigateTab(ctx context.Context, port, tabID, url string) error {
	body := fmt.Sprintf(`{"url":%q,"waitFor":"dom"}`, url)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, bridgeURL(port, "/tabs/"+tabID+"/navigate"), strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("navigate request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := bc.client.Do(bc.authorized(req))
	if err != nil {
		return fmt.Errorf("navigate: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if err := statusError(resp, "navigate"); err != nil {
		return err
	}

	return nil
}

// CloseTab closes a tab on a bridge instance.
func (bc *BridgeClient) CloseTab(ctx context.Context, port, tabID string) error {
	body := fmt.Sprintf(`{"tabId":%q}`, tabID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, bridgeURL(port, "/close"), strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("close tab request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := bc.client.Do(bc.authorized(req))
	if err != nil {
		return fmt.Errorf("close tab: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if err := statusError(resp, "close tab"); err != nil {
		return err
	}
	return nil
}

// SnapshotTab calls GET /tabs/{tabID}/snapshot on the bridge to populate
// the snapshot cache. The response body is discarded.
func (bc *BridgeClient) SnapshotTab(ctx context.Context, port, tabID string) {
	url := bridgeURL(port, "/tabs/"+tabID+"/snapshot")
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return
	}
	resp, err := bc.client.Do(bc.authorized(req))
	if err != nil {
		return
	}
	_ = resp.Body.Close()
}

// ProxyWithTabID proxies a request to a bridge shorthand endpoint (e.g. /find),
// injecting the tabId into the JSON request body so the bridge knows which tab
// to operate on. Used for endpoints that don't support /tabs/{id}/... paths.
func (bc *BridgeClient) ProxyWithTabID(w http.ResponseWriter, r *http.Request, port, tabID, path string) {
	var body map[string]any
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			body = map[string]any{}
		}
	} else {
		body = map[string]any{}
	}
	body["tabId"] = tabID

	encoded, err := json.Marshal(body)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, fmt.Errorf("encode body: %w", err))
		return
	}

	targetURL := bridgeURL(port, path)
	proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, strings.NewReader(string(encoded)))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, fmt.Errorf("proxy request: %w", err))
		return
	}
	proxyReq.Header.Set("Content-Type", "application/json")
	// This hop re-encodes the body and so builds a fresh request rather than copying the
	// caller's, which means it tells the instance nothing unless asked to. The request id
	// is forwarded explicitly — and only the request id, so re-encoding does not become a
	// back door around the headers the copying hops deliberately strip.
	httpx.ForwardRequestID(proxyReq.Header, r.Header)

	resp, err := bc.client.Do(proxyReq)
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, fmt.Errorf("proxy failed: %w", err))
		return
	}
	defer func() { _ = resp.Body.Close() }()

	httpx.CopyProxiedResponseHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// ProxyToTab forwards an HTTP request to a specific bridge tab endpoint.
// It builds the URL as http://localhost:{port}/tabs/{tabID}/{suffix} and
// copies the request method, body, and headers.
func (bc *BridgeClient) ProxyToTab(w http.ResponseWriter, r *http.Request, port, tabID, suffix string) {
	targetURL := bridgeURL(port, "/tabs/"+tabID+suffix)
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, r.Body)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, fmt.Errorf("proxy request: %w", err))
		return
	}

	for key, values := range r.Header {
		if httpx.IsHopByHopHeader(key) {
			continue
		}
		for _, v := range values {
			proxyReq.Header.Add(key, v)
		}
	}

	resp, err := bc.client.Do(proxyReq)
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, fmt.Errorf("proxy failed: %w", err))
		return
	}
	defer func() { _ = resp.Body.Close() }()

	httpx.CopyProxiedResponseHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func bridgeURL(port, path string) string {
	return "http://localhost:" + port + path
}

// statusError returns a "<label>: status <code>: <body>" error when resp is not
// 200, or nil otherwise. Centralizes the status-check + error-body read that the
// typed bridge calls (fetch/create/navigate/close) each repeated. The caller
// still owns closing resp.Body.
func statusError(resp *http.Response, label string) error {
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("%s: status %d: %s", label, resp.StatusCode, body)
}
