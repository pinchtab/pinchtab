package actions

import (
	"fmt"
	"net/url"
	"time"

	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

func PDF(conn apiclient.Conn, cmd *cobra.Command) {
	params := url.Values{}
	params.Set("raw", "true")

	outFile, _ := cmd.Flags().GetString("output")
	tabID, _ := cmd.Flags().GetString("tab")

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
	if v, _ := cmd.Flags().GetBool("file-output"); v {
		params.Del("raw")
		params.Set("output", "file")
	}
	if v, _ := cmd.Flags().GetString("path"); v != "" {
		params.Set("path", v)
	}

	autoNamed := outFile == ""
	if autoNamed {
		outFile = fmt.Sprintf("page-%s.pdf", time.Now().Format("20060102-150405"))
	}

	data := conn.GetRaw(apiclient.TabPath(tabID, "pdf"), params)
	if data == nil {
		return
	}
	saved, err := writeOutputFile(outFile, autoNamed, data)
	if err != nil {
		cli.Fatal("Write failed: %v", err)
	}
	printSaved(saved, len(data))
}
