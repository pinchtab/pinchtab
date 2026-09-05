package apiclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// request describes a single API call. body is a JSON payload (nil = no body;
// Content-Type is set only when body is non-nil). headers are extra per-call
// headers applied after the standard client headers.
type request struct {
	method  string
	url     string
	body    map[string]any
	headers map[string]string
	// respHeaders, when non-nil, receives the response headers so a caller can
	// read one (the vocabulary token) without changing doRequest's return shape,
	// which mustRequest and the other verbs share.
	respHeaders *http.Header
}

// requestIDHeader is the correlation id the server stamps on every response. Declared
// here rather than imported: the CLI is a client of the wire, and the test pins the
// bytes against httpx.RequestIDHeader.
const requestIDHeader = "X-Request-Id"

// lastRequestID is the id the server used for the most recent response, recorded at
// the one transport funnel so every error renderer can print it. It is cleared before
// each request, so a failure with no response never carries a stale id.
var lastRequestID string

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
		data, _ := json.Marshal(r.body)
		bodyReader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(r.method, r.url, bodyReader)
	if err != nil {
		return 0, nil, err
	}
	if r.body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	setClientHeaders(req, token)
	for key, value := range r.headers {
		req.Header.Set(key, value)
	}
	lastRequestID = ""
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	lastRequestID = resp.Header.Get(requestIDHeader)
	if r.respHeaders != nil {
		*r.respHeaders = resp.Header
	}
	// A short read is a failed request, never a body: a connection dropping
	// mid-response would otherwise reach the caller as a fragment carrying its
	// status, and the CLI would parse or print the fragment as the answer.
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("read response from %s: %w", r.url, err)
	}
	return resp.StatusCode, body, nil
}

func setClientHeaders(req *http.Request, token string) {
	req.Header.Set("X-PinchTab-Source", "client")
	if token == "" {
		return
	}
	if strings.HasPrefix(token, "ses_") {
		req.Header.Set("Authorization", "Session "+token)
	} else {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}
