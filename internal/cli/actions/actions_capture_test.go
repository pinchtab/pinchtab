package actions

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

func TestCaptureThreadsExplicitTabIntoPairedRequest(t *testing.T) {
	m := newMockServer()
	m.response = `{"status":"ok","tabId":"tab-target","image":{"format":"png","base64":"aW1n"},"snapshot":{"nodeCount":1,"nodes":[{"ref":"e1","role":"button","name":"Save"}]},"pairing":{"navigated":false}}`
	defer m.close()

	cmd := &cobra.Command{}
	cmd.Flags().Bool("json", false, "")
	cmd.Flags().String("tab", "", "")
	_ = cmd.Flags().Set("json", "true")
	_ = cmd.Flags().Set("tab", "tab-target")

	out := captureStdout(t, func() {
		Capture(apiclient.Conn{Client: m.server.Client(), Base: m.base()}, cmd)
	})
	if m.lastPath != "/capture" || !strings.Contains(m.lastQuery, "tabId=tab-target") {
		t.Fatalf("paired capture request = %s?%s, want /capture?tabId=tab-target", m.lastPath, m.lastQuery)
	}
	for _, want := range []string{`"tabId": "tab-target"`, `"base64": "aW1n"`, `"name": "Save"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("capture JSON missing %s: %s", want, out)
		}
	}
}

// capture must infer the image format from the -o extension when --format is
// unset, matching screenshot, so `capture -o x.png` writes real PNG bytes rather
// than JPEG-in-.png. The server here reflects the requested format into the image
// it returns, so the written file's magic bytes prove the whole chain: the CLI
// inferred the format, sent it, and wrote what came back.
func TestCaptureInfersFormatFromOutputExtension(t *testing.T) {
	pngMagic := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
	jpegMagic := []byte{0xff, 0xd8, 0xff, 0xe0}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		magic, format := jpegMagic, "jpeg"
		if r.URL.Query().Get("format") == "png" {
			magic, format = pngMagic, "png"
		}
		resp := map[string]any{
			"status":  "ok",
			"image":   map[string]any{"format": format, "base64": base64.StdEncoding.EncodeToString(magic)},
			"pairing": map[string]any{"navigated": false},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	newCaptureCmd := func() *cobra.Command {
		cmd := &cobra.Command{}
		cmd.Flags().String("output", "", "")
		cmd.Flags().String("format", "", "")
		cmd.Flags().Bool("json", false, "")
		cmd.Flags().String("tab", "", "")
		return cmd
	}

	for _, tc := range []struct {
		name      string
		outName   string
		setFormat string
		wantMagic []byte
	}{
		{"png extension infers png", "x.png", "", pngMagic},
		{"jpg extension defaults jpeg", "x.jpg", "", jpegMagic},
		{"uppercase .PNG infers png", "x.PNG", "", pngMagic},
		{"explicit format overrides extension", "x.png", "jpeg", jpegMagic},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), tc.outName)
			cmd := newCaptureCmd()
			_ = cmd.Flags().Set("output", out)
			if tc.setFormat != "" {
				_ = cmd.Flags().Set("format", tc.setFormat)
			}
			_ = captureStdout(t, func() {
				Capture(apiclient.Conn{Client: srv.Client(), Base: srv.URL}, cmd)
			})
			got, err := os.ReadFile(out)
			if err != nil {
				t.Fatalf("read output: %v", err)
			}
			if !bytes.HasPrefix(got, tc.wantMagic) {
				t.Fatalf("written bytes %x do not carry the magic %x for the format the -o extension implies", got, tc.wantMagic)
			}
		})
	}

	// No -o at all still defaults to JPEG (auto-named in the working dir).
	t.Run("no output defaults to jpeg", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		cmd := newCaptureCmd()
		_ = captureStdout(t, func() {
			Capture(apiclient.Conn{Client: srv.Client(), Base: srv.URL}, cmd)
		})
		matches, _ := filepath.Glob(filepath.Join(dir, "capture-*.jpg"))
		if len(matches) != 1 {
			t.Fatalf("want exactly one auto-named .jpg, got %v", matches)
		}
		got, err := os.ReadFile(matches[0])
		if err != nil {
			t.Fatalf("read auto-named file: %v", err)
		}
		if !bytes.HasPrefix(got, jpegMagic) {
			t.Fatalf("auto-named default is not JPEG: %x", got)
		}
	})
}
