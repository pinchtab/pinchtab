package handlers

import (
	"testing"

	"github.com/pinchtab/pinchtab/internal/browsers"
	_ "github.com/pinchtab/pinchtab/internal/browsers/chrome"
	_ "github.com/pinchtab/pinchtab/internal/browsers/cloak"
	_ "github.com/pinchtab/pinchtab/internal/browsers/ghostchrome"
)

func TestCheckBrowserCanHandle_ChromeAlwaysHandles(t *testing.T) {
	intents := []browsers.RequestIntent{
		{Shape: browsers.ShapeRenderedRead},
		{Shape: browsers.ShapeStaticSnapshot},
		{Shape: browsers.ShapeInteraction, StateChanging: true},
		{Shape: browsers.ShapeVisual},
	}

	for _, intent := range intents {
		decision, err := checkBrowserCanHandle("chrome", intent)
		if err != nil {
			t.Errorf("checkBrowserCanHandle(\"chrome\", %+v) = %v, want nil", intent, err)
		}
		if decision.Decision != browsers.DecisionHandle {
			t.Errorf("checkBrowserCanHandle(\"chrome\", %+v) decision = %q, want %q", intent, decision.Decision, browsers.DecisionHandle)
		}
	}
}

func TestCheckBrowserCanHandle_GhostChromeSkipsRenderedRead(t *testing.T) {
	decision, err := checkBrowserCanHandle("ghost-chrome", browsers.RequestIntent{
		Shape: browsers.ShapeRenderedRead,
	})
	if err != nil {
		t.Fatalf("checkBrowserCanHandle(\"ghost-chrome\", rendered-read) = %v, want nil (skip without error)", err)
	}
	if decision.Decision != browsers.DecisionSkip {
		t.Errorf("decision = %q, want %q", decision.Decision, browsers.DecisionSkip)
	}
}

func TestCheckBrowserCanHandle_GhostChromeSkipsStateChanging(t *testing.T) {
	decision, err := checkBrowserCanHandle("ghost-chrome", browsers.RequestIntent{
		Shape:         browsers.ShapeInteraction,
		StateChanging: true,
	})
	if err != nil {
		t.Fatalf("checkBrowserCanHandle(\"ghost-chrome\", interaction+stateChanging) = %v, want nil (skip without error)", err)
	}
	if decision.Decision != browsers.DecisionSkip {
		t.Errorf("decision = %q, want %q", decision.Decision, browsers.DecisionSkip)
	}
}

func TestCheckBrowserCanHandle_ExplicitCloakHandlesAll(t *testing.T) {
	shapes := []string{
		browsers.ShapeStaticRead, browsers.ShapeRenderedRead,
		browsers.ShapeInteraction, browsers.ShapeVisual,
		browsers.ShapeStaticSnapshot, browsers.ShapeSessionState,
	}
	for _, shape := range shapes {
		t.Run(shape, func(t *testing.T) {
			decision, err := checkBrowserCanHandle("cloak", browsers.RequestIntent{Shape: shape})
			if err != nil {
				t.Errorf("cloak shape=%q: unexpected error %v", shape, err)
			}
			if decision.Decision != browsers.DecisionHandle {
				t.Errorf("cloak shape=%q: decision=%q, want handle", shape, decision.Decision)
			}
		})
	}
}

func TestCheckBrowserCanHandle_UnknownBrowserReturnsNil(t *testing.T) {
	decision, err := checkBrowserCanHandle("totally-unknown-browser", browsers.RequestIntent{
		Shape: browsers.ShapeRenderedRead,
	})
	if err != nil {
		t.Errorf("checkBrowserCanHandle(\"totally-unknown-browser\", ...) = %v, want nil", err)
	}
	if decision.Decision != browsers.DecisionHandle {
		t.Errorf("decision = %q, want %q", decision.Decision, browsers.DecisionHandle)
	}
}
