package actions

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/pinchtab/pinchtab/internal/cli/output"
	"github.com/spf13/cobra"
)

// Box retrieves the bounding box of an element identified by ref.
func Box(conn apiclient.Conn, cmd *cobra.Command, args []string) {
	if len(args) == 0 {
		output.Error("box", "ref argument is required", 1)
		return
	}
	ref := args[0]

	params := url.Values{}
	params.Set("ref", ref)
	if v, _ := cmd.Flags().GetString("tab"); v != "" {
		params.Set("tabId", v)
	}

	jsonOutput, _ := cmd.Flags().GetBool("json")
	if jsonOutput {
		conn.Get("/box", params)
		return
	}

	body := conn.GetRaw("/box", params)
	var result struct {
		Box struct {
			X      float64 `json:"x"`
			Y      float64 `json:"y"`
			Width  float64 `json:"width"`
			Height float64 `json:"height"`
			Top    float64 `json:"top"`
			Right  float64 `json:"right"`
			Bottom float64 `json:"bottom"`
			Left   float64 `json:"left"`
		} `json:"box"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		output.Value(string(body))
		return
	}
	output.Value(fmt.Sprintf("x:%.0f y:%.0f width:%.0f height:%.0f top:%.0f right:%.0f bottom:%.0f left:%.0f",
		result.Box.X, result.Box.Y, result.Box.Width, result.Box.Height,
		result.Box.Top, result.Box.Right, result.Box.Bottom, result.Box.Left))
}
