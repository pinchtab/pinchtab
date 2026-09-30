package actions

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

func newEmulationCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("tab", "", "")
	cmd.Flags().Bool("json", false, "")
	cmd.Flags().Float64("accuracy", 0, "")
	cmd.Flags().Float64("dpr", 0, "")
	cmd.Flags().Bool("mobile", false, "")
	return cmd
}

func TestEmulationSetters(t *testing.T) {
	setters := []struct {
		name     string
		setter   func(apiclient.Conn, *cobra.Command, []string)
		args     []string
		endpoint string
		wantBody map[string]any
		human    string
	}{
		{"geolocation", SetGeolocation, []string{"1.5", "-2.25"}, "emulation/geolocation", map[string]any{"latitude": 1.5, "longitude": -2.25}, "1.500000,-2.250000"},
		{"viewport", SetViewport, []string{"800", "600"}, "emulation/viewport", map[string]any{"width": 800.0, "height": 600.0}, "800x600"},
		{"offline", SetOffline, []string{"true"}, "emulation/offline", map[string]any{"offline": true}, "offline"},
		{"headers", SetHeaders, []string{`{"X-A":"1"}`}, "emulation/headers", map[string]any{"headers": map[string]any{"X-A": "1"}}, "applied"},
		{"media", SetMedia, []string{"prefers-color-scheme", "dark"}, "emulation/media", map[string]any{"feature": "prefers-color-scheme", "value": "dark"}, "ok"},
		{"credentials", SetCredentials, []string{"alice", "s3cret"}, "emulation/credentials", map[string]any{"username": "alice", "password": "s3cret"}, "ok"},
	}
	modes := []struct {
		name     string
		tab      string
		json     bool
		wantPath string
	}{
		{"no tab human", "", false, "/"},
		{"tab json", "tab-1", true, "/tabs/tab-1/"},
	}

	for _, s := range setters {
		for _, mode := range modes {
			t.Run(s.name+"/"+mode.name, func(t *testing.T) {
				m := newMockServer()
				defer m.close()
				cmd := newEmulationCmd()
				_ = cmd.Flags().Set("tab", mode.tab)
				if mode.json {
					_ = cmd.Flags().Set("json", "true")
				}

				out := captureStdout(t, func() { s.setter(apiclient.Conn{Client: m.server.Client(), Base: m.base()}, cmd, s.args) })

				if want := mode.wantPath + s.endpoint; m.lastPath != want {
					t.Errorf("path = %s, want %s", m.lastPath, want)
				}
				var body map[string]any
				if err := json.Unmarshal([]byte(m.lastBody), &body); err != nil {
					t.Fatalf("body %q: %v", m.lastBody, err)
				}
				gotBody, _ := json.Marshal(body)
				wantBody, _ := json.Marshal(s.wantBody)
				if string(gotBody) != string(wantBody) {
					t.Errorf("body = %s, want %s", gotBody, wantBody)
				}

				want := s.human
				if mode.json {
					want = "{\n  \"status\": \"ok\"\n}"
				}
				if got := strings.TrimSpace(out); got != want {
					t.Errorf("output = %q, want %q", got, want)
				}
			})
		}
	}
}
