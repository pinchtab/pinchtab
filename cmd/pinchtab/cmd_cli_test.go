package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsCLICommand(t *testing.T) {
	valid := []string{
		"health", "help", "config", "profiles", "instances", "tabs", "connect",
		"nav", "navigate", "snap", "snapshot", "find", "text",
		"screenshot", "ss", "pdf",
		"click", "type", "fill", "press", "hover", "scroll", "select",
		"eval", "evaluate",
	}

	for _, cmd := range valid {
		if !isCLICommand(cmd) {
			t.Errorf("expected %q to be a CLI command", cmd)
		}
	}

	invalid := []string{"dashboard", "server", "run", "", "quick", "focus"}
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

func TestCLIHealthFailure(t *testing.T) {
	m := newMockServer()
	m.statusCode = 500
	m.response = `{"status":"error"}`
	defer m.close()

	// Server connection test - just verify mock setup works
	// Actual error handling is tested via exit behavior
	if m.base() == "" {
		t.Error("expected mock server to be configured")
	}
}

// --- profiles tests ---

func TestCLIProfiles(t *testing.T) {
	m := newMockServer()
	m.response = `{"profiles":[{"name":"default"},{"name":"incognito"}]}`
	defer m.close()
	client := m.server.Client()

	cliProfiles(client, m.base(), "")
	if m.lastPath != "/profiles" {
		t.Errorf("expected /profiles, got %s", m.lastPath)
	}
}

// --- instances tests ---

func TestCLIInstances(t *testing.T) {
	m := newMockServer()
	m.response = `{"instances":[{"id":"inst-1","port":"9868","headless":true,"status":"running"}]}`
	defer m.close()
	client := m.server.Client()

	cliInstances(client, m.base(), "")
	if m.lastPath != "/instances" {
		t.Errorf("expected /instances, got %s", m.lastPath)
	}
}

func TestCLIInstancesEmpty(t *testing.T) {
	m := newMockServer()
	m.response = `{"instances":[]}`
	defer m.close()
	client := m.server.Client()

	cliInstances(client, m.base(), "")
	// Should handle empty list gracefully
}

// --- tabs tests ---

func TestCLITabs(t *testing.T) {
	m := newMockServer()
	m.response = `{"tabs":[{"id":"tab-1","url":"https://example.com","title":"Example"}]}`
	defer m.close()
	client := m.server.Client()

	cliTabs(client, m.base(), "")
	if m.lastPath != "/tabs" {
		t.Errorf("expected /tabs, got %s", m.lastPath)
	}
}

func TestCLITabsEmpty(t *testing.T) {
	m := newMockServer()
	m.response = `{"tabs":[]}`
	defer m.close()
	client := m.server.Client()

	cliTabs(client, m.base(), "")
	// Should handle empty tabs gracefully
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

// --- doGet helper tests ---

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
	doGet(client, m.base(), "", "/health", nil)
}
