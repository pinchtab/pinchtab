package actions

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

func RecordStart(client *http.Client, base, token string, cmd *cobra.Command, args []string) {
	outFile := args[0]

	ext := strings.ToLower(filepath.Ext(outFile))
	var format string
	switch ext {
	case ".gif":
		format = "gif"
	case ".webm":
		format = "webm"
	case ".mp4":
		format = "mp4"
	default:
		cli.Fatal("Unsupported format %q — use .gif, .webm, or .mp4", ext)
	}

	fps, _ := cmd.Flags().GetInt("fps")
	quality, _ := cmd.Flags().GetInt("quality")
	scale, _ := cmd.Flags().GetFloat64("scale")
	tab, _ := cmd.Flags().GetString("tab")

	body := map[string]any{
		"format":  format,
		"fps":     fps,
		"quality": quality,
		"scale":   scale,
	}
	if tab != "" {
		body["tabId"] = tab
	}

	apiclient.DoPost(client, base, token, "/record/start", body)

	writeRecordingState(outFile)
	fmt.Println(cli.StyleStdout(cli.SuccessStyle, fmt.Sprintf("Recording started → %s (%s, %d fps)", outFile, format, fps)))
}

func RecordStop(client *http.Client, base, token string) {
	outFile := readRecordingState()
	if outFile == "" {
		outFile = fmt.Sprintf("recording-%s.gif", time.Now().Format("20060102-150405"))
	}

	data := apiclient.DoPostRaw(client, base, token, "/record/stop", map[string]any{})
	if data == nil {
		return
	}

	if err := os.WriteFile(outFile, data, 0600); err != nil {
		cli.Fatal("Write failed: %v", err)
	}
	clearRecordingState()
	fmt.Println(cli.StyleStdout(cli.SuccessStyle, fmt.Sprintf("Saved %s (%d bytes)", outFile, len(data))))
}

func RecordStatus(client *http.Client, base, token string) {
	raw := apiclient.DoGetRaw(client, base, token, "/record/status", nil)
	if raw == nil {
		return
	}

	var status struct {
		Active   bool    `json:"active"`
		Format   string  `json:"format"`
		Duration float64 `json:"durationSeconds"`
		Frames   int     `json:"frames"`
		TabID    string  `json:"tabId"`
		FPS      int     `json:"fps"`
	}
	if err := json.Unmarshal(raw, &status); err != nil {
		cli.Fatal("Decode failed: %v", err)
	}

	if !status.Active {
		fmt.Println(cli.StyleStdout(cli.MutedStyle, "No active recording"))
		return
	}

	fmt.Printf("Recording: %s @ %d fps  |  %.1fs  |  %d frames  |  tab %s\n",
		status.Format, status.FPS, status.Duration, status.Frames, status.TabID)
}

func recordingStateFile() string {
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		return dir + "/pinchtab/current-recording"
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.TempDir()
	}
	return home + "/.local/state/pinchtab/current-recording"
}

func writeRecordingState(outFile string) {
	path := recordingStateFile()
	dir := filepath.Dir(path)
	_ = os.MkdirAll(dir, 0700)
	tmp, err := os.CreateTemp(dir, ".current-recording-*")
	if err != nil {
		_ = os.WriteFile(path, []byte(outFile+"\n"), 0600)
		return
	}
	_, _ = tmp.WriteString(outFile + "\n")
	_ = tmp.Chmod(0600)
	tmpName := tmp.Name()
	_ = tmp.Close()
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
	}
}

func readRecordingState() string {
	data, err := os.ReadFile(recordingStateFile())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func clearRecordingState() {
	_ = os.Remove(recordingStateFile())
}
