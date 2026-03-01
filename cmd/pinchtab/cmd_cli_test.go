package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestIsCLICommand(t *testing.T) {
	valid := []string{"nav", "navigate", "snap", "snapshot", "click", "type",
		"press", "fill", "hover", "scroll", "select", "focus",
		"text", "tabs", "tab", "screenshot", "ss", "eval", "evaluate",
		"pdf", "health", "quick"}

	for _, cmd := range valid {
		if !isCLICommand(cmd) {
			t.Errorf("expected %q to be a CLI command", cmd)
		}
	}

	invalid := []string{"dashboard", "connect", "config", "server", "run", ""}
	for _, cmd := range invalid {
		if isCLICommand(cmd) {
			t.Errorf("expected %q to NOT be a CLI command", cmd)
		}
	}
}

func TestPrintHelp(t *testing.T) {
	printHelp()
}

// mockServer records the last request and returns a configurable response.
type mockServer struct {
	server      *httptest.Server
	lastMethod  string
	lastPath    string
	lastQuery   string
	lastBody    string
	lastHeaders http.Header
	response    string
	statusCode  int
}

func newMockServer() *mockServer {
	m := &mockServer{statusCode: 200, response: `{"status":"ok"}`}
	m.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.lastMethod = r.Method
		m.lastPath = r.URL.Path
		m.lastQuery = r.URL.RawQuery
		m.lastHeaders = r.Header
		if r.Body != nil {
			body, _ := io.ReadAll(r.Body)
			m.lastBody = string(body)
		}
		w.WriteHeader(m.statusCode)
		_, _ = w.Write([]byte(m.response))
	}))
	return m
}

func (m *mockServer) close()       { m.server.Close() }
func (m *mockServer) base() string { return m.server.URL }

// --- navigate tests ---

func TestCLINavigate(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cliNavigate(client, m.base(), "", []string{"https://example.com"})
	if m.lastMethod != "POST" {
		t.Errorf("expected POST, got %s", m.lastMethod)
	}
	if m.lastPath != "/navigate" {
		t.Errorf("expected /navigate, got %s", m.lastPath)
	}
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["url"] != "https://example.com" {
		t.Errorf("expected url=https://example.com, got %v", body["url"])
	}
}

func TestCLINavigateWithFlags(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cliNavigate(client, m.base(), "", []string{"https://example.com", "--new-tab", "--block-images"})
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["newTab"] != true {
		t.Error("expected newTab=true")
	}
	if body["blockImages"] != true {
		t.Error("expected blockImages=true")
	}
}

func TestCLINavigateWithBlockAds(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cliNavigate(client, m.base(), "", []string{"https://example.com", "--block-ads"})
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["blockAds"] != true {
		t.Error("expected blockAds=true")
	}
}

func TestCLIInstanceNavigateUsesTabRoute(t *testing.T) {
	m := newMockServer()
	m.response = `{"tabId":"tab-abc"}`
	defer m.close()
	client := m.server.Client()

	cliInstanceNavigate(client, m.base(), "", []string{"inst-123", "https://example.com"})

	if m.lastMethod != "POST" {
		t.Errorf("expected POST, got %s", m.lastMethod)
	}
	if m.lastPath != "/tabs/tab-abc/navigate" {
		t.Errorf("expected tab-scoped navigate path, got %s", m.lastPath)
	}

	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["url"] != "https://example.com" {
		t.Errorf("expected navigate URL in body, got %v", body["url"])
	}
}

// --- snapshot tests ---

