package apiclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/pinchtab/pinchtab/internal/api/types"
	"github.com/pinchtab/pinchtab/internal/authn"
)

// request describes a single API call. body is a JSON payload (nil = no body;
// Content-Type is set only when body is non-nil). headers are extra per-call
// headers applied after the standard client headers.
type request struct {
	method  string
	url     string
	query   url.Values
	body    map[string]any
	headers map[string]string
	base    string
	quiet   bool
	vocab   *vocabCapture
}

type vocabCapture struct {
	implicit bool
}

type RequestOption func(*request)

// CaptureVocab persists a successful response's vocabulary token keyed by the tab
// the server resolved (the X-PinchTab-Tab-Id header), not by how the caller spelled
// --tab. When implicit (no --tab), the resolved tab also becomes the store's
// current pointer, so a later implicit action echoes that tab's token. The token
// is delivered as a response header so it survives every snapshot format,
// including the compact text the CLI defaults to.
func CaptureVocab(implicit bool) RequestOption {
	return func(r *request) { r.vocab = &vocabCapture{implicit: implicit} }
}

func WithQuery(params url.Values) RequestOption {
	return func(r *request) { r.query = params }
}

func WithBody(body map[string]any) RequestOption {
	return func(r *request) { r.body = body }
}

func WithHeaders(headers map[string]string) RequestOption {
	return func(r *request) { r.headers = headers }
}

func Quiet() RequestOption {
	return func(r *request) { r.quiet = true }
}

func newRequest(method, base, path string, opts ...RequestOption) request {
	r := request{method: method, base: base}
	for _, opt := range opts {
		opt(&r)
	}
	r.url = buildURL(base, path, r.query)
	return r
}

func buildURL(base, path string, params url.Values) string {
	u := base + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	return u
}

// doRequest builds and executes the request and reads the body. It does NOT
// interpret the status code or print anything — callers apply their own
// error/render policy.
func doRequest(client *http.Client, token string, r request) (int, []byte, error) {
	var bodyReader io.Reader
	if r.body != nil {
		data, err := json.Marshal(r.body)
		if err != nil {
			return 0, nil, fmt.Errorf("encode request body for %s: %w", r.url, err)
		}
		bodyReader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(r.method, r.url, bodyReader)
	if err != nil {
		return 0, nil, fmt.Errorf("build request: %w", err)
	}
	if r.body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	setClientHeaders(req, token)
	for key, value := range r.headers {
		req.Header.Set(key, value)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	// A short read is a failed request, never a body: a connection dropping
	// mid-response would otherwise reach the caller as a fragment carrying its
	// status, and the CLI would parse or print the fragment as the answer.
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("read response from %s: %w", r.url, err)
	}
	if r.vocab != nil && resp.StatusCode < http.StatusBadRequest {
		storeVocabToken(r.base, resp.Header.Get(vocabTabIDHeader), resp.Header.Get(vocabHeader), r.vocab.implicit)
	}
	return resp.StatusCode, body, nil
}

func setClientHeaders(req *http.Request, token string) {
	req.Header.Set(types.HeaderSource, "client")
	authn.SetAuthorization(req.Header, token)
}

func OpenStream(client *http.Client, base, token, path string, params url.Values) (*http.Response, error) {
	u := buildURL(base, path, params)
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("build stream request: %w", err)
	}
	setClientHeaders(req, token)
	req.Header.Set("Accept", "text/event-stream")
	streamClient := &http.Client{Transport: client.Transport}
	resp, err := streamClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("open stream %s: %w", u, err)
	}
	return resp, nil
}

func TabPath(tabID, suffix string) string {
	suffix = "/" + strings.TrimPrefix(suffix, "/")
	if tabID == "" {
		return suffix
	}
	return fmt.Sprintf("/tabs/%s%s", url.PathEscape(tabID), suffix)
}
