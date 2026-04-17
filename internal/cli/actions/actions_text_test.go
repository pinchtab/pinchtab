package actions

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func newTextCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("raw", false, "")
	cmd.Flags().Bool("full", false, "")
	cmd.Flags().String("tab", "", "")
	cmd.Flags().String("frame", "", "")
	return cmd
}

func TestText(t *testing.T) {
	m := newMockServer()
	m.response = `{"url":"https://pinchtab.com","title":"Example","text":"Hello"}`
	defer m.close()
	client := m.server.Client()

	cmd := newTextCmd()
	Text(client, m.base(), "", cmd)
	if m.lastPath != "/text" {
		t.Errorf("expected /text, got %s", m.lastPath)
	}
}

func TestTextRaw(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newTextCmd()
	_ = cmd.Flags().Set("raw", "true")
	Text(client, m.base(), "", cmd)
	if !strings.Contains(m.lastQuery, "mode=raw") {
		t.Errorf("expected mode=raw, got %s", m.lastQuery)
	}
	if !strings.Contains(m.lastQuery, "format=text") {
		t.Errorf("expected format=text, got %s", m.lastQuery)
	}
}

func TestTextFull(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newTextCmd()
	_ = cmd.Flags().Set("full", "true")
	Text(client, m.base(), "", cmd)
	if !strings.Contains(m.lastQuery, "mode=raw") {
		t.Errorf("expected --full to set mode=raw, got %s", m.lastQuery)
	}
	if !strings.Contains(m.lastQuery, "format=text") {
		t.Errorf("expected --full to set format=text, got %s", m.lastQuery)
	}
}

func TestTextFrame(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newTextCmd()
	_ = cmd.Flags().Set("frame", "FRAME123")
	Text(client, m.base(), "", cmd)
	if !strings.Contains(m.lastQuery, "frameId=FRAME123") {
		t.Errorf("expected frameId=FRAME123, got %s", m.lastQuery)
	}
}

func TestTextTab(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newTextCmd()
	_ = cmd.Flags().Set("tab", "TAB1")
	Text(client, m.base(), "", cmd)
	if !strings.Contains(m.lastQuery, "tabId=TAB1") {
		t.Errorf("expected tabId=TAB1, got %s", m.lastQuery)
	}
}