func TestCLISnapshot(t *testing.T) {
	m := newMockServer()
	m.response = `[{"ref":"e0","role":"button","name":"Submit"}]`
	defer m.close()
	client := m.server.Client()

	cliSnapshot(client, m.base(), "", []string{"-i", "-c"})
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

func TestCLISnapshotDiff(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cliSnapshot(client, m.base(), "", []string{"--diff", "--selector", "main", "--max-tokens", "2000", "--depth", "5"})
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

func TestCLISnapshotTabId(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cliSnapshot(client, m.base(), "", []string{"--tab", "ABC123"})
	if !strings.Contains(m.lastQuery, "tabId=ABC123") {
		t.Errorf("expected tabId=ABC123, got %s", m.lastQuery)
	}
}

// --- action tests ---

func TestCLIClick(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cliAction(client, m.base(), "", "click", []string{"e5"})
	if m.lastPath != "/action" {
		t.Errorf("expected /action, got %s", m.lastPath)
	}
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["kind"] != "click" {
		t.Errorf("expected kind=click, got %v", body["kind"])
	}
	if body["ref"] != "e5" {
		t.Errorf("expected ref=e5, got %v", body["ref"])
	}
}

func TestCLIClickWaitNav(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cliAction(client, m.base(), "", "click", []string{"e5", "--wait-nav"})
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["waitNav"] != true {
		t.Error("expected waitNav=true")
	}
}

func TestCLIType(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cliAction(client, m.base(), "", "type", []string{"e12", "hello", "world"})
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["kind"] != "type" {
		t.Errorf("expected kind=type, got %v", body["kind"])
	}
	if body["ref"] != "e12" {
		t.Errorf("expected ref=e12, got %v", body["ref"])
	}
	if body["text"] != "hello world" {
		t.Errorf("expected text='hello world', got %v", body["text"])
	}
}

func TestCLIPress(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cliAction(client, m.base(), "", "press", []string{"Enter"})
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["key"] != "Enter" {
		t.Errorf("expected key=Enter, got %v", body["key"])
	}
}

func TestCLIFill(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	// Fill with ref
	cliAction(client, m.base(), "", "fill", []string{"e3", "test value"})
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["ref"] != "e3" {
		t.Errorf("expected ref=e3, got %v", body["ref"])
	}
	if body["text"] != "test value" {
		t.Errorf("expected text='test value', got %v", body["text"])
	}

	// Fill with selector
	cliAction(client, m.base(), "", "fill", []string{"#email", "user@test.com"})
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["selector"] != "#email" {
		t.Errorf("expected selector=#email, got %v", body["selector"])
	}
}

func TestCLIScroll(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	// Scroll by ref
	cliAction(client, m.base(), "", "scroll", []string{"e20"})
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["ref"] != "e20" {
		t.Errorf("expected ref=e20, got %v", body["ref"])
	}

	// Scroll by pixels
	cliAction(client, m.base(), "", "scroll", []string{"800"})
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["scrollY"] != "800" {
		t.Errorf("expected scrollY=800, got %v", body["scrollY"])
	}
}

func TestCLISelect(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cliAction(client, m.base(), "", "select", []string{"e10", "option2"})
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["ref"] != "e10" {
		t.Errorf("expected ref=e10, got %v", body["ref"])
	}
	if body["value"] != "option2" {
		t.Errorf("expected value=option2, got %v", body["value"])
	}
}

// --- text tests ---

func TestCLIText(t *testing.T) {
	m := newMockServer()
	m.response = `{"url":"https://example.com","title":"Example","text":"Hello"}`
	defer m.close()
	client := m.server.Client()

	cliText(client, m.base(), "", nil)
	if m.lastPath != "/text" {
		t.Errorf("expected /text, got %s", m.lastPath)
	}
}

func TestCLITextRaw(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cliText(client, m.base(), "", []string{"--raw"})
	if !strings.Contains(m.lastQuery, "mode=raw") {
		t.Errorf("expected mode=raw, got %s", m.lastQuery)
	}
}

func TestCLITextTab(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cliText(client, m.base(), "", []string{"--tab", "TAB1"})
	if !strings.Contains(m.lastQuery, "tabId=TAB1") {
		t.Errorf("expected tabId=TAB1, got %s", m.lastQuery)
	}
}

// --- tabs tests ---

func TestCLITabsList(t *testing.T) {
	m := newMockServer()
	m.response = `[{"id":"TAB1","url":"https://example.com"}]`
	defer m.close()
	client := m.server.Client()

	cliTabs(client, m.base(), "", nil)
	if m.lastPath != "/tabs" {
		t.Errorf("expected /tabs, got %s", m.lastPath)
	}
}

func TestCLITabsNew(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cliTabs(client, m.base(), "", []string{"new", "https://example.com"})
	if m.lastPath != "/tab" {
		t.Errorf("expected /tab, got %s", m.lastPath)
	}
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["action"] != "new" {
		t.Errorf("expected action=new, got %v", body["action"])
	}
	if body["url"] != "https://example.com" {
		t.Errorf("expected url, got %v", body["url"])
	}
}

// --- evaluate tests ---

func TestCLIEvaluate(t *testing.T) {
	m := newMockServer()
	m.response = `{"result":"Example Domain"}`
	defer m.close()
	client := m.server.Client()

	cliEvaluate(client, m.base(), "", []string{"document.title"})
	if m.lastPath != "/evaluate" {
		t.Errorf("expected /evaluate, got %s", m.lastPath)
	}
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["expression"] != "document.title" {
		t.Errorf("expected expression=document.title, got %v", body["expression"])
	}
}

func TestCLIEvaluateMultiWord(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cliEvaluate(client, m.base(), "", []string{"1", "+", "2"})
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["expression"] != "1 + 2" {
		t.Errorf("expected expression='1 + 2', got %v", body["expression"])
	}
}

// --- screenshot tests ---

func TestCLIScreenshot(t *testing.T) {
	m := newMockServer()
	m.response = "FAKEJPEGDATA"
	defer m.close()
	client := m.server.Client()

	outFile := t.TempDir() + "/test.jpg"
	cliScreenshot(client, m.base(), "", []string{"-o", outFile, "-q", "50"})
	if m.lastPath != "/screenshot" {
		t.Errorf("expected /screenshot, got %s", m.lastPath)
	}
	if !strings.Contains(m.lastQuery, "quality=50") {
		t.Errorf("expected quality=50, got %s", m.lastQuery)
	}
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("file not written: %v", err)
	}
	if string(data) != "FAKEJPEGDATA" {
		t.Errorf("unexpected content: %s", string(data))
	}
}

// --- pdf tests ---

func TestCLIPDF(t *testing.T) {
	m := newMockServer()
	m.response = "FAKEPDFDATA"
	defer m.close()
	client := m.server.Client()

	outFile := t.TempDir() + "/test.pdf"
	cliPDF(client, m.base(), "", []string{"-o", outFile, "--tab", "tab-abc", "--landscape", "--scale", "0.8"})
	if m.lastPath != "/tabs/tab-abc/pdf" {
		t.Errorf("expected /tabs/tab-abc/pdf, got %s", m.lastPath)
	}
	if !strings.Contains(m.lastQuery, "landscape=true") {
		t.Errorf("expected landscape=true, got %s", m.lastQuery)
	}
	if !strings.Contains(m.lastQuery, "scale=0.8") {
		t.Errorf("expected scale=0.8, got %s", m.lastQuery)
	}
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("file not written: %v", err)
	}
	if string(data) != "FAKEPDFDATA" {
		t.Errorf("unexpected content: %s", string(data))
	}
}

