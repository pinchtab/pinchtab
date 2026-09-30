package actions

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"

	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
)

const downloadDataPreviewLimit = 256

func Download(client *http.Client, base, token string, args []string, output, tabID string) {
	if len(args) < 1 {
		cli.Fatal("Usage: pinchtab download <url> [-o <file>] [--tab <id>]")
	}

	targetURL := args[0]

	params := url.Values{}
	params.Set("url", targetURL)

	path := apiclient.TabPath(tabID, "download")

	body := apiclient.DoGetRaw(client, base, token, path, params)
	if body == nil {
		return
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		fmt.Println(string(body))
		return
	}

	if output == "" {
		printDownloadResult(result)
		return
	}

	savedBytes, err := saveDownloadData(result, output)
	if err != nil {
		switch {
		case errors.Is(err, errMissingDownloadData):
			cli.Fatal("No base64 data in response")
		case errors.Is(err, errDecodeDownloadData):
			cli.Fatal("Failed to decode base64: %v", err)
		default:
			cli.Fatal("Write failed: %v", err)
		}
	}
	printSaved(output, savedBytes)
}

func printDownloadResult(result map[string]any) {
	fmt.Println(formatDownloadResult(result))
}

func formatDownloadResult(result map[string]any) string {
	view := make(map[string]any, len(result)+2)
	for k, v := range result {
		view[k] = v
	}

	if b64, ok := view["data"].(string); ok && len(b64) > downloadDataPreviewLimit {
		view["data"] = b64[:downloadDataPreviewLimit] + "... (truncated)"
		view["dataLength"] = len(b64)
		view["dataTruncated"] = true
	}

	formatted, err := json.MarshalIndent(view, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(formatted)
}

var (
	errMissingDownloadData = errors.New("missing download data")
	errDecodeDownloadData  = errors.New("decode download data")
)

func saveDownloadData(result map[string]any, output string) (int, error) {
	b64, _ := result["data"].(string)
	if b64 == "" {
		return 0, errMissingDownloadData
	}
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", errDecodeDownloadData, err)
	}
	if err := os.WriteFile(output, data, 0600); err != nil {
		return 0, err
	}
	return len(data), nil
}
