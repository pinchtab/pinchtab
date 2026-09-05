package apiclient

import (
	"testing"
)

func TestDoGetPrettyPrintsJSON(t *testing.T) {
	m := newMockServer()
	m.response = `{"a":1,"b":2}`
	defer m.close()
	client := m.server.Client()

	// Just verify it doesn't panic with valid JSON
	DoGet(client, m.base(), "", "/health", nil)
}

func TestDoGetNonJSON(t *testing.T) {
	m := newMockServer()
	m.response = "plain text response"
	defer m.close()
	client := m.server.Client()

	// Should handle non-JSON gracefully
	DoGet(client, m.base(), "", "/text", nil)
}

func TestDoPostContentType(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	_ = DoPost(client, m.base(), "", "/action", map[string]any{"kind": "click"})
	ct := m.lastHeaders.Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected application/json, got %q", ct)
	}
}

func TestResolveInstanceBase(t *testing.T) {
	orch := newMockServer()
	orch.response = `{"id":"abc123","port":"9901","status":"running"}`
	defer orch.close()

	got := ResolveInstanceBase(orch.base(), "", "abc123", "127.0.0.1")

	if orch.lastPath != "/instances/abc123" {
		t.Errorf("expected GET /instances/abc123, got %s", orch.lastPath)
	}
	if got != "http://127.0.0.1:9901" {
		t.Errorf("ResolveInstanceBase = %q, want %q", got, "http://127.0.0.1:9901")
	}
}

func TestResolveInstanceBase_ForwardsToken(t *testing.T) {
	orch := newMockServer()
	orch.response = `{"id":"xyz","port":"9902","status":"running"}`
	defer orch.close()

	ResolveInstanceBase(orch.base(), "my-token", "xyz", "localhost")

	authHeader := orch.lastHeaders.Get("Authorization")
	if authHeader != "Bearer my-token" {
		t.Errorf("Authorization header = %q, want %q", authHeader, "Bearer my-token")
	}
}