func TestCLIPDFAllOptions(t *testing.T) {
	m := newMockServer()
	m.response = "FAKEPDFDATA"
	defer m.close()
	client := m.server.Client()

	outFile := t.TempDir() + "/test.pdf"
	args := []string{
		"-o", outFile,
		"--landscape",
		"--scale", "1.5",
		"--paper-width", "11",
		"--paper-height", "8.5",
		"--margin-top", "1",
		"--margin-bottom", "1",
		"--margin-left", "0.5",
		"--margin-right", "0.5",
		"--page-ranges", "1-3,5",
		"--prefer-css-page-size",
		"--display-header-footer",
		"--header-template", "<span class='title'></span>",
		"--footer-template", "<span class='pageNumber'></span>",
		"--generate-tagged-pdf",
		"--generate-document-outline",
		"--tab", "tab-123",
	}

	cliPDF(client, m.base(), "", args)
	if m.lastPath != "/tabs/tab-123/pdf" {
		t.Errorf("expected /tabs/tab-123/pdf, got %s", m.lastPath)
	}

	// Check all parameters were set correctly
	expectedParams := []string{
		"landscape=true",
		"scale=1.5",
		"paperWidth=11",
		"paperHeight=8.5",
		"marginTop=1",
		"marginBottom=1",
		"marginLeft=0.5",
		"marginRight=0.5",
		"pageRanges=1-3%2C5", // URL encoded
		"preferCSSPageSize=true",
		"displayHeaderFooter=true",
		"generateTaggedPDF=true",
		"generateDocumentOutline=true",
		"raw=true",
	}

	for _, expected := range expectedParams {
		if !strings.Contains(m.lastQuery, expected) {
			t.Errorf("expected %s in query, got %s", expected, m.lastQuery)
		}
	}

	// Check encoded HTML templates are present
	if !strings.Contains(m.lastQuery, "headerTemplate=") || !strings.Contains(m.lastQuery, "span") {
		t.Error("expected headerTemplate with HTML content")
	}
	if !strings.Contains(m.lastQuery, "footerTemplate=") || !strings.Contains(m.lastQuery, "pageNumber") {
		t.Error("expected footerTemplate with HTML content")
	}
}

