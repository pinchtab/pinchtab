package apiclient

import (
	"bytes"
	"encoding/json"
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
		data, _ := json.Marshal(r.body)
		bodyReader = bytes.NewReader(data)
	}
	req, _ := http.NewRequest(r.method, r.url, bodyReader)
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
	body, _ := io.ReadAll(resp.Body)
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
