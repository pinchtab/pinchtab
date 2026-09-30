package actions

import (
	"testing"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

func newHealthCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("json", false, "")
	return cmd
}

func TestHealth(t *testing.T) {
	m := newMockServer()
	m.response = `{"status":"ok","version":"dev"}`
	defer m.close()
	client := m.server.Client()

	cmd := newHealthCmd()
	Health(apiclient.Conn{Client: client, Base: m.base()}, cmd)
	if m.lastPath != "/health" {
		t.Errorf("expected /health, got %s", m.lastPath)
	}
}

func TestAuthHeader(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newHealthCmd()
	Health(apiclient.Conn{Client: client, Base: m.base(), Token: "my-secret-token"}, cmd)
	auth := m.lastHeaders.Get("Authorization")
	if auth != "Bearer my-secret-token" {
		t.Errorf("expected 'Bearer my-secret-token', got %q", auth)
	}
}

func TestNoAuthHeader(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newHealthCmd()
	Health(apiclient.Conn{Client: client, Base: m.base()}, cmd)
	auth := m.lastHeaders.Get("Authorization")
	if auth != "" {
		t.Errorf("expected no auth header, got %q", auth)
	}
}

func TestFormatProfileListSeparatesQuarantinedAndTotalsTheirSize(t *testing.T) {
	out := formatProfileList([]map[string]any{
		{"id": "p1", "name": "work", "diskUsage": float64(500 << 20)},
		{"id": "p2", "name": "quarantine-notes", "diskUsage": float64(1 << 20)},
		{"id": "p3", "name": "work.quarantine-1785343990", "diskUsage": float64(1 << 30), "quarantined": true},
		{"id": "p4", "name": "personal.quarantine-1785343991", "diskUsage": float64(1 << 29), "quarantined": true},
	})

	want := "p1\twork\n" +
		"p2\tquarantine-notes\n" +
		"\nQuarantined (2, 1.5 GB total):\n" +
		"p3\twork.quarantine-1785343990\t1.0 GB\n" +
		"p4\tpersonal.quarantine-1785343991\t512.0 MB\n"
	if out != want {
		t.Fatalf("formatProfileList() =\n%q\nwant\n%q", out, want)
	}
}

func TestFormatProfileListWithoutQuarantineHasNoSection(t *testing.T) {
	out := formatProfileList([]map[string]any{{"id": "p1", "name": "work", "diskUsage": float64(1 << 20)}})
	if out != "p1\twork\n" {
		t.Fatalf("formatProfileList() = %q, want a plain listing", out)
	}
}

func TestFormatProfileListEmpty(t *testing.T) {
	if out := formatProfileList(nil); out != "No profiles available\n" {
		t.Fatalf("formatProfileList(nil) = %q", out)
	}
}
