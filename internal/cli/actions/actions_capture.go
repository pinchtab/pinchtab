package actions

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

// Capture is the CLI shim for `pinchtab capture`. Requests an inline
// (base64) capture so we can both write the image locally and print the
// snapshot summary in one round trip. Mirrors how `pinchtab screenshot`
// localizes the file.
func Capture(client *http.Client, base, token string, cmd *cobra.Command) {
	params := url.Values{}
	params.Set("output", "inline")

	outFile, _ := cmd.Flags().GetString("output")
	format, _ := cmd.Flags().GetString("format")
	if format == "" {
		format = "jpeg"
	}
	if format != "jpeg" {
		params.Set("format", format)
	}

	if v, _ := cmd.Flags().GetString("quality"); v != "" {
		params.Set("quality", v)
	}
	if v, _ := cmd.Flags().GetString("selector"); v != "" {
		params.Set("selector", v)
	}
	if v, _ := cmd.Flags().GetString("filter"); v != "" {
		params.Set("filter", v)
	}
	if v, _ := cmd.Flags().GetString("depth"); v != "" {
		params.Set("depth", v)
	}
	if v, _ := cmd.Flags().GetString("wait"); v != "" {
		params.Set("wait", v)
	}
	if v, _ := cmd.Flags().GetBool("beyond-viewport"); v {
		params.Set("beyondViewport", "true")
	}
	if v, _ := cmd.Flags().GetBool("require-pair"); v {
		params.Set("requirePair", "true")
	}
	// --with-bounds defaults true on the server; explicit false suppresses.
	if cmd.Flags().Changed("with-bounds") {
		if v, _ := cmd.Flags().GetBool("with-bounds"); !v {
			params.Set("withBounds", "false")
		}
	}
	if v, _ := cmd.Flags().GetString("tab"); v != "" {
		params.Set("tabId", v)
	}

	raw := apiclient.DoGetRaw(client, base, token, "/capture", params)
	if raw == nil {
		return
	}

	var resp struct {
		Status string `json:"status"`
		TabID  string `json:"tabId"`
		URL    string `json:"url"`
		Title  string `json:"title"`
		Epoch  struct {
			FrameID  string `json:"frameId"`
			LoaderID string `json:"loaderId"`
			DomEpoch string `json:"domEpoch"`
		} `json:"epoch"`
		Pairing struct {
			Navigated         bool  `json:"navigated"`
			CaptureDurationMs int64 `json:"captureDurationMs"`
		} `json:"pairing"`
		Image struct {
			Format          string  `json:"format"`
			Base64          string  `json:"base64"`
			Bytes           int     `json:"bytes"`
			CoordinateSpace string  `json:"coordinateSpace"`
			DPR             float64 `json:"devicePixelRatio"`
		} `json:"image"`
		Snapshot struct {
			Filter    string `json:"filter"`
			NodeCount int    `json:"nodeCount"`
		} `json:"snapshot"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		cli.Fatal("Decode capture response: %v", err)
	}

	ext := ".jpg"
	if resp.Image.Format == "png" {
		ext = ".png"
	}
	if outFile == "" {
		outFile = fmt.Sprintf("capture-%s%s", time.Now().Format("20060102-150405"), ext)
	}
	img, err := base64.StdEncoding.DecodeString(resp.Image.Base64)
	if err != nil {
		cli.Fatal("Decode image bytes: %v", err)
	}
	if err := os.WriteFile(outFile, img, 0600); err != nil {
		cli.Fatal("Write image: %v", err)
	}

	fmt.Println(cli.StyleStdout(cli.SuccessStyle,
		fmt.Sprintf("Saved %s (%d bytes, %s)", outFile, len(img), resp.Image.CoordinateSpace)))
	fmt.Printf("  url:      %s\n", resp.URL)
	fmt.Printf("  title:    %s\n", resp.Title)
	fmt.Printf("  epoch:    %s (frame=%s loader=%s)\n",
		resp.Epoch.DomEpoch, short(resp.Epoch.FrameID), short(resp.Epoch.LoaderID))
	fmt.Printf("  pairing:  navigated=%v  duration=%dms\n",
		resp.Pairing.Navigated, resp.Pairing.CaptureDurationMs)
	fmt.Printf("  snapshot: %d nodes (filter=%q)\n",
		resp.Snapshot.NodeCount, resp.Snapshot.Filter)
}

func short(s string) string {
	if len(s) <= 8 {
		return s
	}
	return s[:8] + "…"
}
