package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestHandleClipboardReadWriteWithoutTabs(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{AllowClipboard: true}, nil, nil, nil)

	writeReq := httptest.NewRequest(http.MethodPost, "/clipboard/write", bytes.NewReader([]byte(`{"text":"hello"}`)))
	writeW := httptest.NewRecorder()
	h.HandleClipboardWrite(writeW, writeReq)
	if writeW.Code != http.StatusOK {
		t.Fatalf("expected write status 200, got %d: %s", writeW.Code, writeW.Body.String())
	}
	if strings.Contains(writeW.Body.String(), "tabId") {
		t.Fatalf("expected write response to omit tabId, got %s", writeW.Body.String())
	}

	readReq := httptest.NewRequest(http.MethodGet, "/clipboard/read", nil)
	readW := httptest.NewRecorder()
	h.HandleClipboardRead(readW, readReq)
	if readW.Code != http.StatusOK {
		t.Fatalf("expected read status 200, got %d: %s", readW.Code, readW.Body.String())
	}
	if !strings.Contains(readW.Body.String(), `"text":"hello"`) {
		t.Fatalf("expected read response to include clipboard text, got %s", readW.Body.String())
	}
	if strings.Contains(readW.Body.String(), "tabId") {
		t.Fatalf("expected read response to omit tabId, got %s", readW.Body.String())
	}
}

func TestHandleClipboardReadIgnoresTabID(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{AllowClipboard: true}, nil, nil, nil)
	h.clipboard.Write("shared")

	req := httptest.NewRequest(http.MethodGet, "/clipboard/read?tabId=missing", nil)
	w := httptest.NewRecorder()
	h.HandleClipboardRead(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected read status 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"text":"shared"`) {
		t.Fatalf("expected shared clipboard text, got %s", w.Body.String())
	}
}

func TestHandleClipboardPasteAliasesRead(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{AllowClipboard: true}, nil, nil, nil)
	h.clipboard.Write("copied")

	mux := http.NewServeMux()
	h.RegisterRoutes(mux, nil)

	req := httptest.NewRequest(http.MethodGet, "/clipboard/paste", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected paste status 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"text":"copied"`) {
		t.Fatalf("expected paste response to include clipboard text, got %s", w.Body.String())
	}
}

func TestHandleClipboardWriteRejectsLargeText(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{AllowClipboard: true}, nil, nil, nil)
	tooLarge := strings.Repeat("x", maxClipboardTextBytes+1)

	req := httptest.NewRequest(http.MethodPost, "/clipboard/write", bytes.NewReader([]byte(`{"text":"`+tooLarge+`"}`)))
	w := httptest.NewRecorder()
	h.HandleClipboardWrite(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected oversized write status 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleClipboardDisabled(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/clipboard/read", nil)
	w := httptest.NewRecorder()
	h.HandleClipboardRead(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected disabled read status 403, got %d", w.Code)
	}
}

func TestHelpIncludesClipboardSecurityStatus(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/help", nil)
	w := httptest.NewRecorder()
	h.HandleHelp(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected help status 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "security.allowClipboard") {
		t.Fatalf("expected help response to include clipboard security setting, got %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "GET /clipboard/paste") {
		t.Fatalf("expected help response to include GET clipboard paste route, got %s", w.Body.String())
	}
}
