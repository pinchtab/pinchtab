//go:build integration

package integration

import (
	"testing"
)

// E1: Simple eval
func TestEval_Simple(t *testing.T) {
	navigate(t, "https://example.com")
	code, body := httpPost(t, "/evaluate", map[string]string{"expression": "1+1"})
	if code != 200 {
		t.Fatalf("expected 200, got %d", code)
	}
	result := jsonField(t, body, "result")
	if result != "2" {
		t.Errorf("expected result '2', got %q", result)
	}
}

// E2: DOM eval
func TestEval_DOM(t *testing.T) {
	navigate(t, "https://example.com")
	code, body := httpPost(t, "/evaluate", map[string]string{"expression": "document.title"})
	if code != 200 {
		t.Fatalf("expected 200, got %d", code)
	}
	result := jsonField(t, body, "result")
	if result != "Example Domain" {
		t.Errorf("expected 'Example Domain', got %q", result)
	}
}

// E3: Missing expression
func TestEval_MissingExpression(t *testing.T) {
	code, _ := httpPost(t, "/evaluate", map[string]string{})
	if code != 400 {
		t.Errorf("expected 400, got %d", code)
	}
}

// E4: Bad JSON
func TestEval_BadJSON(t *testing.T) {
	code, _ := httpPostRaw(t, "/evaluate", "{broken")
	if code != 400 {
		t.Errorf("expected 400, got %d", code)
	}
}
