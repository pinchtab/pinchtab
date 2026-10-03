package actions

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

// refLabelDigits extracts the numeric suffix the overlay renders (e.g.
// "e7" -> "7"). Falls back to the whole ref when there are no digits.
func refLabelDigits(ref string) string {
	out := make([]byte, 0, len(ref))
	for i := 0; i < len(ref); i++ {
		c := ref[i]
		if c >= '0' && c <= '9' {
			out = append(out, c)
		} else if len(out) > 0 {
			break
		}
	}
	if len(out) == 0 {
		return ref
	}
	return string(out)
}

func Screenshot(conn apiclient.Conn, cmd *cobra.Command) {
	params := url.Values{}

	outFile, _ := cmd.Flags().GetString("output")
	annotate, _ := cmd.Flags().GetBool("annotate")
	format, _ := cmd.Flags().GetString("format")
	if format == "" {
		if strings.EqualFold(filepath.Ext(outFile), ".png") {
			format = "png"
		} else {
			format = "jpeg"
		}
	}
	if v, _ := cmd.Flags().GetString("quality"); v != "" {
		params.Set("quality", v)
	}
	if v, _ := cmd.Flags().GetString("selector"); v != "" {
		params.Set("selector", v)
	}
	if v, _ := cmd.Flags().GetString("scale"); v != "" {
		params.Set("scale", v)
	}
	if v, _ := cmd.Flags().GetBool("beyond-viewport"); v {
		params.Set("beyondViewport", "true")
	}
	if v, _ := cmd.Flags().GetString("tab"); v != "" {
		params.Set("tabId", v)
	}
	if format != "" && format != "jpeg" {
		params.Set("format", format)
	}

	ext := ".jpg"
	if format == "png" {
		ext = ".png"
	}

	if annotate {
		params.Set("annotate", "true")
		raw := conn.GetRaw("/screenshot", params, apiclient.CaptureVocab(params.Get("tabId") == ""))
		if raw == nil {
			return
		}
		var envelope struct {
			Format      string `json:"format"`
			Base64      string `json:"base64"`
			Annotations []struct {
				Ref  string `json:"ref"`
				Role string `json:"role"`
				Name string `json:"name"`
				Tag  string `json:"tag"`
			} `json:"annotations"`
		}
		if err := json.Unmarshal(raw, &envelope); err != nil {
			cli.Fatal("Decode failed: %v", err)
		}
		img, err := base64.StdEncoding.DecodeString(envelope.Base64)
		if err != nil {
			cli.Fatal("Decode image: %v", err)
		}
		autoNamed := outFile == ""
		if autoNamed {
			if outFile, err = autoNamedOutputPath("screenshot", ext); err != nil {
				cli.Fatal("Output path: %v", err)
			}
		}
		saved, err := writeOutputFile(outFile, autoNamed, img)
		if err != nil {
			cli.Fatal("Write failed: %v", err)
		}
		printSaved(saved, len(img))
		// Print a human-readable legend so the operator can correlate visual
		// labels with refs at a glance. The bracketed number must match what
		// the overlay draws (the numeric portion of the ref) — using i+1
		// here would drift when refs start above e0 or filtering skips refs.
		for _, a := range envelope.Annotations {
			line := fmt.Sprintf("[%s] %s", refLabelDigits(a.Ref), a.Ref)
			if a.Role != "" {
				line += " " + a.Role
			}
			if a.Name != "" {
				line += " \"" + a.Name + "\""
			} else if a.Tag != "" {
				line += " <" + a.Tag + ">"
			}
			fmt.Println(line)
		}
		return
	}

	params.Set("raw", "true")
	autoNamed := outFile == ""
	if autoNamed {
		var err error
		if outFile, err = autoNamedOutputPath("screenshot", ext); err != nil {
			cli.Fatal("Output path: %v", err)
		}
	}
	data := conn.GetRaw("/screenshot", params)
	if data == nil {
		return
	}
	saved, err := writeOutputFile(outFile, autoNamed, data)
	if err != nil {
		cli.Fatal("Write failed: %v", err)
	}
	printSaved(saved, len(data))
}
