package cdptk

import (
	"context"
	"testing"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/pinchtab/pinchtab/internal/assets"
)

func TestCreateScreencastExecutionContext_UsesTopFrameNamedWorld(t *testing.T) {
	origGetFrameTree := GetFrameTree
	origCreateWorld := CreateIsolatedWorld
	defer func() {
		GetFrameTree = origGetFrameTree
		CreateIsolatedWorld = origCreateWorld
	}()

	GetFrameTree = func(context.Context) (*page.FrameTree, error) {
		return &page.FrameTree{Frame: &cdp.Frame{ID: cdp.FrameID("frame-top")}}, nil
	}

	var gotParams *page.CreateIsolatedWorldParams
	CreateIsolatedWorld = func(_ context.Context, params *page.CreateIsolatedWorldParams) (runtime.ExecutionContextID, error) {
		gotParams = params
		return runtime.ExecutionContextID(77), nil
	}

	execCtxID, err := CreateExecutionContext(context.Background())
	if err != nil {
		t.Fatalf("CreateExecutionContext returned error: %v", err)
	}
	if execCtxID != runtime.ExecutionContextID(77) {
		t.Fatalf("CreateExecutionContext returned %v, want 77", execCtxID)
	}
	if gotParams == nil {
		t.Fatal("CreateExecutionContext did not create isolated world params")
	}
	if gotParams.FrameID != cdp.FrameID("frame-top") {
		t.Fatalf("isolated world frame id = %q, want %q", gotParams.FrameID, cdp.FrameID("frame-top"))
	}
	if gotParams.WorldName != ScreencastRepaintWorldName {
		t.Fatalf("isolated world name = %q, want %q", gotParams.WorldName, ScreencastRepaintWorldName)
	}
}

func TestStartScreencastRepaintLoop_ReusesExecutionContextForStop(t *testing.T) {
	origGetFrameTree := GetFrameTree
	origCreateWorld := CreateIsolatedWorld
	origEvaluate := EvaluateInWorld
	defer func() {
		GetFrameTree = origGetFrameTree
		CreateIsolatedWorld = origCreateWorld
		EvaluateInWorld = origEvaluate
	}()

	GetFrameTree = func(context.Context) (*page.FrameTree, error) {
		return &page.FrameTree{Frame: &cdp.Frame{ID: cdp.FrameID("frame-top")}}, nil
	}
	CreateIsolatedWorld = func(_ context.Context, _ *page.CreateIsolatedWorldParams) (runtime.ExecutionContextID, error) {
		return runtime.ExecutionContextID(91), nil
	}

	type evalCall struct {
		ContextID  runtime.ExecutionContextID
		Expression string
	}
	var gotCalls []evalCall
	EvaluateInWorld = func(_ context.Context, params *runtime.EvaluateParams) (*runtime.RemoteObject, *runtime.ExceptionDetails, error) {
		gotCalls = append(gotCalls, evalCall{
			ContextID:  params.ContextID,
			Expression: params.Expression,
		})
		return &runtime.RemoteObject{}, nil, nil
	}

	stop := StartRepaintLoop(context.Background())
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
