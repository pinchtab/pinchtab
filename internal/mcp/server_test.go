package mcp

import (
	"testing"
)

func TestNewServer(t *testing.T) {
	s := NewServer("http://localhost:9867", "tok")
	if s == nil {
		t.Fatal("NewServer returned nil")
	}
}

func TestNewServerRegistersAllTools(t *testing.T) {
	_ = NewServer("http://localhost:9867", "")
	tools := allTools()

	// The server should have registered all tools.
	// We verify by checking that NewServer doesn't panic — the panic
	// in NewServer fires if any tool lacks a handler.
	if len(tools) != 36 {
		t.Errorf("expected 36 tools, got %d", len(tools))
	}
}

func TestAllToolsHaveUniqueNames(t *testing.T) {
	tools := allTools()
	seen := make(map[string]bool, len(tools))
	for _, tool := range tools {
		if seen[tool.Name] {
			t.Errorf("duplicate tool name: %s", tool.Name)
		}
		seen[tool.Name] = true
	}
}

func TestAllToolsHaveHandlers(t *testing.T) {
	tools := allTools()
	handlers := handlerMap(NewClient("http://localhost:9867", ""))
	for _, tool := range tools {
		if _, ok := handlers[tool.Name]; !ok {
			t.Errorf("tool %q has no handler", tool.Name)
		}
	}
}

func TestVersionDefault(t *testing.T) {
	if Version != "dev" {
		t.Errorf("default Version = %q, want 'dev'", Version)
	}
}
