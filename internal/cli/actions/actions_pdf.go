package actions

import (
	"encoding/json"
	"net/url"

	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

func PDF(conn apiclient.Conn, cmd *cobra.Command) {
	params := url.Values{}
	params.Set("raw", "true")

	outFile, _ := cmd.Flags().GetString("output")
	tabID, _ := cmd.Flags().GetString("tab")
	fileOutput, _ := cmd.Flags().GetBool("file-output")
	if fileOutput && outFile != "" {
		usageErr("Error: --file-output saves on the server and cannot be combined with -o/--output; use --path to choose the server-side location")
	}

	if v, _ := cmd.Flags().GetBool("landscape"); v {
		params.Set("landscape", "true")
	}
	if v, _ := cmd.Flags().GetString("scale"); v != "" {
		params.Set("scale", v)
	}
	if v, _ := cmd.Flags().GetString("paper-width"); v != "" {
		params.Set("paperWidth", v)
	}
	if v, _ := cmd.Flags().GetString("paper-height"); v != "" {
		params.Set("paperHeight", v)
	}
	if v, _ := cmd.Flags().GetString("margin-top"); v != "" {
		params.Set("marginTop", v)
	}
	if v, _ := cmd.Flags().GetString("margin-bottom"); v != "" {
		params.Set("marginBottom", v)
	}
	if v, _ := cmd.Flags().GetString("margin-left"); v != "" {
		params.Set("marginLeft", v)
	}
	if v, _ := cmd.Flags().GetString("margin-right"); v != "" {
		params.Set("marginRight", v)
	}
	if v, _ := cmd.Flags().GetString("page-ranges"); v != "" {
		params.Set("pageRanges", v)
	}
	if v, _ := cmd.Flags().GetBool("prefer-css-page-size"); v {
		params.Set("preferCSSPageSize", "true")
	}
	if v, _ := cmd.Flags().GetBool("display-header-footer"); v {
		params.Set("displayHeaderFooter", "true")
	}
	if v, _ := cmd.Flags().GetString("header-template"); v != "" {
		params.Set("headerTemplate", v)
	}
	if v, _ := cmd.Flags().GetString("footer-template"); v != "" {
		params.Set("footerTemplate", v)
	}
	if v, _ := cmd.Flags().GetBool("generate-tagged-pdf"); v {
		params.Set("generateTaggedPDF", "true")
	}
	if v, _ := cmd.Flags().GetBool("generate-document-outline"); v {
		params.Set("generateDocumentOutline", "true")
	}
	if fileOutput {
		params.Del("raw")
		params.Set("output", "file")
	}
	if v, _ := cmd.Flags().GetString("path"); v != "" {
		params.Set("path", v)
	}

	if fileOutput {
		printServerSavedPDF(conn.GetRaw(apiclient.TabPath(tabID, "pdf"), params))
		return
	}

	target, err := resolveOutput(outFile, "page", ".pdf")
	if err != nil {
		cli.Fatal("Output path: %v", err)
	}

	data := conn.GetRaw(apiclient.TabPath(tabID, "pdf"), params)
	if data == nil {
		return
	}
	saved, err := target.write(data)
	if err != nil {
		cli.Fatal("Write failed: %v", err)
	}
	printSaved(saved, len(data))
}

func printServerSavedPDF(reply []byte) {
	if reply == nil {
		return
	}
	var saved struct {
		Path string `json:"path"`
		Size int    `json:"size"`
	}
	if err := json.Unmarshal(reply, &saved); err != nil || saved.Path == "" {
		cli.Fatal("Unexpected reply to a server-side PDF save: %s", reply)
	}
	printSaved(saved.Path, saved.Size)
}
