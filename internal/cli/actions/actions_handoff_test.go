package actions

import (
	"encoding/json"
	"testing"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

func newHandoffCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("json", true, "")
	return cmd
}

func TestTabHandoff(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	TabHandoff(apiclient.Conn{Client: client, Base: m.base()}, "tab_123", "captcha_manual", 60000, newHandoffCmd())

	if m.lastMethod != "POST" {
		t.Fatalf("expected POST, got %s", m.lastMethod)
	}
	if m.lastPath != "/tabs/tab_123/handoff" {
		t.Fatalf("expected /tabs/tab_123/handoff, got %s", m.lastPath)
	}

	var body map[string]any
	if err := json.Unmarshal([]byte(m.lastBody), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if got, _ := body["reason"].(string); got != "captcha_manual" {
		t.Fatalf("expected reason=captcha_manual, got %v", body["reason"])
	}
	if got, _ := body["timeoutMs"].(float64); int(got) != 60000 {
		t.Fatalf("expected timeoutMs=60000, got %v", body["timeoutMs"])
	}
}

func TestTabResume(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	TabResume(apiclient.Conn{Client: client, Base: m.base()}, "tab_123", "completed", newHandoffCmd())

	if m.lastMethod != "POST" {
		t.Fatalf("expected POST, got %s", m.lastMethod)
	}
	if m.lastPath != "/tabs/tab_123/resume" {
		t.Fatalf("expected /tabs/tab_123/resume, got %s", m.lastPath)
	}

	var body map[string]any
	if err := json.Unmarshal([]byte(m.lastBody), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if got, _ := body["status"].(string); got != "completed" {
		t.Fatalf("expected status=completed, got %v", body["status"])
	}
}

func TestTabHandoffStatus(t *testing.T) {
	m := newMockServer()
	m.response = `{"tabId":"tab_123","status":"active"}`
	defer m.close()
	client := m.server.Client()

	TabHandoffStatus(apiclient.Conn{Client: client, Base: m.base()}, "tab_123", newHandoffCmd())

	if m.lastMethod != "GET" {
		t.Fatalf("expected GET, got %s", m.lastMethod)
	}
	if m.lastPath != "/tabs/tab_123/handoff" {
		t.Fatalf("expected /tabs/tab_123/handoff, got %s", m.lastPath)
	}
}

func TestTabHandoffEscapesTheTabID(t *testing.T) {
	m := newMockServer()
	defer m.close()

	TabHandoff(apiclient.Conn{Client: m.server.Client(), Base: m.base()}, "a/b", "", 0, newHandoffCmd())

	if m.lastRawPath != "/tabs/a%2Fb/handoff" {
		t.Fatalf("expected /tabs/a%%2Fb/handoff, got %s", m.lastRawPath)
	}
}
