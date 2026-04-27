package actions

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func newInspectCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("tab", "", "")
	cmd.Flags().String("frame", "", "")
	cmd.Flags().String("selector", "", "")
	cmd.Flags().String("max-chars", "", "")
	cmd.Flags().String("prop", "", "")
	cmd.Flags().Bool("json", false, "")
	return cmd
}

func TestTitle(t *testing.T) {
	m := newMockServer()
	m.response = `{"title":"Example Page"}`
	defer m.close()

	cmd := newInspectCmd()
	Title(m.server.Client(), m.base(), "", cmd)
	if m.lastPath != "/title" {
		t.Fatalf("expected /title, got %s", m.lastPath)
	}
}

func TestURL(t *testing.T) {
	m := newMockServer()
	m.response = `{"url":"https://pinchtab.com/page"}`
	defer m.close()

	cmd := newInspectCmd()
	URL(m.server.Client(), m.base(), "", cmd)
	if m.lastPath != "/url" {
		t.Fatalf("expected /url, got %s", m.lastPath)
	}
}

func TestHTML_WithSelectorAndFrameAndMaxChars(t *testing.T) {
	m := newMockServer()
	m.response = `{"html":"<div>Hello</div>"}`
	defer m.close()

	cmd := newInspectCmd()
	_ = cmd.Flags().Set("frame", "FRAME123")
	_ = cmd.Flags().Set("max-chars", "120")
	HTML(m.server.Client(), m.base(), "", cmd, []string{"main article"})

	if m.lastPath != "/html" {
		t.Fatalf("expected /html, got %s", m.lastPath)
	}
	if !strings.Contains(m.lastQuery, "frameId=FRAME123") {
		t.Fatalf("expected frameId query, got %s", m.lastQuery)
	}
	if !strings.Contains(m.lastQuery, "maxChars=120") {
		t.Fatalf("expected maxChars query, got %s", m.lastQuery)
	}
	if !strings.Contains(m.lastQuery, "selector=main+article") {
		t.Fatalf("expected selector query, got %s", m.lastQuery)
	}
}

func TestHTML_UsesRefForRefSelector(t *testing.T) {
	m := newMockServer()
	m.response = `{"html":"<button>Save</button>"}`
	defer m.close()

	cmd := newInspectCmd()
	HTML(m.server.Client(), m.base(), "", cmd, []string{"e12"})

	if !strings.Contains(m.lastQuery, "ref=e12") {
		t.Fatalf("expected ref=e12, got %s", m.lastQuery)
	}
	if strings.Contains(m.lastQuery, "selector=") {
		t.Fatalf("did not expect selector query for ref input, got %s", m.lastQuery)
	}
}

func TestCSS_WithPropAndTab(t *testing.T) {
	m := newMockServer()
	m.response = `{"css":{"display":"block"}}`
	defer m.close()

	cmd := newInspectCmd()
	_ = cmd.Flags().Set("tab", "TAB1")
	_ = cmd.Flags().Set("prop", "display")
	CSS(m.server.Client(), m.base(), "", cmd, []string{"button.primary"})

	if m.lastPath != "/css" {
		t.Fatalf("expected /css, got %s", m.lastPath)
	}
	if !strings.Contains(m.lastQuery, "tabId=TAB1") {
		t.Fatalf("expected tabId query, got %s", m.lastQuery)
	}
	if !strings.Contains(m.lastQuery, "prop=display") {
		t.Fatalf("expected prop query, got %s", m.lastQuery)
	}
	if !strings.Contains(m.lastQuery, "selector=button.primary") && !strings.Contains(m.lastQuery, "selector=button%2Eprimary") {
		t.Fatalf("expected selector query, got %s", m.lastQuery)
	}
}

func TestCSS_WithoutSelectorHitsDocumentCSS(t *testing.T) {
	m := newMockServer()
	m.response = `{"css":{"display":"block"}}`
	defer m.close()

	cmd := newInspectCmd()
	CSS(m.server.Client(), m.base(), "", cmd, nil)

	if m.lastPath != "/css" {
		t.Fatalf("expected /css, got %s", m.lastPath)
	}
	if strings.Contains(m.lastQuery, "selector=") || strings.Contains(m.lastQuery, "ref=") {
		t.Fatalf("did not expect selector/ref query, got %s", m.lastQuery)
	}
}

func TestInspectJSONOutputUsesRawEndpoint(t *testing.T) {
	m := newMockServer()
	m.response = `{"title":"Example Page"}`
	defer m.close()

	cmd := newInspectCmd()
	_ = cmd.Flags().Set("json", "true")
	Title(m.server.Client(), m.base(), "", cmd)

	if m.lastPath != "/title" {
		t.Fatalf("expected /title, got %s", m.lastPath)
	}
}
