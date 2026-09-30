package actions

import (
	"encoding/base64"
	"os"
	"path/filepath"

	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
)

func Upload(conn apiclient.Conn, args []string, selector, tabID string) {
	if len(args) < 1 {
		cli.Fatal("Usage: pinchtab upload <file-path> [--selector <css>] [--tab <id>]")
	}

	var files, fileNames []string
	for _, path := range args {
		data, err := os.ReadFile(path)
		if err != nil {
			cli.Fatal("Failed to read %s: %v", path, err)
		}
		files = append(files, base64.StdEncoding.EncodeToString(data))
		fileNames = append(fileNames, filepath.Base(path))
	}

	body := map[string]any{
		"files":     files,
		"fileNames": fileNames,
	}
	if selector != "" {
		body["selector"] = selector
	}

	path := apiclient.TabPath(tabID, "upload")

	conn.Post(path, body)
}
