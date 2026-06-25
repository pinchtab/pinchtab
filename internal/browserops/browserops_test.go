package browserops

import (
	"encoding/json"
	"testing"
)

func TestSingleBrowserRoute(t *testing.T) {
	route := SingleBrowserRoute("chrome")
	if route.RequestedBrowser != "chrome" {
		t.Errorf("RequestedBrowser = %q, want %q", route.RequestedBrowser, "chrome")
	}
	if route.UsedBrowser != "chrome" {
		t.Errorf("UsedBrowser = %q, want %q", route.UsedBrowser, "chrome")
	}
	if route.Escalated {
		t.Error("Escalated should be false")
	}
	if len(route.Attempts) != 1 {
		t.Fatalf("len(Attempts) = %d, want 1", len(route.Attempts))
	}
	if route.Attempts[0].Browser != "chrome" {
		t.Errorf("Attempts[0].Browser = %q, want %q", route.Attempts[0].Browser, "chrome")
	}
	if !route.Attempts[0].Accepted {
		t.Error("Attempts[0].Accepted should be true")
	}
}

func TestRouteMetadataJSONKeys(t *testing.T) {
	result := NavigateResult{
		TabID: "tab-1",
		URL:   "https://example.com",
		Title: "Example",
		Route: SingleBrowserRoute("chrome"),
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	routeRaw, ok := m["route"]
	if !ok {
		t.Fatal("expected \"route\" key in JSON output")
	}
	route, ok := routeRaw.(map[string]any)
	if !ok {
		t.Fatalf("route is %T, want map[string]any", routeRaw)
	}

	// Must use provider naming.
	if _, ok := route["requestedProvider"]; !ok {
		t.Fatal("route missing \"requestedProvider\" key")
	}
	if _, ok := route["usedProvider"]; !ok {
		t.Fatal("route missing \"usedProvider\" key")
	}
	if _, ok := route["browserops"]; ok {
		t.Fatal("route must not contain \"browserops\" key")
	}

	if got := route["requestedProvider"]; got != "chrome" {
		t.Fatalf("requestedProvider = %v, want \"chrome\"", got)
	}
	if got := route["usedProvider"]; got != "chrome" {
		t.Fatalf("usedProvider = %v, want \"chrome\"", got)
	}
	if got := route["escalated"]; got != false {
		t.Fatalf("escalated = %v, want false", got)
	}

	attemptsRaw, ok := route["attempts"]
	if !ok {
		t.Fatal("route missing \"attempts\" key")
	}
	attempts, ok := attemptsRaw.([]any)
	if !ok {
		t.Fatalf("attempts is %T, want []any", attemptsRaw)
	}
	if len(attempts) == 0 {
		t.Fatal("attempts is empty, want at least one entry")
	}
	first, ok := attempts[0].(map[string]any)
	if !ok {
		t.Fatalf("attempts[0] is %T, want map[string]any", attempts[0])
	}
	if got := first["provider"]; got != "chrome" {
		t.Fatalf("attempts[0].provider = %v, want \"chrome\"", got)
	}
	if got := first["accepted"]; got != true {
		t.Fatalf("attempts[0].accepted = %v, want true", got)
	}
}

// TestResultStructsOmitEngine verifies that all result structs serialize
// with "route" metadata and never include an "browserops" key.
func TestResultStructsOmitEngine(t *testing.T) {
	route := SingleBrowserRoute("lite")

	tests := []struct {
		name string
		data any
	}{
		{
			name: "NavigateResult",
			data: NavigateResult{TabID: "t1", URL: "https://x.com", Title: "X", Route: route},
		},
		{
			name: "SnapshotResult",
			data: SnapshotResult{Nodes: []SnapshotNode{{Ref: "e1", Role: "button", Name: "OK"}}, Route: route},
		},
		{
			name: "TextResult",
			data: TextResult{Text: "hello", URL: "https://x.com", Route: route},
		},
		{
			name: "ActionResult",
			data: ActionResult{Data: map[string]any{"ok": true}, Route: route},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b, err := json.Marshal(tc.data)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			var m map[string]any
			if err := json.Unmarshal(b, &m); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if _, ok := m["browserops"]; ok {
				t.Fatalf("%s JSON must not contain \"browserops\" key", tc.name)
			}
			routeRaw, ok := m["route"]
			if !ok {
				t.Fatalf("%s JSON must contain \"route\" key", tc.name)
			}
			routeMap, ok := routeRaw.(map[string]any)
			if !ok {
				t.Fatalf("route is %T, want map[string]any", routeRaw)
			}
			if _, ok := routeMap["requestedProvider"]; !ok {
				t.Fatal("route missing requestedProvider")
			}
			if _, ok := routeMap["usedProvider"]; !ok {
				t.Fatal("route missing usedProvider")
			}
		})
	}
}
