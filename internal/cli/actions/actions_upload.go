package actions

import (
	"encoding/base64"
	"net/http"
	"net/url"
	"os"

	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
)

func Upload(client *http.Client, base, token string, args []string, selector, tabID string) {
	if len(args) < 1 {
		cli.Fatal("Usage: pinchtab upload <file-path> [--selector <css>] [--tab <id>]")
	}

	var files []string
	for _, path := range args {
		data, err := os.ReadFile(path)
		if err != nil {
			cli.Fatal("Failed to read %s: %v", path, err)
		}
		files = append(files, base64.StdEncoding.EncodeToString(data))
	}

	body := map[string]any{
		"files": files,
	}
	if selector != "" {
		body["selector"] = selector
	}

	path := "/upload"
	if tabID != "" {
		path = "/tabs/" + url.PathEscape(tabID) + "/upload"
	}

	apiclient.DoPost(client, base, token, path, body)
}
