package apiclient

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

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
	r := request{method: "GET", url: buildURL(base, path, params)}
	status, body := mustRequest(client, token, r)
	exitOnAPIError(r, status, body)
	return body
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

// DoPostRaw sends a POST and returns the raw response body without printing.
// Exits on HTTP errors.
func DoPostRaw(client *http.Client, base, token, path string, body map[string]any) []byte {
	statusCode, respBody, _ := doPostQuietWithStatus(client, base, token, path, body, nil)
	exitOnAPIError(request{method: "POST", url: base + path, body: body}, statusCode, respBody)
	return respBody
}

// DoPostRawE sends a POST and returns an error instead of terminating the
// process. Long-running commands use it when they need to release resources
// before reporting a request failure.
func DoPostRawE(client *http.Client, base, token, path string, body map[string]any) ([]byte, error) {
	r := request{method: "POST", url: base + path, body: body}
	statusCode, respBody, err := doRequest(client, token, r)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if statusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("%s", strings.TrimSpace(renderAPIError(r, statusCode, respBody)))
	}
	return respBody, nil
}

// DoGetRawE sends a GET and returns an error instead of terminating the
// process. See DoPostRawE.
func DoGetRawE(client *http.Client, base, token, path string, params url.Values) ([]byte, error) {
	r := request{method: "GET", url: buildURL(base, path, params)}
	statusCode, respBody, err := doRequest(client, token, r)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if statusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("%s", strings.TrimSpace(renderAPIError(r, statusCode, respBody)))
	}
	return respBody, nil
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
