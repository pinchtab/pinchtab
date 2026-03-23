package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/pinchtab/pinchtab/internal/assets"
	"github.com/pinchtab/pinchtab/internal/authn"
	"github.com/pinchtab/pinchtab/internal/config"
)

func TestHandleScreencast_AuthRejectsNoToken(t *testing.T) {
	cfg := &config.RuntimeConfig{Token: "secret-token-123", AllowScreencast: true}
	h := New(&mockBridge{}, cfg, nil, nil, nil)

	req := httptest.NewRequest("GET", "/screencast", nil)
	w := httptest.NewRecorder()
	handler := AuthMiddleware(cfg, http.HandlerFunc(h.HandleScreencast))
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized without token, got %d", w.Code)
	}
}

func TestHandleScreencast_AuthRejectsWrongToken(t *testing.T) {
	cfg := &config.RuntimeConfig{Token: "secret-token-123", AllowScreencast: true}
	h := New(&mockBridge{}, cfg, nil, nil, nil)

	req := httptest.NewRequest("GET", "/screencast", nil)
	req.AddCookie(&http.Cookie{Name: authn.CookieName, Value: "wrong-token"})
	req.Header.Set("Referer", "http://example.com/dashboard")
	w := httptest.NewRecorder()
	handler := AuthMiddleware(cfg, http.HandlerFunc(h.HandleScreencast))
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized with wrong token, got %d", w.Code)
	}
}

func TestHandleScreencast_AuthRejectsWrongHeader(t *testing.T) {
	cfg := &config.RuntimeConfig{Token: "secret-token-123", AllowScreencast: true}
	h := New(&mockBridge{}, cfg, nil, nil, nil)

	req := httptest.NewRequest("GET", "/screencast", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()
	handler := AuthMiddleware(cfg, http.HandlerFunc(h.HandleScreencast))
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized with wrong Bearer header, got %d", w.Code)
	}
}

func TestHandleScreencast_NoTokenConfigRejectsRequest(t *testing.T) {
	cfg := &config.RuntimeConfig{} // No token configured
	h := New(&mockBridge{failTab: true}, cfg, nil, nil, nil)

	req := httptest.NewRequest("GET", "/screencast", nil)
	w := httptest.NewRecorder()
	handler := AuthMiddleware(cfg, http.HandlerFunc(h.HandleScreencast))
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 when no token is configured, got %d", w.Code)
	}
}

func TestHandleScreencast_Disabled(t *testing.T) {
	cfg := &config.RuntimeConfig{}
	h := New(&mockBridge{}, cfg, nil, nil, nil)
	req := httptest.NewRequest("GET", "/screencast", nil)
	w := httptest.NewRecorder()
	h.HandleScreencast(w, req)
	if w.Code != 403 {
		t.Errorf("expected 403 when screencast disabled, got %d", w.Code)
	}
}

func TestCreateScreencastExecutionContext_UsesTopFrameNamedWorld(t *testing.T) {
	origGetFrameTree := getScreencastFrameTree
	origCreateWorld := createScreencastIsolatedWorld
	defer func() {
		getScreencastFrameTree = origGetFrameTree
		createScreencastIsolatedWorld = origCreateWorld
	}()

	getScreencastFrameTree = func(context.Context) (*page.FrameTree, error) {
		return &page.FrameTree{Frame: &cdp.Frame{ID: cdp.FrameID("frame-top")}}, nil
	}

	var gotParams *page.CreateIsolatedWorldParams
	createScreencastIsolatedWorld = func(_ context.Context, params *page.CreateIsolatedWorldParams) (runtime.ExecutionContextID, error) {
		gotParams = params
		return runtime.ExecutionContextID(77), nil
	}

	execCtxID, err := createScreencastExecutionContext(context.Background())
	if err != nil {
		t.Fatalf("createScreencastExecutionContext returned error: %v", err)
	}
	if execCtxID != runtime.ExecutionContextID(77) {
		t.Fatalf("createScreencastExecutionContext returned %v, want 77", execCtxID)
	}
	if gotParams == nil {
		t.Fatal("createScreencastExecutionContext did not create isolated world params")
	}
	if gotParams.FrameID != cdp.FrameID("frame-top") {
		t.Fatalf("isolated world frame id = %q, want %q", gotParams.FrameID, cdp.FrameID("frame-top"))
	}
	if gotParams.WorldName != screencastRepaintWorldName {
		t.Fatalf("isolated world name = %q, want %q", gotParams.WorldName, screencastRepaintWorldName)
	}
}

func TestStartScreencastRepaintLoop_ReusesExecutionContextForStop(t *testing.T) {
	origGetFrameTree := getScreencastFrameTree
	origCreateWorld := createScreencastIsolatedWorld
	origEvaluate := evaluateScreencastInWorld
	defer func() {
		getScreencastFrameTree = origGetFrameTree
		createScreencastIsolatedWorld = origCreateWorld
		evaluateScreencastInWorld = origEvaluate
	}()

	getScreencastFrameTree = func(context.Context) (*page.FrameTree, error) {
		return &page.FrameTree{Frame: &cdp.Frame{ID: cdp.FrameID("frame-top")}}, nil
	}
	createScreencastIsolatedWorld = func(_ context.Context, _ *page.CreateIsolatedWorldParams) (runtime.ExecutionContextID, error) {
		return runtime.ExecutionContextID(91), nil
	}

	type evalCall struct {
		ContextID  runtime.ExecutionContextID
		Expression string
	}
	var gotCalls []evalCall
	evaluateScreencastInWorld = func(_ context.Context, params *runtime.EvaluateParams) (*runtime.RemoteObject, *runtime.ExceptionDetails, error) {
		gotCalls = append(gotCalls, evalCall{
			ContextID:  params.ContextID,
			Expression: params.Expression,
		})
		return &runtime.RemoteObject{}, nil, nil
	}

	stop := startScreencastRepaintLoop(context.Background())
	if len(gotCalls) != 1 {
		t.Fatalf("start calls = %d, want 1", len(gotCalls))
	}
	if gotCalls[0].ContextID != runtime.ExecutionContextID(91) {
		t.Fatalf("start context id = %v, want 91", gotCalls[0].ContextID)
	}
	if gotCalls[0].Expression != assets.ScreencastRepaintStartJS {
		t.Fatalf("start expression = %q, want screencast repaint start asset", gotCalls[0].Expression)
	}

	stop()
	if len(gotCalls) != 2 {
		t.Fatalf("start+stop calls = %d, want 2", len(gotCalls))
	}
	if gotCalls[1].ContextID != runtime.ExecutionContextID(91) {
		t.Fatalf("stop context id = %v, want 91", gotCalls[1].ContextID)
	}
	if gotCalls[1].Expression != assets.ScreencastRepaintStopJS {
		t.Fatalf("stop expression = %q, want screencast repaint stop asset", gotCalls[1].Expression)
	}
}
