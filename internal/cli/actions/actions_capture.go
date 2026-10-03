package actions

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/pinchtab/pinchtab/internal/cli/output"
	"github.com/spf13/cobra"
)

// Capture defaults to the agent-friendly terse form (image path +
// epoch/pairing/viewport line + compact snapshot); --json dumps the full
// server envelope.
func Capture(conn apiclient.Conn, cmd *cobra.Command) {
	params := url.Values{}
	params.Set("output", "inline")

	outFile, _ := cmd.Flags().GetString("output")
	jsonOutput, _ := cmd.Flags().GetBool("json")
	outFileExplicit := cmd.Flags().Changed("output")

	format, _ := cmd.Flags().GetString("format")
	if format == "" {
		// Infer from the -o extension when --format is unset, matching screenshot,
		// so `capture -o x.png` writes a real PNG rather than JPEG-in-.png. An
		// explicit --format stays authoritative because it makes format non-empty.
		if strings.EqualFold(filepath.Ext(outFile), ".png") {
			format = "png"
		} else {
			format = "jpeg"
		}
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
	if v, _ := cmd.Flags().GetString("scale"); v != "" {
		params.Set("scale", v)
	}
	if v, _ := cmd.Flags().GetBool("require-pair"); v {
		params.Set("requirePair", "true")
	}
	if cmd.Flags().Changed("with-bounds") {
		if v, _ := cmd.Flags().GetBool("with-bounds"); !v {
			params.Set("withBounds", "false")
		}
	}
	if v, _ := cmd.Flags().GetString("tab"); v != "" {
		params.Set("tabId", v)
	}

	raw := conn.GetRaw("/capture", params, apiclient.CaptureVocab(params.Get("tabId") == ""))
	if raw == nil {
		return
	}

	var resp captureResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		output.Error("capture", fmt.Sprintf("decode response: %v", err), output.ExitRuntime)
		return
	}

	// --json without an explicit --output skips the file write so piping JSON
	// doesn't leave a stray image in pwd. Base64 stays in the envelope.
	ext := ".jpg"
	if resp.Image.Format == "png" {
		ext = ".png"
	}
	writeImage := !jsonOutput || outFileExplicit
	var img []byte
	if writeImage {
		target, err := resolveOutput(outFile, "capture", ext)
		if err != nil {
			output.Error("capture", fmt.Sprintf("output path: %v", err), output.ExitRuntime)
			return
		}
		decoded, err := base64.StdEncoding.DecodeString(resp.Image.Base64)
		if err != nil {
			output.Error("capture", fmt.Sprintf("decode image bytes: %v", err), output.ExitRuntime)
			return
		}
		img = decoded
		saved, err := target.write(img)
		if err != nil {
			output.Error("capture", fmt.Sprintf("write image: %v", err), output.ExitRuntime)
			return
		}
		outFile = saved
	}

	if jsonOutput {
		var pretty map[string]any
		if err := json.Unmarshal(raw, &pretty); err == nil {
			output.JSON(pretty)
		} else {
			fmt.Println(string(raw))
		}
		return
	}

	output.Value(fmt.Sprintf("saved %s (%d bytes)", outFile, len(img)))
	output.Value(fmt.Sprintf("url: %s", resp.URL))
	if resp.Title != "" {
		output.Value(fmt.Sprintf("title: %s", resp.Title))
	}
	output.Value(fmt.Sprintf("epoch: %s navigated=%v duration=%dms",
		resp.Epoch.DomEpoch, resp.Pairing.Navigated, resp.Pairing.CaptureDurationMs))
	output.Value(fmt.Sprintf("viewport: %.0fx%.0f dpr=%g space=%s",
		resp.Image.Viewport.W, resp.Image.Viewport.H, resp.Image.DPR, resp.Image.CoordinateSpace))

	if len(resp.Snapshot.Nodes) > 0 {
		fmt.Println()
		filter := resp.Snapshot.Filter
		if filter == "" {
			filter = "interactive"
		}
		output.Value(fmt.Sprintf("snapshot: %d nodes (%s)", resp.Snapshot.NodeCount, filter))
		for _, n := range resp.Snapshot.Nodes {
			output.Value(formatCaptureNode(n))
		}
	}

	if resp.IDPIWarning != "" {
		output.Hint("idpi: " + resp.IDPIWarning)
	}
}

type captureResponse struct {
	Status     string `json:"status"`
	TabID      string `json:"tabId"`
	URL        string `json:"url"`
	Title      string `json:"title"`
	CapturedAt string `json:"capturedAt"`
	Epoch      struct {
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
		Viewport        struct {
			W, H, ScrollX, ScrollY float64
		} `json:"viewport"`
	} `json:"image"`
	Snapshot struct {
		Filter    string            `json:"filter"`
		NodeCount int               `json:"nodeCount"`
		Nodes     []captureNodeWire `json:"nodes"`
	} `json:"snapshot"`
	IDPIWarning string `json:"idpiWarning,omitempty"`
}

type captureNodeWire struct {
	Ref         string `json:"ref"`
	Role        string `json:"role"`
	Name        string `json:"name"`
	Value       string `json:"value,omitempty"`
	BoundingBox *struct {
		X, Y, W, H float64
	} `json:"boundingBox,omitempty"`
	Visible bool `json:"visible,omitempty"`
}

// formatCaptureNode renders one node like snap's compact line, with bounds
// appended when present: [eN] role "name" (x,y wxh).
func formatCaptureNode(n captureNodeWire) string {
	var b strings.Builder
	b.WriteString("[")
	b.WriteString(n.Ref)
	b.WriteString("] ")
	b.WriteString(n.Role)
	if n.Name != "" {
		b.WriteString(" \"")
		b.WriteString(n.Name)
		b.WriteString("\"")
	}
	if n.BoundingBox != nil {
		fmt.Fprintf(&b, " (%.0f,%.0f %.0fx%.0f)",
			n.BoundingBox.X, n.BoundingBox.Y, n.BoundingBox.W, n.BoundingBox.H)
	}
	return b.String()
}
