package actions

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spf13/cobra"
)

func storageTestCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "probe"}
	cmd.Flags().String("type", "", "")
	cmd.Flags().String("key", "", "")
	cmd.Flags().Bool("all", false, "")
	cmd.Flags().String("tab", "", "")
	return cmd
}

func TestStorageKeyComesFromThePositionalOrTheFlag(t *testing.T) {
	var gotQuery, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("key")
		body, _ := io.ReadAll(r.Body)
		var decoded map[string]any
		_ = json.Unmarshal(body, &decoded)
		gotBody, _ = decoded["key"].(string)
		_, _ = w.Write([]byte(`{"origin":"http://x","local":{}}`))
	}))
	defer srv.Close()

	cases := []struct {
		name       string
		positional string
		flag       string
		want       string
	}{
		{"positional", "k1", "", "k1"},
		{"flag", "", "k2", "k2"},
		{"positional wins over the flag", "k1", "k2", "k1"},
		{"neither", "", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := storageTestCommand()
			_ = cmd.Flags().Set("key", tc.flag)
			StorageGet(srv.Client(), srv.URL, "", cmd, tc.positional)
			if gotQuery != tc.want {
				t.Fatalf("get sent key %q, want %q", gotQuery, tc.want)
			}
			StorageDelete(srv.Client(), srv.URL, "", cmd, tc.positional)
			if gotBody != tc.want {
				t.Fatalf("delete sent key %q, want %q", gotBody, tc.want)
			}
		})
	}
}
