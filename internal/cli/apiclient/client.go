package apiclient

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const vocabHeader = "X-PinchTab-Vocab"

// DoGetCapturingVocab performs a GET like DoGet and, on success, persists the
// response's vocabulary token for (base, tabKey) so a later action on that tab
// echoes it and a ref renumbered by an intervening snapshot is refused rather
// than mis-resolved. The token is delivered as a response header so it survives
// every snapshot format, including the compact text the CLI defaults to.
func DoGetCapturingVocab(client *http.Client, base, token, path string, params url.Values, tabKey string) map[string]any {
	var headers http.Header
	r := request{method: "GET", url: buildURL(base, path, params), respHeaders: &headers}
	status, body := mustRequest(client, token, r)
	exitOnAPIError(r, status, body)
	storeVocabToken(base, tabKey, headers.Get(vocabHeader))
	return printAndDecode(body)
}

// VocabTokenFor returns the last vocabulary token stored for (base, tabKey), or "".
func VocabTokenFor(base, tabKey string) string {
	data, err := os.ReadFile(vocabTokenPath(base, tabKey))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func storeVocabToken(base, tabKey, token string) {
	if token == "" {
		return
	}
	path := vocabTokenPath(base, tabKey)
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	_ = os.WriteFile(path, []byte(token+"\n"), 0644)
}

func vocabTokenPath(base, tabKey string) string {
	dir := os.Getenv("XDG_STATE_HOME")
	if dir != "" {
		dir += "/pinchtab"
	} else if home, err := os.UserHomeDir(); err == nil {
		dir = home + "/.local/state/pinchtab"
	} else {
		dir = "/tmp/pinchtab"
	}
	key := tabKey
	if key == "" {
		key = "default"
	}
	return filepath.Join(dir, "vocab-"+fileSlug(base)+"-"+fileSlug(key))
}

func fileSlug(s string) string {
	s = strings.TrimPrefix(strings.TrimPrefix(s, "http://"), "https://")
	s = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-':
			return r
		default:
			return '-'
		}
	}, strings.Trim(s, "/"))
	if s == "" {
		return "default"
	}
	return s
}

// doAndRender runs the request with the standard fatal-on-transport-error +
// exit-on-HTTP-error policy, then pretty-prints and decodes the body.
func doAndRender(client *http.Client, token string, r request) map[string]any {
	status, body := mustRequest(client, token, r)
	exitOnAPIError(r, status, body)
	return printAndDecode(body)
}

func DoGet(client *http.Client, base, token, path string, params url.Values) map[string]any {
	return doAndRender(client, token, request{method: "GET", url: buildURL(base, path, params)})
}

func DoGetRaw(client *http.Client, base, token, path string, params url.Values) []byte {
	return doRaw(client, token, request{method: "GET", url: buildURL(base, path, params)})
}

func doRaw(client *http.Client, token string, r request) []byte {
	status, body := mustRequest(client, token, r)
	exitOnAPIError(r, status, body)
	return body
}

func doRequestE(client *http.Client, token string, r request) ([]byte, error) {
	statusCode, respBody, err := doRequest(client, token, r)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if statusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("%s", strings.TrimSpace(renderAPIError(r, statusCode, respBody)))
	}
	return respBody, nil
}

// DoGetRawAndPrint fetches and prints the raw response body (for --snap flag).
// Best-effort: it reports errors to stderr but does not exit.
func DoGetRawAndPrint(client *http.Client, base, token, pathWithQuery string) {
	status, body, err := doRequest(client, token, request{method: "GET", url: base + pathWithQuery})
	if err != nil {
		fmt.Fprintf(os.Stderr, "snapshot failed: %v\n", err)
		return
	}
	if status >= 400 {
		fmt.Fprintf(os.Stderr, "snapshot error %d: %s\n", status, string(body))
		return
	}
	fmt.Println(string(body))
}

func DoPost(client *http.Client, base, token, path string, body map[string]any) map[string]any {
	return DoPostWithHeaders(client, base, token, path, body, nil)
}

// DoPostQuiet is like DoPost but does not print the response body. Callers are
// responsible for rendering whatever output is appropriate (e.g. a single
// field for machine-friendly piping).
func DoPostQuiet(client *http.Client, base, token, path string, body map[string]any) map[string]any {
	return DoPostQuietWithHeaders(client, base, token, path, body, nil)
}

func DoPostRaw(client *http.Client, base, token, path string, body map[string]any) []byte {
	return doRaw(client, token, request{method: "POST", url: base + path, body: body})
}

func DoPostRawE(client *http.Client, base, token, path string, body map[string]any) ([]byte, error) {
	return doRequestE(client, token, request{method: "POST", url: base + path, body: body})
}

func DoGetRawE(client *http.Client, base, token, path string, params url.Values) ([]byte, error) {
	return doRequestE(client, token, request{method: "GET", url: buildURL(base, path, params)})
}

func DoPostQuietWithStatus(client *http.Client, base, token, path string, body map[string]any) (int, []byte, map[string]any) {
	return doPostQuietWithStatus(client, base, token, path, body, nil)
}

// DoPostQuietWithHeaders is like DoPostQuiet but allows custom headers.
func DoPostQuietWithHeaders(client *http.Client, base, token, path string, body map[string]any, headers map[string]string) map[string]any {
	statusCode, respBody, result := doPostQuietWithStatus(client, base, token, path, body, headers)
	exitOnAPIError(request{method: "POST", url: base + path, body: body, headers: headers}, statusCode, respBody)
	return result
}

func doPostQuietWithStatus(client *http.Client, base, token, path string, body map[string]any, headers map[string]string) (int, []byte, map[string]any) {
	status, respBody := mustRequest(client, token, request{method: "POST", url: base + path, body: body, headers: headers})

	var result map[string]any
	if status < 400 {
		// Object responses populate result; array/scalar responses leave it nil.
		// Callers that need a map should branch on result == nil.
		_ = json.Unmarshal(respBody, &result)
	}
	return status, respBody, result
}

func DoPostWithHeaders(client *http.Client, base, token, path string, body map[string]any, headers map[string]string) map[string]any {
	return doAndRender(client, token, request{method: "POST", url: base + path, body: body, headers: headers})
}

// DoDelete sends a DELETE request with an optional JSON body (e.g. for ?name= query params, pass nil body and handle params in path).
func DoDelete(client *http.Client, base, token, path string, params url.Values) map[string]any {
	return doAndRender(client, token, request{method: "DELETE", url: buildURL(base, path, params)})
}

// DoDeleteJSON sends a DELETE request with a JSON body.
func DoDeleteJSON(client *http.Client, base, token, path string, body map[string]any) map[string]any {
	return doAndRender(client, token, request{method: "DELETE", url: base + path, body: body})
}

// ResolveInstanceBase fetches the named instance from the orchestrator and returns
// a base URL pointing directly at that instance's API port.
func ResolveInstanceBase(orchBase, token, instanceID, bind string) string {
	c := &http.Client{Timeout: 10 * time.Second}
	body := DoGetRaw(c, orchBase, token, fmt.Sprintf("/instances/%s", instanceID), nil)

	var inst struct {
		Port string `json:"port"`
	}
	if err := json.Unmarshal(body, &inst); err != nil {
		fatal("failed to parse instance %q: %v", instanceID, err)
	}
	if inst.Port == "" {
		fatal("instance %q has no port assigned (is it still starting?)", instanceID)
	}
	return fmt.Sprintf("http://%s:%s", bind, inst.Port)
}
