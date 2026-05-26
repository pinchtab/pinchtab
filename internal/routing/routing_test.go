package routing_test

import (
	"context"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/browserops"
	"github.com/pinchtab/pinchtab/internal/browsers"
	_ "github.com/pinchtab/pinchtab/internal/browsers/all"
	"github.com/pinchtab/pinchtab/internal/browsers/ghostchrome"
	"github.com/pinchtab/pinchtab/internal/routing"
)

// stubFetcher implements ghostchrome.StaticFetcher for testing.
type stubFetcher struct {
	text string
}

func (s *stubFetcher) Navigate(_ context.Context, url string) (ghostchrome.StaticNavResult, error) {
	return ghostchrome.StaticNavResult{URL: url, Title: "Test"}, nil
}

func (s *stubFetcher) Text(_ context.Context, _ string) (ghostchrome.StaticTextResult, error) {
	return ghostchrome.StaticTextResult{Text: s.text}, nil
}

func TestRoute_DirectChrome(t *testing.T) {
	res, err := routing.Route(routing.Request{
		Ctx:     context.Background(),
		URL:     "https://example.com",
		Intent:  browsers.RequestIntent{Shape: browsers.ShapeStaticRead},
		Browser: "chrome",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.UsedBrowser != "chrome" {
		t.Errorf("UsedBrowser = %q; want %q", res.UsedBrowser, "chrome")
	}
	if res.Route == nil {
		t.Fatal("Route is nil")
	}
	if res.Route.RequestedBrowser != "chrome" {
		t.Errorf("Route.RequestedBrowser = %q; want %q", res.Route.RequestedBrowser, "chrome")
	}
	if res.Route.UsedBrowser != "chrome" {
		t.Errorf("Route.UsedBrowser = %q; want %q", res.Route.UsedBrowser, "chrome")
	}
	if res.Escalated {
		t.Error("Escalated = true; want false")
	}
}

func TestRoute_DirectCloak(t *testing.T) {
	res, err := routing.Route(routing.Request{
		Ctx:     context.Background(),
		URL:     "https://example.com",
		Intent:  browsers.RequestIntent{Shape: browsers.ShapeStaticRead},
		Browser: "cloak",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.UsedBrowser != "cloak" {
		t.Errorf("UsedBrowser = %q; want %q", res.UsedBrowser, "cloak")
	}
}

func TestRoute_GhostChromeAccepted(t *testing.T) {
	res, err := routing.Route(routing.Request{
		Ctx:         context.Background(),
		URL:         "https://example.com",
		Intent:      browsers.RequestIntent{Shape: browsers.ShapeStaticRead},
		Browser:     "ghost-chrome",
		LiteFetcher: &stubFetcher{text: strings.Repeat("word ", 250)},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.UsedBrowser != "ghost" {
		t.Errorf("UsedBrowser = %q; want %q", res.UsedBrowser, "ghost")
	}
	if res.Escalated {
		t.Error("Escalated = true; want false")
	}
	if res.Quality < 60 {
		t.Errorf("Quality = %d; want >= 60", res.Quality)
	}
	if res.Route == nil {
		t.Fatal("Route is nil")
	}
	if len(res.Route.Attempts) != 1 {
		t.Fatalf("len(Route.Attempts) = %d; want 1", len(res.Route.Attempts))
	}
	if !res.Route.Attempts[0].Accepted {
		t.Error("Route.Attempts[0].Accepted = false; want true")
	}
}

func TestRoute_GhostChromeEscalated(t *testing.T) {
	res, err := routing.Route(routing.Request{
		Ctx:         context.Background(),
		URL:         "https://example.com",
		Intent:      browsers.RequestIntent{Shape: browsers.ShapeStaticRead},
		Browser:     "ghost-chrome",
		LiteFetcher: &stubFetcher{text: ""},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.UsedBrowser != "chrome" {
		t.Errorf("UsedBrowser = %q; want %q", res.UsedBrowser, "chrome")
	}
	if !res.Escalated {
		t.Error("Escalated = false; want true")
	}
	if res.Route == nil {
		t.Fatal("Route is nil")
	}
	if len(res.Route.Attempts) != 2 {
		t.Fatalf("len(Route.Attempts) = %d; want 2", len(res.Route.Attempts))
	}
}

func TestRoute_GhostChromeSkipsShape(t *testing.T) {
	res, err := routing.Route(routing.Request{
		Ctx:     context.Background(),
		URL:     "https://example.com",
		Intent:  browsers.RequestIntent{Shape: browsers.ShapeInteraction},
		Browser: "ghost-chrome",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.UsedBrowser != "chrome" {
		t.Errorf("UsedBrowser = %q; want %q", res.UsedBrowser, "chrome")
	}
	if !res.Escalated {
		t.Error("Escalated = false; want true")
	}
	if res.Route == nil {
		t.Fatal("Route is nil")
	}
	if res.Route.RequestedBrowser != "ghost-chrome" {
		t.Errorf("Route.RequestedBrowser = %q; want %q", res.Route.RequestedBrowser, "ghost-chrome")
	}
	if res.Route.UsedBrowser != "chrome" {
		t.Errorf("Route.UsedBrowser = %q; want %q", res.Route.UsedBrowser, "chrome")
	}
	if len(res.Route.Attempts) != 2 {
		t.Fatalf("len(Route.Attempts) = %d; want 2", len(res.Route.Attempts))
	}
	if res.Route.Attempts[0].Accepted {
		t.Error("Route.Attempts[0].Accepted = true; want false")
	}
	if !res.Route.Attempts[1].Accepted {
		t.Error("Route.Attempts[1].Accepted = false; want true")
	}
	if res.Route.Attempts[1].Browser != "chrome" {
		t.Errorf("Route.Attempts[1].Browser = %q; want %q", res.Route.Attempts[1].Browser, "chrome")
	}
}

func TestRoute_DecisionSkipFallback(t *testing.T) {
	// DecisionSkip should return a fallback result with Escalated=true, no error.
	// Ghost-chrome with interaction intent triggers DecisionSkip.
	res, err := routing.Route(routing.Request{
		Ctx:     context.Background(),
		URL:     "https://example.com",
		Intent:  browsers.RequestIntent{Shape: browsers.ShapeVisual},
		Browser: "ghost-chrome",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.UsedBrowser != "chrome" {
		t.Errorf("UsedBrowser = %q; want %q", res.UsedBrowser, "chrome")
	}
	if !res.Escalated {
		t.Error("Escalated = false; want true")
	}
	if res.Route == nil {
		t.Fatal("Route is nil")
	}
	if !res.Route.Escalated {
		t.Error("Route.Escalated = false; want true")
	}
	if res.Route.Attempts[0].Reason == "" {
		t.Error("Route.Attempts[0].Reason is empty; want skip reason")
	}
	if res.Route.Attempts[1].Reason != "fallback from skip" {
		t.Errorf("Route.Attempts[1].Reason = %q; want %q", res.Route.Attempts[1].Reason, "fallback from skip")
	}
}

func TestRoute_ChromeHandlesAllShapes(t *testing.T) {
	shapes := []string{
		browsers.ShapeStaticRead,
		browsers.ShapeRenderedRead,
		browsers.ShapeVisual,
		browsers.ShapeInteraction,
		browsers.ShapeSessionState,
	}
	for _, shape := range shapes {
		t.Run(shape, func(t *testing.T) {
			res, err := routing.Route(routing.Request{
				Ctx:     context.Background(),
				URL:     "https://example.com",
				Intent:  browsers.RequestIntent{Shape: shape},
				Browser: "chrome",
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.UsedBrowser != "chrome" {
				t.Errorf("UsedBrowser = %q; want %q", res.UsedBrowser, "chrome")
			}
			if res.Escalated {
				t.Error("Escalated = true; want false")
			}
		})
	}
}

func TestRoute_CloakHandlesAllShapes(t *testing.T) {
	shapes := []string{
		browsers.ShapeStaticRead,
		browsers.ShapeRenderedRead,
		browsers.ShapeVisual,
		browsers.ShapeInteraction,
		browsers.ShapeSessionState,
	}
	for _, shape := range shapes {
		t.Run(shape, func(t *testing.T) {
			res, err := routing.Route(routing.Request{
				Ctx:     context.Background(),
				URL:     "https://example.com",
				Intent:  browsers.RequestIntent{Shape: shape},
				Browser: "cloak",
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.UsedBrowser != "cloak" {
				t.Errorf("UsedBrowser = %q; want %q", res.UsedBrowser, "cloak")
			}
			if res.Escalated {
				t.Error("Escalated = true; want false")
			}
		})
	}
}

func TestRoute_UnknownBrowser(t *testing.T) {
	res, err := routing.Route(routing.Request{
		Ctx:     context.Background(),
		URL:     "https://example.com",
		Browser: "nonexistent",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.UsedBrowser != "nonexistent" {
		t.Errorf("UsedBrowser = %q; want %q", res.UsedBrowser, "nonexistent")
	}
	if res.Route == nil {
		t.Fatal("Route is nil")
	}
	// SingleBrowserRoute sets RequestedBrowser and UsedBrowser to the same value.
	if res.Route.RequestedBrowser != "nonexistent" {
		t.Errorf("Route.RequestedBrowser = %q; want %q", res.Route.RequestedBrowser, "nonexistent")
	}
	if res.Route.UsedBrowser != "nonexistent" {
		t.Errorf("Route.UsedBrowser = %q; want %q", res.Route.UsedBrowser, "nonexistent")
	}
}

func TestRoute_ConvertAttempts(t *testing.T) {
	// We test convertAttempts indirectly via a ghost-chrome route that
	// produces known attempts.
	res, err := routing.Route(routing.Request{
		Ctx:         context.Background(),
		URL:         "https://example.com",
		Intent:      browsers.RequestIntent{Shape: browsers.ShapeStaticRead},
		Browser:     "ghost-chrome",
		LiteFetcher: &stubFetcher{text: strings.Repeat("word ", 250)},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Route == nil {
		t.Fatal("Route is nil")
	}

	attempts := res.Route.Attempts
	if len(attempts) < 1 {
		t.Fatal("expected at least 1 attempt")
	}

	// Verify each attempt was properly converted to browserops.RouteAttempt.
	a := attempts[0]
	if a.Browser != "ghost" {
		t.Errorf("Attempts[0].Browser = %q; want %q", a.Browser, "ghost")
	}
	if !a.Accepted {
		t.Error("Attempts[0].Accepted = false; want true")
	}

	// Verify the type is browserops.RouteAttempt by accessing a typed field.
	_ = browserops.RouteAttempt(a)
}
