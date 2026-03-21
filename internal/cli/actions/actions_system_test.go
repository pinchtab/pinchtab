package actions

import (
	"testing"
)

func TestHealth(t *testing.T) {
	m := newMockServer()
	m.response = `{"status":"ok","version":"dev"}`
	defer m.close()
	client := m.server.Client()

	Health(client, m.base(), "")
	if m.lastPath != "/health" {
		t.Errorf("expected /health, got %s", m.lastPath)
	}
}

func TestAuthHeader(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	Health(client, m.base(), "my-secret-token")
	auth := m.lastHeaders.Get("Authorization")
	if auth != "Bearer my-secret-token" {
		t.Errorf("expected 'Bearer my-secret-token', got %q", auth)
	}
}

func TestNoAuthHeader(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	Health(client, m.base(), "")
	auth := m.lastHeaders.Get("Authorization")
	if auth != "" {
		t.Errorf("expected no auth header, got %q", auth)
	}
}

func TestGetInstances_ArrayResponse(t *testing.T) {
	m := newMockServer()
	m.response = `[{"id":"inst_123","port":"9868","status":"running","headless":true}]`
	defer m.close()
	client := m.server.Client()

	instances := getInstances(client, m.base(), "")
	if len(instances) != 1 {
		t.Fatalf("len(instances) = %d, want 1", len(instances))
	}
	if got, _ := instances[0]["id"].(string); got != "inst_123" {
		t.Fatalf("id = %q, want %q", got, "inst_123")
	}
}

func TestGetInstances_EnvelopeResponseRejected(t *testing.T) {
	m := newMockServer()
	m.response = `{"instances":[{"id":"inst_456","port":"9869","status":"running","headless":false}]}`
	defer m.close()
	client := m.server.Client()

	instances := getInstances(client, m.base(), "")
	if instances != nil {
		t.Fatalf("instances = %#v, want nil for legacy envelope response", instances)
	}
}