// --- health tests ---

func TestCLIHealth(t *testing.T) {
	m := newMockServer()
	m.response = `{"status":"ok","version":"dev"}`
	defer m.close()
	client := m.server.Client()

	cliHealth(client, m.base(), "")
	if m.lastPath != "/health" {
		t.Errorf("expected /health, got %s", m.lastPath)
	}
}

// --- auth header tests ---

func TestCLIAuthHeader(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cliHealth(client, m.base(), "my-secret-token")
	auth := m.lastHeaders.Get("Authorization")
	if auth != "Bearer my-secret-token" {
		t.Errorf("expected 'Bearer my-secret-token', got %q", auth)
	}
}

func TestCLINoAuthHeader(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cliHealth(client, m.base(), "")
	auth := m.lastHeaders.Get("Authorization")
	if auth != "" {
		t.Errorf("expected no auth header, got %q", auth)
	}
}

// --- doGet/doPost helpers ---

func TestDoGetPrettyPrintsJSON(t *testing.T) {
	m := newMockServer()
	m.response = `{"a":1,"b":2}`
	defer m.close()
	client := m.server.Client()

	// Just verify it doesn't panic with valid JSON
	doGet(client, m.base(), "", "/health", nil)
}

func TestDoGetNonJSON(t *testing.T) {
	m := newMockServer()
	m.response = "plain text response"
	defer m.close()
	client := m.server.Client()

	// Should handle non-JSON gracefully
	doGet(client, m.base(), "", "/text", nil)
}

func TestDoPostContentType(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	_ = doPost(client, m.base(), "", "/action", map[string]any{"kind": "click"})
	ct := m.lastHeaders.Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected application/json, got %q", ct)
	}
}

// --- server guidance tests ---

func TestCheckServerAndGuide(t *testing.T) {
	// Test successful connection
	m := newMockServer()
	m.response = `{"status":"ok"}`
	defer m.close()
	client := m.server.Client()

	result := checkServerAndGuide(client, m.base(), "")
	if !result {
		t.Error("expected checkServerAndGuide to return true for working server")
	}

	// Test auth required (401)
	m2 := newMockServer()
	m2.statusCode = 401
	m2.response = `{"error":"unauthorized"}`
	defer m2.close()
	client2 := m2.server.Client()

	// Capture stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	result2 := checkServerAndGuide(client2, m2.base(), "")

	_ = w.Close()
	os.Stderr = oldStderr
	output, _ := io.ReadAll(r)

	if result2 {
		t.Error("expected checkServerAndGuide to return false for 401")
	}
	if !strings.Contains(string(output), "Authentication required") {
		t.Error("expected auth error message")
	}
}
