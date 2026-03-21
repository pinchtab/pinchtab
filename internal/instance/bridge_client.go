package instance

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/httpx"
)

// BridgeClient makes HTTP calls to a bridge instance.
// Each method targets a specific bridge endpoint.
type BridgeClient struct {
	client *http.Client
}

// NewBridgeClient creates a BridgeClient.
func NewBridgeClient() *BridgeClient {
	return &BridgeClient{
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

// FetchTabs implements TabFetcher by querying a bridge's /tabs endpoint.
func (bc *BridgeClient) FetchTabs(instanceURL string) ([]bridge.InstanceTab, error) {
	resp, err := bc.client.Get(instanceURL + "/tabs")
	if err != nil {
		return nil, fmt.Errorf("fetch tabs: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch tabs: status %d", resp.StatusCode)
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

	resp, err := bc.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("create tab: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("create tab: status %d: %s", resp.StatusCode, respBody)
	}

	var result struct {
		TabID string `json:"tabId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode create tab response: %w", err)
	}

	// If URL provided and not about:blank, navigate to it
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

	resp, err := bc.client.Do(req)
	if err != nil {
		return fmt.Errorf("navigate: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("navigate: status %d: %s", resp.StatusCode, respBody)
	}

	return nil
}

// CloseTab closes a tab on a bridge instance.
func (bc *BridgeClient) CloseTab(ctx context.Context, port, tabID string) error {
	body := fmt.Sprintf(`{"action":"close","tabId":%q}`, tabID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, bridgeURL(port, "/tab"), strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("close tab request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := bc.client.Do(req)
	if err != nil {
		return fmt.Errorf("close tab: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("close tab: status %d", resp.StatusCode)
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
	resp, err := bc.client.Do(req)
	if err != nil {
		return
	}
	_ = resp.Body.Close()
}

// ProxyWithTabID proxies a request to a bridge shorthand endpoint (e.g. /find),
// injecting the tabId into the JSON request body so the bridge knows which tab
// to operate on. Used for endpoints that don't support /tabs/{id}/... paths.
func (bc *BridgeClient) ProxyWithTabID(w http.ResponseWriter, r *http.Request, port, tabID, path string) {
	// Read original body and inject tabId.
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

	resp, err := bc.client.Do(proxyReq)
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, fmt.Errorf("proxy failed: %w", err))
		return
	}
	defer func() { _ = resp.Body.Close() }()

	for key, values := range resp.Header {
		for _, v := range values {
			w.Header().Add(key, v)
		}
	}
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
		switch key {
		case "Host", "Connection", "Keep-Alive", "Proxy-Authenticate",
			"Proxy-Authorization", "Te", "Trailers", "Transfer-Encoding", "Upgrade":
		default:
			for _, v := range values {
				proxyReq.Header.Add(key, v)
			}
		}
	}

	resp, err := bc.client.Do(proxyReq)
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, fmt.Errorf("proxy failed: %w", err))
		return
	}
	defer func() { _ = resp.Body.Close() }()

	for key, values := range resp.Header {
		for _, v := range values {
			w.Header().Add(key, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func bridgeURL(port, path string) string {
	return "http://localhost:" + port + path
}
