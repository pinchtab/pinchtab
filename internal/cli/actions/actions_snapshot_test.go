package actions

import (
	"net/url"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/pinchtab/pinchtab/internal/handlers"
	"github.com/spf13/cobra"
)

func newSnapshotCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("interactive", true, "")
	cmd.Flags().Bool("compact", true, "")
	cmd.Flags().Bool("json", false, "")
	cmd.Flags().Bool("full", false, "")
	cmd.Flags().Bool("text", false, "")
	cmd.Flags().Bool("diff", false, "")
	cmd.Flags().String("selector", "", "")
	cmd.Flags().String("max-tokens", "", "")
	cmd.Flags().String("depth", "", "")
	cmd.Flags().String("tab", "", "")
	return cmd
}

func TestSnapshotFormatFollowsTheFlagsInOnePlace(t *testing.T) {
	for _, tc := range []struct {
		name  string
		flags map[string]string
		want  string
	}{
		{name: "no flags at all", flags: nil, want: "compact"},
		{name: "compact declined", flags: map[string]string{"compact": "false"}, want: "json"},
		{name: "json asked for", flags: map[string]string{"json": "true"}, want: "json"},
		{name: "full", flags: map[string]string{"full": "true"}, want: "json"},
		{name: "text", flags: map[string]string{"text": "true"}, want: "text"},
		{name: "text beats json", flags: map[string]string{"text": "true", "json": "true"}, want: "text"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newMockServer()
			defer m.close()

			cmd := newSnapshotCmd()
			for flag, value := range tc.flags {
				if err := cmd.Flags().Set(flag, value); err != nil {
					t.Fatalf("cannot set --%s: %v", flag, err)
				}
			}
			Snapshot(apiclient.Conn{Client: m.server.Client(), Base: m.base()}, cmd, "")

			query, err := url.ParseQuery(m.lastQuery)
			if err != nil {
				t.Fatalf("unreadable query %q: %v", m.lastQuery, err)
			}
			if got := query.Get("format"); got != tc.want {
				t.Errorf("snap asked for format=%q, want %q; the flags decide the cost of the answer, so the wire is what has to say so", got, tc.want)
			}
			if _, err := handlers.ParseSnapshotCostControls(query); err != nil {
				t.Errorf("the server rejects the query snap sends: %v (query=%v)", err, query)
			}
		})
	}
}

func TestSnapshot(t *testing.T) {
	m := newMockServer()
	m.response = `[{"ref":"e0","role":"button","name":"Submit"}]`
	defer m.close()
	client := m.server.Client()

	cmd := newSnapshotCmd()
	_ = cmd.Flags().Set("interactive", "true")
	_ = cmd.Flags().Set("compact", "true")
	Snapshot(apiclient.Conn{Client: client, Base: m.base()}, cmd, "")
	if m.lastMethod != "GET" {
		t.Errorf("expected GET, got %s", m.lastMethod)
	}
	if m.lastPath != "/snapshot" {
		t.Errorf("expected /snapshot, got %s", m.lastPath)
	}
	if !strings.Contains(m.lastQuery, "filter=interactive") {
		t.Errorf("expected filter=interactive in query, got %s", m.lastQuery)
	}
	if !strings.Contains(m.lastQuery, "format=compact") {
		t.Errorf("expected format=compact in query, got %s", m.lastQuery)
	}
}

func TestSnapshotDiff(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newSnapshotCmd()
	_ = cmd.Flags().Set("diff", "true")
	_ = cmd.Flags().Set("selector", "main")
	_ = cmd.Flags().Set("max-tokens", "2000")
	_ = cmd.Flags().Set("depth", "5")
	Snapshot(apiclient.Conn{Client: client, Base: m.base()}, cmd, "")
	if !strings.Contains(m.lastQuery, "diff=true") {
		t.Errorf("expected diff=true, got %s", m.lastQuery)
	}
	if !strings.Contains(m.lastQuery, "selector=main") {
		t.Errorf("expected selector=main, got %s", m.lastQuery)
	}
	if !strings.Contains(m.lastQuery, "maxTokens=2000") {
		t.Errorf("expected maxTokens=2000, got %s", m.lastQuery)
	}
	if !strings.Contains(m.lastQuery, "depth=5") {
		t.Errorf("expected depth=5, got %s", m.lastQuery)
	}
}

func TestSnapshotTabId(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newSnapshotCmd()
	_ = cmd.Flags().Set("tab", "ABC123")
	Snapshot(apiclient.Conn{Client: client, Base: m.base()}, cmd, "")
	if !strings.Contains(m.lastQuery, "tabId=ABC123") {
		t.Errorf("expected tabId=ABC123, got %s", m.lastQuery)
	}
}

func TestSnapshotSelectorOverride(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newSnapshotCmd()
	Snapshot(apiclient.Conn{Client: client, Base: m.base()}, cmd, "#main")
	if !strings.Contains(m.lastQuery, "selector=%23main") {
		t.Errorf("expected selector override, got %s", m.lastQuery)
	}
}
