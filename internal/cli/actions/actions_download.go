package actions

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"

	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
)

const downloadDataPreviewLimit = 256

func Download(client *http.Client, base, token string, args []string, output string) {
	if len(args) < 1 {
		cli.Fatal("Usage: pinchtab download <url> [-o <file>]")
	}

	targetURL := args[0]

	params := url.Values{}
	params.Set("url", targetURL)

	body := apiclient.DoGetRaw(client, base, token, "/download", params)
	if body == nil {
		return
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		fmt.Println(string(body))
		return
	}

	printDownloadResult(result)

	// If -o flag set, decode base64 and save to file
	if output != "" {
		b64, _ := result["data"].(string)
		if b64 == "" {
			cli.Fatal("No base64 data in response")
		}
		data, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			cli.Fatal("Failed to decode base64: %v", err)
		}
		if err := os.WriteFile(output, data, 0600); err != nil {
			cli.Fatal("Write failed: %v", err)
		}
		fmt.Println(cli.StyleStdout(cli.SuccessStyle, fmt.Sprintf("Saved %s (%d bytes)", output, len(data))))
	}
}

func printDownloadResult(result map[string]any) {
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
		fmt.Println("{}")
		return
	}
	fmt.Println(string(formatted))
}
