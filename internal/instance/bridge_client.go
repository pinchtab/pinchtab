package instance

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

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

// statusError returns a "<label>: status <code>: <body>" error when resp is not
// 200, or nil otherwise. The caller still owns closing resp.Body.
func statusError(resp *http.Response, label string) error {
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("%s: status %d: %s", label, resp.StatusCode, body)
}
