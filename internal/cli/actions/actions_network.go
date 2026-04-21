package actions

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

// Network lists recent network requests.
func Network(client *http.Client, base, token string, cmd *cobra.Command, args []string) {
	// If a positional arg is given, treat it as a requestId detail lookup
	if len(args) > 0 {
		NetworkDetail(client, base, token, cmd, args[0])
		return
	}

	// Check for --clear flag
	if v, _ := cmd.Flags().GetBool("clear"); v {
		NetworkClear(client, base, token, cmd)
		return
	}

	// Check for --stream flag
	if v, _ := cmd.Flags().GetBool("stream"); v {
		NetworkStream(client, base, token, cmd)
		return
	}

	params := url.Values{}
	if v, _ := cmd.Flags().GetString("tab"); v != "" {
		params.Set("tabId", v)
	}
	if v, _ := cmd.Flags().GetString("filter"); v != "" {
		params.Set("filter", v)
	}
	if v, _ := cmd.Flags().GetString("method"); v != "" {
		params.Set("method", v)
	}
	if v, _ := cmd.Flags().GetString("status"); v != "" {
		params.Set("status", v)
	}
	if v, _ := cmd.Flags().GetString("type"); v != "" {
		params.Set("type", v)
	}
	if v, _ := cmd.Flags().GetString("limit"); v != "" {
		params.Set("limit", v)
	}
	if v, _ := cmd.Flags().GetString("buffer-size"); v != "" {
		params.Set("bufferSize", v)
	}

	jsonOutput, _ := cmd.Flags().GetBool("json")
	if jsonOutput {
		apiclient.DoGet(client, base, token, "/network", params)
		return
	}

	body := apiclient.DoGetRaw(client, base, token, "/network", params)
	var resp struct {
		Count   int `json:"count"`
		Entries []struct {
			Method string `json:"method"`
			Status int    `json:"status"`
			URL    string `json:"url"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		fmt.Println(string(body))
		return
	}
	if resp.Count == 0 {
		fmt.Println("No requests captured")
		return
	}
	for _, e := range resp.Entries {
		truncURL := e.URL
		if len(truncURL) > 80 {
			truncURL = truncURL[:77] + "..."
		}
		fmt.Printf("%-6s %3d  %s\n", e.Method, e.Status, truncURL)
	}
}

// NetworkDetail shows full details for a specific request.
func NetworkDetail(client *http.Client, base, token string, cmd *cobra.Command, requestID string) {
	params := url.Values{}
	if v, _ := cmd.Flags().GetString("tab"); v != "" {
		params.Set("tabId", v)
	}
	if v, _ := cmd.Flags().GetBool("body"); v {
		params.Set("body", "true")
	}
	path := fmt.Sprintf("/network/%s", url.PathEscape(requestID))
	apiclient.DoGet(client, base, token, path, params)
}

// NetworkClear clears captured network data.
func NetworkClear(client *http.Client, base, token string, cmd *cobra.Command) {
	params := url.Values{}
	if v, _ := cmd.Flags().GetString("tab"); v != "" {
		params.Set("tabId", v)
	}
	result := apiclient.DoPost(client, base, token, "/network/clear", map[string]any{})
	if result == nil {
		fmt.Fprintln(os.Stderr, "Failed to clear network data")
	}
}

// NetworkStream connects to the SSE stream and prints entries as they arrive.
func NetworkStream(client *http.Client, base, token string, cmd *cobra.Command) {
	params := url.Values{}
	if v, _ := cmd.Flags().GetString("tab"); v != "" {
		params.Set("tabId", v)
	}
	if v, _ := cmd.Flags().GetString("filter"); v != "" {
		params.Set("filter", v)
	}
	if v, _ := cmd.Flags().GetString("method"); v != "" {
		params.Set("method", v)
	}
	if v, _ := cmd.Flags().GetString("status"); v != "" {
		params.Set("status", v)
	}
	if v, _ := cmd.Flags().GetString("type"); v != "" {
		params.Set("type", v)
	}
	if v, _ := cmd.Flags().GetString("buffer-size"); v != "" {
		params.Set("bufferSize", v)
	}

	u := base + "/network/stream"
	if len(params) > 0 {
		u += "?" + params.Encode()
	}

	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", "text/event-stream")

	// Use a client without timeout for streaming
	streamClient := &http.Client{}
	resp, err := streamClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to stream: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort cleanup

	if resp.StatusCode != 200 {
		fmt.Fprintf(os.Stderr, "Error: HTTP %d\n", resp.StatusCode)
		os.Exit(1)
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			fmt.Println(line[6:])
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Stream error: %v\n", err)
		os.Exit(1)
	}
}
