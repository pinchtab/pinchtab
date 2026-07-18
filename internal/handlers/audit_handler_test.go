package handlers

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestHandleAuditSeaportalStaticPagesDoNotRequireBrowser(t *testing.T) {
	bridge := &mockBridge{ensureBrowserErr: errors.New("browser unavailable")}
	h := New(bridge, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/audit", bytes.NewBufferString(`{"seaportalResults":[{"url":"https://example.com/a","title":"Static page","statusCode":200,"profile":{"browserRecommended":false}}]}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleAudit(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	if bridge.ensureBrowserCall != 0 {
		t.Fatalf("EnsureBrowser called %d times, want 0", bridge.ensureBrowserCall)
	}
}
