package apiclient

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

// mustRequest is doRequest with the common fatal-on-transport-error policy.
func mustRequest(client *http.Client, token string, r request) (int, []byte) {
	status, body, err := doRequest(client, token, r)
	if err != nil {
		fatal("Request failed: %v", err)
	}
	return status, body
}

func exitOnAPIError(status int, body []byte) {
	if status >= 400 {
		handleAPIError(status, body)
		os.Exit(1)
	}
}

func ExitWithAPIError(statusCode int, body []byte) {
	handleAPIError(statusCode, body)
	os.Exit(1)
}

func handleAPIError(statusCode int, body []byte) {
	var errResp struct {
		Error   string         `json:"error"`
		Code    string         `json:"code"`
		Details map[string]any `json:"details"`
	}

	if err := json.Unmarshal(body, &errResp); err != nil {
		fmt.Fprintf(os.Stderr, "Error %d: %s\n", statusCode, string(body))
		return
	}

	if errResp.Error != "" {
		fmt.Fprintf(os.Stderr, "Error %d: %s\n", statusCode, errResp.Error)
	} else {
		fmt.Fprintf(os.Stderr, "Error %d: %s\n", statusCode, string(body))
	}

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
