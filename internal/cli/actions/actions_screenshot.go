package actions

import (
	"fmt"
	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
	"net/http"
	"net/url"
	"os"
	"time"
)

func Screenshot(client *http.Client, base, token string, cmd *cobra.Command) {
	params := url.Values{}
	params.Set("raw", "true")

	outFile, _ := cmd.Flags().GetString("output")
	if v, _ := cmd.Flags().GetString("quality"); v != "" {
		params.Set("quality", v)
	}
	if v, _ := cmd.Flags().GetString("selector"); v != "" {
		params.Set("selector", v)
	}
	if v, _ := cmd.Flags().GetBool("css-1x"); v {
		params.Set("css1x", "true")
	}
	if v, _ := cmd.Flags().GetString("tab"); v != "" {
		params.Set("tabId", v)
	}

	if outFile == "" {
		outFile = fmt.Sprintf("screenshot-%s.jpg", time.Now().Format("20060102-150405"))
	}

	data := apiclient.DoGetRaw(client, base, token, "/screenshot", params)
	if data == nil {
		return
	}
	if err := os.WriteFile(outFile, data, 0600); err != nil {
		cli.Fatal("Write failed: %v", err)
	}
	fmt.Println(cli.StyleStdout(cli.SuccessStyle, fmt.Sprintf("Saved %s (%d bytes)", outFile, len(data))))
}
