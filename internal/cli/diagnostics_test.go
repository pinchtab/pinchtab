package cli

import (
	"io"
	"os"
	"strings"
	"testing"
)

func TestCheckServerAndGuide(t *testing.T) {
	m := newMockServer()
	m.response = `{"status":"ok"}`
	defer m.close()
	client := m.server.Client()

	result := CheckServerAndGuide(client, m.base(), "")
	if !result {
		t.Error("expected CheckServerAndGuide to return true for working server")
	}

	// Test auth required (401)
	m2 := newMockServer()
	m2.statusCode = 401
	m2.response = `{"error":"unauthorized"}`
	defer m2.close()
	client2 := m2.server.Client()

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	result2 := CheckServerAndGuide(client2, m2.base(), "")

	_ = w.Close()
	os.Stderr = oldStderr
	output, _ := io.ReadAll(r)

	if result2 {
		t.Error("expected CheckServerAndGuide to return false for 401")
	}
	if !strings.Contains(string(output), "Authentication required") {
		t.Error("expected auth error message")
	}
}
