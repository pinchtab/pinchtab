package apiclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

func DoGet(client *http.Client, base, token, path string, params url.Values) map[string]any {
	u := base + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	req, _ := http.NewRequest("GET", u, nil)
	setClientHeaders(req, token)
	resp, err := client.Do(req)
	if err != nil {
		fatal("Request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		handleAPIError(resp.StatusCode, body)
		os.Exit(1)
	}

	return printAndDecode(body)
}

func DoGetRaw(client *http.Client, base, token, path string, params url.Values) []byte {
	u := base + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	req, _ := http.NewRequest("GET", u, nil)
	setClientHeaders(req, token)
	resp, err := client.Do(req)
	if err != nil {
		fatal("Request failed: %v", err)
		return nil
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		fmt.Fprintf(os.Stderr, "Error %d: %s\n", resp.StatusCode, string(body))
		os.Exit(1)
	}
	return body
}

func DoPost(client *http.Client, base, token, path string, body map[string]any) map[string]any {
	return DoPostWithHeaders(client, base, token, path, body, nil)
}

// DoPostQuiet is like DoPost but does not print the response body. Callers are
// responsible for rendering whatever output is appropriate (e.g. a single
// field for machine-friendly piping).
func DoPostQuiet(client *http.Client, base, token, path string, body map[string]any) map[string]any {
	data, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", base+path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	setClientHeaders(req, token)
	resp, err := client.Do(req)
	if err != nil {
		fatal("Request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		handleAPIError(resp.StatusCode, respBody)
		os.Exit(1)
	}

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		log.Printf("warning: error unmarshaling response: %v", err)
	}
	return result
}

func DoPostWithHeaders(client *http.Client, base, token, path string, body map[string]any, headers map[string]string) map[string]any {
	data, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", base+path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	setClientHeaders(req, token)
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := client.Do(req)
	if err != nil {
		fatal("Request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		handleAPIError(resp.StatusCode, respBody)
		os.Exit(1)
	}

	return printAndDecode(respBody)
}

// DoDelete sends a DELETE request with an optional JSON body (e.g. for ?name= query params, pass nil body and handle params in path).
func DoDelete(client *http.Client, base, token, path string, params url.Values) map[string]any {
	u := base + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	req, _ := http.NewRequest("DELETE", u, nil)
	setClientHeaders(req, token)
	resp, err := client.Do(req)
	if err != nil {
		fatal("Request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		handleAPIError(resp.StatusCode, respBody)
		os.Exit(1)
	}

	return printAndDecode(respBody)
}

// DoDeleteJSON sends a DELETE request with a JSON body.
func DoDeleteJSON(client *http.Client, base, token, path string, body map[string]any) map[string]any {
	data, _ := json.Marshal(body)
	req, _ := http.NewRequest("DELETE", base+path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	setClientHeaders(req, token)
	resp, err := client.Do(req)
	if err != nil {
		fatal("Request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		handleAPIError(resp.StatusCode, respBody)
		os.Exit(1)
	}

	return printAndDecode(respBody)
}

// printAndDecode pretty-prints the body when it is JSON, falls back to
// raw output otherwise, and returns the parsed map (if any) for the
// suggestion logic. It only warns on genuine JSON decode errors — inherently
// non-JSON responses like /snapshot's compact text format pass silently.
func printAndDecode(body []byte) map[string]any {
	var buf bytes.Buffer
	isJSON := json.Indent(&buf, body, "", "  ") == nil
	if isJSON {
		fmt.Println(buf.String())
	} else {
		fmt.Println(string(body))
	}
	var result map[string]any
	if isJSON {
		if err := json.Unmarshal(body, &result); err != nil {
			log.Printf("warning: error unmarshaling response: %v", err)
		}
	}
	return result
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

// handleAPIError parses and displays API error responses with hints
func handleAPIError(statusCode int, body []byte) {
	var errResp struct {
		Error   string         `json:"error"`
		Code    string         `json:"code"`
		Details map[string]any `json:"details"`
	}

	if err := json.Unmarshal(body, &errResp); err != nil {
		// Fallback to raw output if not valid JSON
		fmt.Fprintf(os.Stderr, "Error %d: %s\n", statusCode, string(body))
		return
	}

	// Print main error
	if errResp.Error != "" {
		fmt.Fprintf(os.Stderr, "Error %d: %s\n", statusCode, errResp.Error)
	} else {
		fmt.Fprintf(os.Stderr, "Error %d: %s\n", statusCode, string(body))
	}

	// Print hint and remedy if present
	if errResp.Details != nil {
		if hint, ok := errResp.Details["hint"].(string); ok && hint != "" {
			fmt.Fprintf(os.Stderr, "\n💡 %s\n", hint)
		}
		if remedy, ok := errResp.Details["remedy"].(string); ok && remedy != "" {
			fmt.Fprintf(os.Stderr, "   Remedy: %s\n", remedy)
		}
	}
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
