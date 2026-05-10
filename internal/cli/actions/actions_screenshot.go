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

func Screenshot(client *http.Client, base, token string, cmd *cobra.Command) {
	params := url.Values{}

	outFile, _ := cmd.Flags().GetString("output")
	annotate, _ := cmd.Flags().GetBool("annotate")
	format, _ := cmd.Flags().GetString("format")
	if format == "" {
		format = "jpeg"
	}
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
	if format != "" && format != "jpeg" {
		params.Set("format", format)
	}

	ext := ".jpg"
	if format == "png" {
		ext = ".png"
	}

	if annotate {
		params.Set("annotate", "true")
		raw := apiclient.DoGetRaw(client, base, token, "/screenshot", params)
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
		if outFile == "" {
			outFile = fmt.Sprintf("screenshot-%s%s", time.Now().Format("20060102-150405"), ext)
		}
		if err := os.WriteFile(outFile, img, 0600); err != nil {
			cli.Fatal("Write failed: %v", err)
		}
		fmt.Println(cli.StyleStdout(cli.SuccessStyle, fmt.Sprintf("Saved %s (%d bytes)", outFile, len(img))))
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
	if outFile == "" {
		outFile = fmt.Sprintf("screenshot-%s%s", time.Now().Format("20060102-150405"), ext)
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
