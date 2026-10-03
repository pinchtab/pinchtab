package actions

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/pinchtab/semantic/recovery"
	"github.com/spf13/cobra"
)

func newActionCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("css", "", "")
	cmd.Flags().Bool("wait-nav", false, "")
	cmd.Flags().Bool("dismiss-banners", false, "")
	cmd.Flags().Bool("dismiss-known-interstitials", false, "")
	cmd.Flags().String("tab", "", "")
	cmd.Flags().Float64("x", 0, "")
	cmd.Flags().Float64("y", 0, "")
	cmd.Flags().String("button", "", "")
	cmd.Flags().Int("dx", 0, "")
	cmd.Flags().Int("dy", 0, "")
	cmd.Flags().String("dialog-action", "", "")
	cmd.Flags().String("dialog-text", "", "")
	cmd.Flags().String("mode", "", "")
	cmd.Flags().Bool("submit", false, "")
	return cmd
}

func newSimpleCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("tab", "", "")
	cmd.Flags().Bool("snap", false, "")
	cmd.Flags().Bool("snap-diff", false, "")
	cmd.Flags().Bool("text", false, "")
	cmd.Flags().Bool("submit", false, "")
	return cmd
}

func TestClick(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newActionCmd()
	Action(apiclient.Conn{Client: client, Base: m.base()}, "click", "e5", cmd)
	if m.lastPath != "/action" {
		t.Errorf("expected /action, got %s", m.lastPath)
	}
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["kind"] != "click" {
		t.Errorf("expected kind=click, got %v", body["kind"])
	}
	if body["ref"] != "e5" {
		t.Errorf("expected ref=e5, got %v", body["ref"])
	}
}

func TestClickSubmit(t *testing.T) {
	m := newMockServer()
	defer m.close()
	cmd := newActionCmd()
	if err := cmd.Flags().Set("submit", "true"); err != nil {
		t.Fatal(err)
	}

	Action(apiclient.Conn{Client: m.server.Client(), Base: m.base()}, "click", "e5", cmd)
	var body map[string]any
	if err := json.Unmarshal([]byte(m.lastBody), &body); err != nil {
		t.Fatal(err)
	}
	if body["submit"] != true {
		t.Fatalf("submit = %v, want true", body["submit"])
	}
}

func TestPrintActionResultSubmitPendingIsNotOK(t *testing.T) {
	got := captureStdout(t, func() {
		printActionResult("click", map[string]any{
			"success": true,
			"result": map[string]any{
				"postState": map[string]any{"status": "pending"},
			},
		})
	})
	if got != "PENDING\n" {
		t.Fatalf("pending output = %q, want PENDING", got)
	}
}

func TestPrintActionResultSubmitValidationBlockedListsInvalidFields(t *testing.T) {
	got := captureStdout(t, func() {
		printActionResult("click", map[string]any{
			"success": true,
			"result": map[string]any{
				"postState": map[string]any{
					"status": "blocked",
					"signal": "validation_blocked",
					"invalidFields": []any{
						map[string]any{"name": "name", "label": "Full name", "message": "Please fill out this field."},
						map[string]any{"name": "email", "message": "Please include an '@' in the email address."},
					},
				},
			},
		})
	})
	want := "BLOCKED validation_blocked\nname (Full name): Please fill out this field.\nemail: Please include an '@' in the email address.\n"
	if got != want {
		t.Fatalf("blocked output = %q, want %q", got, want)
	}
}

func TestPrintActionResultWaitNavWithoutNavigationSaysSo(t *testing.T) {
	got := captureStdout(t, func() {
		printActionResult("click", map[string]any{
			"success": true,
			"result":  map[string]any{"clicked": true, "navigated": false},
		})
	})
	if got != "OK no navigation\n" {
		t.Fatalf("no-navigation output = %q, want %q", got, "OK no navigation\n")
	}
}

func TestPrintActionResultReportsARecoveredRef(t *testing.T) {
	rematched := recoveryBlock(t, recovery.RecoveryResult{
		Recovered:   true,
		OriginalRef: "e14",
		NewRef:      "e18",
		Confidence:  "high",
		FailureType: recovery.FailureElementStale.String(),
	})
	tests := []struct {
		name     string
		result   map[string]any
		want     string
		wantHint bool
	}{
		{
			name:   "direct hit",
			result: map[string]any{"success": true, "result": map[string]any{"clicked": true}},
			want:   "OK\n",
		},
		{
			name:   "recovery attempted but not needed",
			result: map[string]any{"success": true, "recovery": recoveryBlock(t, recovery.RecoveryResult{OriginalRef: "e14"})},
			want:   "OK\n",
		},
		{
			name:   "malformed recovery block",
			result: map[string]any{"success": true, "recovery": map[string]any{"recovered": "yes", "new_ref": "e18"}},
			want:   "OK\n",
		},
		{
			name:   "recovery block of the wrong shape",
			result: map[string]any{"success": true, "recovery": "e18"},
			want:   "OK\n",
		},
		{
			name:     "recovered without a new ref",
			result:   map[string]any{"success": true, "recovery": recoveryBlock(t, recovery.RecoveryResult{Recovered: true, OriginalRef: "e14"})},
			want:     "OK recovered e14 -> \n",
			wantHint: true,
		},
		{
			name:     "recovered",
			result:   map[string]any{"success": true, "result": map[string]any{"clicked": true}, "recovery": rematched},
			want:     "OK recovered e14 -> e18 (high)\n",
			wantHint: true,
		},
		{
			name: "recovered and navigated",
			result: map[string]any{
				"success":  true,
				"result":   map[string]any{"clicked": true, "navigated": true, "url": "https://example.com/next"},
				"recovery": rematched,
			},
			want:     "OK navigated https://example.com/next\nrecovered e14 -> e18 (high)\n",
			wantHint: true,
		},
		{
			name: "re-matched onto the same ref after a navigation cleared the refs",
			result: map[string]any{"success": true, "recovery": recoveryBlock(t, recovery.RecoveryResult{
				Recovered: true, OriginalRef: "e1", NewRef: "e1", Confidence: "high", FailureType: recovery.FailureNavigation.String(),
			})},
			want:     "OK re-matched e1 after navigation (high)\n",
			wantHint: true,
		},
		{
			name: "retargeted after a navigation cleared the refs",
			result: map[string]any{"success": true, "recovery": recoveryBlock(t, recovery.RecoveryResult{
				Recovered: true, OriginalRef: "e1", NewRef: "e3", Confidence: "high", FailureType: recovery.FailureNavigation.String(),
			})},
			want:     "OK recovered e1 -> e3 after navigation (high)\n",
			wantHint: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := captureStdout(t, func() { printActionResult("click", tt.result) }); got != tt.want {
				t.Errorf("stdout = %q, want %q", got, tt.want)
			}
			hint := captureStderr(t, func() { printActionResult("click", tt.result) })
			if got := strings.Contains(hint, "re-matched"); got != tt.wantHint {
				t.Errorf("stderr = %q; re-match hint present = %v, want %v", hint, got, tt.wantHint)
			}
		})
	}
}

func recoveryBlock(t *testing.T, rec recovery.RecoveryResult) map[string]any {
	t.Helper()
	raw, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal recovery: %v", err)
	}
	var block map[string]any
	if err := json.Unmarshal(raw, &block); err != nil {
		t.Fatalf("unmarshal recovery: %v", err)
	}
	return block
}

func TestFailedOnStaleRefMatchesTheEngineVocabulary(t *testing.T) {
	for failType, want := range map[string]bool{
		recovery.FailureElementStale.String():    true,
		recovery.FailureNavigation.String():      true,
		recovery.FailureElementNotFound.String(): false,
		"stale":                                  false,
	} {
		got := failedOnStaleRef(decodeRecovery(map[string]any{"recovery": recoveryBlock(t, recovery.RecoveryResult{FailureType: failType})}))
		if got != want {
			t.Errorf("failedOnStaleRef(%q) = %v, want %v", failType, got, want)
		}
	}
}

const staleFailureChildEnv = "PINCHTAB_TEST_STALE_FAILURE_TYPE"

func TestPrintActionResultFailureHintsReSnapOnlyForAStaleRef(t *testing.T) {
	if failType := os.Getenv(staleFailureChildEnv); failType != "" {
		printActionResult("click", map[string]any{
			"success":  false,
			"error":    "node gone",
			"recovery": recoveryBlock(t, recovery.RecoveryResult{FailureType: failType}),
		})
		return
	}

	for failType, wantHint := range map[string]bool{
		recovery.FailureElementStale.String():    true,
		recovery.FailureNavigation.String():      true,
		recovery.FailureElementNotFound.String(): false,
	} {
		t.Run(failType, func(t *testing.T) {
			child := exec.Command(os.Args[0], "-test.run=TestPrintActionResultFailureHintsReSnapOnlyForAStaleRef", "-test.timeout=60s") // #nosec G204 -- re-executes this test binary.
			child.Env = append(os.Environ(), staleFailureChildEnv+"="+failType)
			raw, err := child.CombinedOutput()

			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) {
				t.Fatalf("failed action exited with %v, want non-zero; output:\n%s", err, raw)
			}
			if !strings.Contains(string(raw), "ERROR: click: node gone") {
				t.Errorf("output lacks the error line:\n%s", raw)
			}
			if got := strings.Contains(string(raw), "snap -i"); got != wantHint {
				t.Errorf("re-snap hint present = %v, want %v; output:\n%s", got, wantHint, raw)
			}
		})
	}
}

func TestPrintActionResultSubmitSuccessNamesObservedSignal(t *testing.T) {
	got := captureStdout(t, func() {
		printActionResult("click", map[string]any{
			"success": true,
			"result": map[string]any{
				"postState": map[string]any{"status": "succeeded", "signal": "dialog_closed"},
			},
		})
	})
	if got != "SUCCEEDED dialog_closed\n" {
		t.Fatalf("success output = %q, want observed submit signal", got)
	}
}

func TestPrintActionResultSubmitSuccessPrintsLiveRegionTextAsServed(t *testing.T) {
	wrapped := "<untrusted_web_content>\nOrder saved\n</untrusted_web_content>"
	tests := []struct {
		name      string
		postState map[string]any
		want      string
	}{
		{
			name:      "plain text follows the headline",
			postState: map[string]any{"status": "succeeded", "signal": "live_region_changed", "liveRegionText": "Order saved"},
			want:      "SUCCEEDED live_region_changed\nOrder saved\n",
		},
		{
			name:      "untrusted-content wrapper is kept",
			postState: map[string]any{"status": "succeeded", "signal": "live_region_changed", "liveRegionText": wrapped},
			want:      "SUCCEEDED live_region_changed\n" + wrapped + "\n",
		},
		{
			name:      "blocked text adds no line",
			postState: map[string]any{"status": "succeeded", "signal": "live_region_changed", "liveRegionBlocked": true, "liveRegionBlockReason": "injection pattern"},
			want:      "SUCCEEDED live_region_changed\n",
		},
		{
			name:      "pending ignores live region text",
			postState: map[string]any{"status": "pending", "liveRegionText": "Saving"},
			want:      "PENDING\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := captureStdout(t, func() {
				printActionResult("click", map[string]any{
					"success": true,
					"result":  map[string]any{"postState": tt.postState},
				})
			})
			if got != tt.want {
				t.Fatalf("output = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClickWaitNav(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newActionCmd()
	_ = cmd.Flags().Set("wait-nav", "true")
	Action(apiclient.Conn{Client: client, Base: m.base()}, "click", "e5", cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["waitNav"] != true {
		t.Error("expected waitNav=true")
	}
}

func TestClickDismissBannersWithWaitNav(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newActionCmd()
	_ = cmd.Flags().Set("wait-nav", "true")
	_ = cmd.Flags().Set("dismiss-banners", "true")
	Action(apiclient.Conn{Client: client, Base: m.base()}, "click", "e5", cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["waitNav"] != true {
		t.Error("expected waitNav=true")
	}
	if body["dismissBanners"] != true {
		t.Errorf("expected dismissBanners=true, got %v", body["dismissBanners"])
	}
}

func TestClickDismissBannersWithoutWaitNavIsNoop(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newActionCmd()
	_ = cmd.Flags().Set("dismiss-banners", "true")
	Action(apiclient.Conn{Client: client, Base: m.base()}, "click", "e5", cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if _, ok := body["dismissBanners"]; ok {
		t.Errorf("expected dismissBanners not sent without --wait-nav, got %v", body["dismissBanners"])
	}
}

func TestClickDismissKnownInterstitialsIsIndependentOfWaitNav(t *testing.T) {
	m := newMockServer()
	defer m.close()
	cmd := newActionCmd()
	_ = cmd.Flags().Set("dismiss-known-interstitials", "true")

	Action(apiclient.Conn{Client: m.server.Client(), Base: m.base()}, "click", "e5", cmd)
	var body map[string]any
	if err := json.Unmarshal([]byte(m.lastBody), &body); err != nil {
		t.Fatal(err)
	}
	if body["dismissKnownInterstitials"] != true {
		t.Fatalf("dismissKnownInterstitials = %v, want true", body["dismissKnownInterstitials"])
	}
}

func TestClickMode(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newActionCmd()
	_ = cmd.Flags().Set("mode", "dom")
	Action(apiclient.Conn{Client: client, Base: m.base()}, "click", "e5", cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["mode"] != "dom" {
		t.Errorf("expected mode=dom, got %v", body["mode"])
	}
}

func TestClickDialogAction(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newActionCmd()
	_ = cmd.Flags().Set("dialog-action", "accept")
	_ = cmd.Flags().Set("dialog-text", "hello")
	Action(apiclient.Conn{Client: client, Base: m.base()}, "click", "#alert-btn", cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["dialogAction"] != "accept" {
		t.Errorf("expected dialogAction=accept, got %v", body["dialogAction"])
	}
	if body["dialogText"] != "hello" {
		t.Errorf("expected dialogText=hello, got %v", body["dialogText"])
	}
}

func TestClickDialogActionOmittedByDefault(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newActionCmd()
	Action(apiclient.Conn{Client: client, Base: m.base()}, "click", "#button", cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if _, present := body["dialogAction"]; present {
		t.Errorf("expected dialogAction to be omitted, got %v", body["dialogAction"])
	}
	if _, present := body["dialogText"]; present {
		t.Errorf("expected dialogText to be omitted, got %v", body["dialogText"])
	}
}

func TestType(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newSimpleCmd()
	ActionSimple(apiclient.Conn{Client: client, Base: m.base()}, "type", []string{"e12", "hello", "world"}, cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["kind"] != "type" {
		t.Errorf("expected kind=type, got %v", body["kind"])
	}
	if body["ref"] != "e12" {
		t.Errorf("expected ref=e12, got %v", body["ref"])
	}
	if body["text"] != "hello world" {
		t.Errorf("expected text='hello world', got %v", body["text"])
	}
}

func TestPress(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newSimpleCmd()
	ActionSimple(apiclient.Conn{Client: client, Base: m.base()}, "press", []string{"Enter"}, cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["key"] != "Enter" {
		t.Errorf("expected key=Enter, got %v", body["key"])
	}
}

func TestPressWithSnapDiffFetchesSnapshot(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()
	m.setResponse("GET", "/snapshot", 200, "snapshot")

	cmd := newSimpleCmd()
	_ = cmd.Flags().Set("snap-diff", "true")
	ActionSimple(apiclient.Conn{Client: client, Base: m.base()}, "press", []string{"Enter"}, cmd)

	if len(m.requests) != 3 {
		t.Fatalf("expected 3 requests (action + settle + snapshot), got %d", len(m.requests))
	}
	if m.requests[2].Path != "/snapshot" {
		t.Fatalf("snapshot path = %q, want /snapshot", m.requests[2].Path)
	}
	if m.requests[2].Query != (url.Values{"filter": {"interactive"}, "format": {"compact"}, "diff": {"true"}}).Encode() {
		t.Fatalf("snapshot query = %q", m.requests[2].Query)
	}
}

func TestClickWithCSS(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newActionCmd()
	_ = cmd.Flags().Set("css", "button.submit")
	Action(apiclient.Conn{Client: client, Base: m.base()}, "click", "", cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["selector"] != "button.submit" {
		t.Errorf("expected selector=button.submit, got %v", body["selector"])
	}
	if _, hasRef := body["ref"]; hasRef {
		t.Error("should not set ref when --css is provided")
	}
}

func TestClickWithCSS_AndWaitNav(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newActionCmd()
	_ = cmd.Flags().Set("wait-nav", "true")
	_ = cmd.Flags().Set("css", "#login-btn")
	Action(apiclient.Conn{Client: client, Base: m.base()}, "click", "", cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["selector"] != "#login-btn" {
		t.Errorf("expected selector=#login-btn, got %v", body["selector"])
	}
	if body["waitNav"] != true {
		t.Error("expected waitNav=true")
	}
}

func TestMouseDownIncludesButton(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newActionCmd()
	_ = cmd.Flags().Set("button", "right")
	_ = cmd.Flags().Set("x", "25")
	_ = cmd.Flags().Set("y", "40")

	MouseAction(apiclient.Conn{Client: client, Base: m.base()}, "mouse-down", nil, cmd)

	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["kind"] != "mouse-down" {
		t.Errorf("expected kind=mouse-down, got %v", body["kind"])
	}
	if body["button"] != "right" {
		t.Errorf("expected button=right, got %v", body["button"])
	}
	if body["x"] != float64(25) || body["y"] != float64(40) {
		t.Errorf("expected x/y coordinates, got %v", body)
	}
}

func TestMouseWheelIncludesExplicitDeltas(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newActionCmd()
	_ = cmd.Flags().Set("dx", "120")
	_ = cmd.Flags().Set("dy", "-300")
	_ = cmd.Flags().Set("x", "10")
	_ = cmd.Flags().Set("y", "20")

	MouseAction(apiclient.Conn{Client: client, Base: m.base()}, "mouse-wheel", nil, cmd)

	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["kind"] != "mouse-wheel" {
		t.Errorf("expected kind=mouse-wheel, got %v", body["kind"])
	}
	if body["deltaX"] != float64(120) {
		t.Errorf("expected deltaX=120, got %v", body["deltaX"])
	}
	if body["deltaY"] != float64(-300) {
		t.Errorf("expected deltaY=-300, got %v", body["deltaY"])
	}
}

func TestMouseMoveSupportsPositionalCoordinates(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newActionCmd()
	MouseAction(apiclient.Conn{Client: client, Base: m.base()}, "mouse-move", []string{"100", "200"}, cmd)

	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["kind"] != "mouse-move" {
		t.Fatalf("expected kind=mouse-move, got %v", body["kind"])
	}
	if body["x"] != float64(100) || body["y"] != float64(200) {
		t.Fatalf("expected positional coordinates, got %v", body)
	}
}

func TestMouseWheelSupportsPositionalDeltaY(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newActionCmd()
	_ = cmd.Flags().Set("dx", "20")
	MouseAction(apiclient.Conn{Client: client, Base: m.base()}, "mouse-wheel", []string{"-120"}, cmd)

	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["deltaX"] != float64(20) {
		t.Fatalf("expected deltaX=20, got %v", body["deltaX"])
	}
	if body["deltaY"] != float64(-120) {
		t.Fatalf("expected deltaY=-120, got %v", body["deltaY"])
	}
}

// The from->to form used to post four independent pointer requests, and four requests
// cannot interpolate: the pointer jumped from source to destination in one move, which
// Chrome never reads as the start of a drag, so an HTML5 draggable saw nothing while all
// four answered OK. The destination has to travel WITH the source in one action.
func TestDragPostsOneActionCarryingItsDestination(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newActionCmd()
	Drag(apiclient.Conn{Client: client, Base: m.base()}, []string{"e5", "e9"}, cmd)

	if len(m.requests) != 1 {
		t.Fatalf("expected 1 request, got %d; a drag assembled from several requests cannot interpolate the pointer", len(m.requests))
	}
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["kind"] != "drag" {
		t.Errorf("kind = %v, want drag", body["kind"])
	}
	if body["ref"] != "e5" {
		t.Errorf("source = %+v, want ref e5", body)
	}
	if body["toSelector"] != "e9" {
		t.Errorf("destination = %+v, want toSelector e9", body)
	}
}

func TestDragToCoordinatesPostsThemAsTheDestination(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newActionCmd()
	Drag(apiclient.Conn{Client: client, Base: m.base()}, []string{"e5", "400,320"}, cmd)

	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["toX"] != float64(400) || body["toY"] != float64(320) {
		t.Errorf("destination = %+v, want toX=400 toY=320", body)
	}
	if _, ok := body["toSelector"]; ok {
		t.Errorf("destination = %+v, want no toSelector when the target is a coordinate pair", body)
	}
}

func TestHoverWithCSS(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newActionCmd()
	_ = cmd.Flags().Set("css", ".nav-item")
	Action(apiclient.Conn{Client: client, Base: m.base()}, "hover", "", cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["selector"] != ".nav-item" {
		t.Errorf("expected selector=.nav-item, got %v", body["selector"])
	}
}

func TestFocus(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newActionCmd()
	Action(apiclient.Conn{Client: client, Base: m.base()}, "focus", "e5", cmd)
	if m.lastPath != "/action" {
		t.Errorf("expected /action, got %s", m.lastPath)
	}
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["kind"] != "focus" {
		t.Errorf("expected kind=focus, got %v", body["kind"])
	}
	if body["ref"] != "e5" {
		t.Errorf("expected ref=e5, got %v", body["ref"])
	}
}

func TestFocusWithCSS(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newActionCmd()
	_ = cmd.Flags().Set("css", "input[name='email']")
	Action(apiclient.Conn{Client: client, Base: m.base()}, "focus", "", cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["selector"] != "input[name='email']" {
		t.Errorf("expected selector=input[name='email'], got %v", body["selector"])
	}
}

func TestClickRefStillWorks(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newActionCmd()
	Action(apiclient.Conn{Client: client, Base: m.base()}, "click", "e42", cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["ref"] != "e42" {
		t.Errorf("expected ref=e42, got %v", body["ref"])
	}
	if _, hasSelector := body["selector"]; hasSelector {
		t.Error("should not set selector when using ref")
	}
}

func TestFill(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newSimpleCmd()
	ActionSimple(apiclient.Conn{Client: client, Base: m.base()}, "fill", []string{"e3", "test value"}, cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["ref"] != "e3" {
		t.Errorf("expected ref=e3, got %v", body["ref"])
	}
	if body["text"] != "test value" {
		t.Errorf("expected text='test value', got %v", body["text"])
	}

	ActionSimple(apiclient.Conn{Client: client, Base: m.base()}, "fill", []string{"#email", "user@test.com"}, cmd)
	body = nil
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["selector"] != "#email" {
		t.Errorf("expected selector=#email, got %v", body["selector"])
	}

	ActionSimple(apiclient.Conn{Client: client, Base: m.base()}, "fill", []string{"embed", "inline content"}, cmd)
	body = nil
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["selector"] != "embed" {
		t.Errorf("expected selector=embed, got %v", body["selector"])
	}
	if _, hasRef := body["ref"]; hasRef {
		t.Errorf("expected no ref for selector embed, got %v", body["ref"])
	}
}

func TestFillSubmit(t *testing.T) {
	m := newMockServer()
	defer m.close()
	cmd := newSimpleCmd()
	if err := cmd.Flags().Set("submit", "true"); err != nil {
		t.Fatal(err)
	}

	ActionSimple(apiclient.Conn{Client: m.server.Client(), Base: m.base()}, "fill", []string{"#search", "needle"}, cmd)
	var body map[string]any
	if err := json.Unmarshal([]byte(m.lastBody), &body); err != nil {
		t.Fatal(err)
	}
	if body["submit"] != true {
		t.Fatalf("submit = %v, want true", body["submit"])
	}
}

func TestScroll(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newSimpleCmd()
	ActionSimple(apiclient.Conn{Client: client, Base: m.base()}, "scroll", []string{"e20"}, cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["ref"] != "e20" {
		t.Errorf("expected ref=e20, got %v", body["ref"])
	}

	ActionSimple(apiclient.Conn{Client: client, Base: m.base()}, "scroll", []string{"800"}, cmd)
	body = nil
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["scrollY"] != float64(800) {
		t.Errorf("expected scrollY=800, got %v", body["scrollY"])
	}

	ActionSimple(apiclient.Conn{Client: client, Base: m.base()}, "scroll", []string{"down"}, cmd)
	body = nil
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["scrollY"] != float64(800) {
		t.Errorf("expected scrollY=800 for direction=down, got %v", body["scrollY"])
	}

	// CSS selector auto-detection: `scroll #footer` should forward as
	// selector, matching how click/fill/hover behave for bare selectors.
	ActionSimple(apiclient.Conn{Client: client, Base: m.base()}, "scroll", []string{"#footer"}, cmd)
	body = nil
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["selector"] != "#footer" {
		t.Errorf("expected selector=#footer, got %v", body["selector"])
	}
	if _, hasScrollY := body["scrollY"]; hasScrollY {
		t.Errorf("should not set scrollY for CSS selector form, got %v", body["scrollY"])
	}

	// XPath also flows through.
	ActionSimple(apiclient.Conn{Client: client, Base: m.base()}, "scroll", []string{"//footer"}, cmd)
	body = nil
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["selector"] != "//footer" {
		t.Errorf("expected selector=//footer, got %v", body["selector"])
	}
}

func TestCheck(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newActionCmd()
	Action(apiclient.Conn{Client: client, Base: m.base()}, "check", "e7", cmd)
	if m.lastPath != "/action" {
		t.Errorf("expected /action, got %s", m.lastPath)
	}
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["kind"] != "check" {
		t.Errorf("expected kind=check, got %v", body["kind"])
	}
	if body["ref"] != "e7" {
		t.Errorf("expected ref=e7, got %v", body["ref"])
	}
	if _, hasSelector := body["selector"]; hasSelector {
		t.Error("should not set selector when using ref")
	}
}

func TestCheckWithCSS(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newActionCmd()
	_ = cmd.Flags().Set("css", "input[type=checkbox]")
	Action(apiclient.Conn{Client: client, Base: m.base()}, "check", "", cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["kind"] != "check" {
		t.Errorf("expected kind=check, got %v", body["kind"])
	}
	if body["selector"] != "input[type=checkbox]" {
		t.Errorf("expected selector=input[type=checkbox], got %v", body["selector"])
	}
	if _, hasRef := body["ref"]; hasRef {
		t.Error("should not set ref when --css is provided")
	}
}

func TestUncheck(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newActionCmd()
	Action(apiclient.Conn{Client: client, Base: m.base()}, "uncheck", "e9", cmd)
	if m.lastPath != "/action" {
		t.Errorf("expected /action, got %s", m.lastPath)
	}
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["kind"] != "uncheck" {
		t.Errorf("expected kind=uncheck, got %v", body["kind"])
	}
	if body["ref"] != "e9" {
		t.Errorf("expected ref=e9, got %v", body["ref"])
	}
	if _, hasSelector := body["selector"]; hasSelector {
		t.Error("should not set selector when using ref")
	}
}

func TestUncheckWithCSS(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newActionCmd()
	_ = cmd.Flags().Set("css", "#agree-checkbox")
	Action(apiclient.Conn{Client: client, Base: m.base()}, "uncheck", "", cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["kind"] != "uncheck" {
		t.Errorf("expected kind=uncheck, got %v", body["kind"])
	}
	if body["selector"] != "#agree-checkbox" {
		t.Errorf("expected selector=#agree-checkbox, got %v", body["selector"])
	}
	if _, hasRef := body["ref"]; hasRef {
		t.Error("should not set ref when --css is provided")
	}
}

func TestSelect(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newSimpleCmd()
	ActionSimple(apiclient.Conn{Client: client, Base: m.base()}, "select", []string{"e10", "option2"}, cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["ref"] != "e10" {
		t.Errorf("expected ref=e10, got %v", body["ref"])
	}
	if body["value"] != "option2" {
		t.Errorf("expected value=option2, got %v", body["value"])
	}
}

func TestKeyboardType(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newSimpleCmd()
	ActionSimple(apiclient.Conn{Client: client, Base: m.base()}, "keyboard-type", []string{"hello", "world"}, cmd)
	if m.lastPath != "/action" {
		t.Errorf("expected /action, got %s", m.lastPath)
	}
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["kind"] != "keyboard-type" {
		t.Errorf("expected kind=keyboard-type, got %v", body["kind"])
	}
	if body["text"] != "hello world" {
		t.Errorf("expected text='hello world', got %v", body["text"])
	}
	if _, has := body["selector"]; has {
		t.Error("keyboard-type should not have selector")
	}
	if _, has := body["ref"]; has {
		t.Error("keyboard-type should not have ref")
	}
}

func TestKeyboardInsertText(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newSimpleCmd()
	ActionSimple(apiclient.Conn{Client: client, Base: m.base()}, "keyboard-inserttext", []string{"pasted", "text"}, cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["kind"] != "keyboard-inserttext" {
		t.Errorf("expected kind=keyboard-inserttext, got %v", body["kind"])
	}
	if body["text"] != "pasted text" {
		t.Errorf("expected text='pasted text', got %v", body["text"])
	}
}

func TestKeyDown(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newSimpleCmd()
	ActionSimple(apiclient.Conn{Client: client, Base: m.base()}, "keydown", []string{"Control"}, cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["kind"] != "keydown" {
		t.Errorf("expected kind=keydown, got %v", body["kind"])
	}
	if body["key"] != "Control" {
		t.Errorf("expected key=Control, got %v", body["key"])
	}
}

func TestKeyUp(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newSimpleCmd()
	ActionSimple(apiclient.Conn{Client: client, Base: m.base()}, "keyup", []string{"Shift"}, cmd)
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["kind"] != "keyup" {
		t.Errorf("expected kind=keyup, got %v", body["kind"])
	}
	if body["key"] != "Shift" {
		t.Errorf("expected key=Shift, got %v", body["key"])
	}
}

func TestKeyDownWithTab(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newSimpleCmd()
	_ = cmd.Flags().Set("tab", "abc123")
	ActionSimple(apiclient.Conn{Client: client, Base: m.base()}, "keydown", []string{"Alt"}, cmd)
	if m.lastPath != "/tabs/abc123/action" {
		t.Errorf("expected /tabs/abc123/action, got %s", m.lastPath)
	}
	var body map[string]any
	_ = json.Unmarshal([]byte(m.lastBody), &body)
	if body["kind"] != "keydown" {
		t.Errorf("expected kind=keydown, got %v", body["kind"])
	}
	if body["key"] != "Alt" {
		t.Errorf("expected key=Alt, got %v", body["key"])
	}
}

func TestKeyboardTypeWithTab(t *testing.T) {
	m := newMockServer()
	defer m.close()
	client := m.server.Client()

	cmd := newSimpleCmd()
	_ = cmd.Flags().Set("tab", "tab42")
	ActionSimple(apiclient.Conn{Client: client, Base: m.base()}, "keyboard-type", []string{"test"}, cmd)
	if m.lastPath != "/tabs/tab42/action" {
		t.Errorf("expected /tabs/tab42/action, got %s", m.lastPath)
	}
}

// newScrollCmd carries the flags cmd/pinchtab registers on `scroll`, so a body built here is
// built from the same inputs production reads.
func newScrollCmd() *cobra.Command {
	cmd := newSimpleCmd()
	cmd.Flags().Int("dy", 0, "")
	cmd.Flags().Int("dx", 0, "")
	return cmd
}

// Scrolling up by an exact pixel count is what had no working spelling: a positional "-300"
// cannot reach the command (cobra reads it as shorthand flags), and `-- -300` swallowed
// --tab. The flag is the reachable route, and negatives are fine as flag VALUES.
func TestScrollByPixelFlags(t *testing.T) {
	for _, tc := range []struct {
		name  string
		flags map[string]string
		want  map[string]float64
	}{
		{name: "up", flags: map[string]string{"dy": "-300"}, want: map[string]float64{"scrollY": -300}},
		{name: "down", flags: map[string]string{"dy": "800"}, want: map[string]float64{"scrollY": 800}},
		{name: "left", flags: map[string]string{"dx": "-120"}, want: map[string]float64{"scrollX": -120}},
		{name: "both axes", flags: map[string]string{"dy": "-300", "dx": "40"}, want: map[string]float64{"scrollY": -300, "scrollX": 40}},
	} {
		m := newMockServer()
		cmd := newScrollCmd()
		for flag, value := range tc.flags {
			if err := cmd.Flags().Set(flag, value); err != nil {
				t.Fatal(err)
			}
		}

		ActionSimple(apiclient.Conn{Client: m.server.Client(), Base: m.base()}, "scroll", nil, cmd)

		var body map[string]any
		_ = json.Unmarshal([]byte(m.lastBody), &body)
		for key, want := range tc.want {
			if body[key] != want {
				t.Errorf("%s: %s = %v, want %v (body %+v)", tc.name, key, body[key], want, body)
			}
		}
		m.close()
	}
}

// The positional forms are what everything else in the docs teaches, and adding the flags
// touched the same precedence path that resolves them.
func TestScrollPositionalFormsAreUnchangedByThePixelFlags(t *testing.T) {
	for _, tc := range []struct {
		arg  string
		key  string
		want any
	}{
		{arg: "800", key: "scrollY", want: float64(800)},
		{arg: "down", key: "scrollY", want: float64(800)},
		{arg: "up", key: "scrollY", want: float64(-800)},
		{arg: "right", key: "scrollX", want: float64(800)},
		{arg: "left", key: "scrollX", want: float64(-800)},
		{arg: "e12", key: "ref", want: "e12"},
		{arg: "#footer", key: "selector", want: "#footer"},
		{arg: "text:Load more", key: "selector", want: "text:Load more"},
	} {
		m := newMockServer()
		ActionSimple(apiclient.Conn{Client: m.server.Client(), Base: m.base()}, "scroll", []string{tc.arg}, newScrollCmd())

		var body map[string]any
		_ = json.Unmarshal([]byte(m.lastBody), &body)
		if body[tc.key] != tc.want {
			t.Errorf("scroll %q: %s = %v, want %v (body %+v)", tc.arg, tc.key, body[tc.key], tc.want, body)
		}
		m.close()
	}
}

// ActionSimple is exported and reachable without cobra's Args hook, so the CLI's
// refusal of the both-specified form is a friendly early error, not the thing that
// keeps this correct. The rule has to live in the builder: a positional wins
// outright. Before it did, the flags were assigned first and the positional then
// overwrote scrollY — so `800` with `--dx -100` built a diagonal scroll out of one
// axis from each spelling, resolved by statement order rather than by any rule.
func TestScrollPositionalWinsOverThePixelFlagsWithoutCobrasArgsHook(t *testing.T) {
	for _, tc := range []struct {
		name   string
		arg    string
		flags  map[string]string
		want   map[string]any
		absent []string
	}{
		{
			name:   "same axis",
			arg:    "800",
			flags:  map[string]string{"dy": "-300"},
			want:   map[string]any{"scrollY": float64(800)},
			absent: []string{"scrollX"},
		},
		{
			name:   "other axis must not survive as half a diagonal",
			arg:    "800",
			flags:  map[string]string{"dx": "-100"},
			want:   map[string]any{"scrollY": float64(800)},
			absent: []string{"scrollX"},
		},
		{
			name:   "a direction keyword also wins",
			arg:    "left",
			flags:  map[string]string{"dy": "-300"},
			want:   map[string]any{"scrollX": float64(-800)},
			absent: []string{"scrollY"},
		},
		{
			name:   "a selector wins and carries no delta",
			arg:    "e12",
			flags:  map[string]string{"dy": "-300", "dx": "40"},
			want:   map[string]any{"ref": "e12"},
			absent: []string{"scrollX", "scrollY"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newMockServer()
			defer m.close()
			cmd := newScrollCmd()
			for flag, value := range tc.flags {
				if err := cmd.Flags().Set(flag, value); err != nil {
					t.Fatal(err)
				}
			}

			ActionSimple(apiclient.Conn{Client: m.server.Client(), Base: m.base()}, "scroll", []string{tc.arg}, cmd)

			var body map[string]any
			if err := json.Unmarshal([]byte(m.lastBody), &body); err != nil {
				t.Fatalf("decode body: %v (%s)", err, m.lastBody)
			}
			for key, want := range tc.want {
				if body[key] != want {
					t.Errorf("%s = %v, want %v (body %+v)", key, body[key], want, body)
				}
			}
			for _, key := range tc.absent {
				if _, present := body[key]; present {
					t.Errorf("%s is present (%v); the positional is the whole argument, so no flag axis may survive alongside it (body %+v)", key, body[key], body)
				}
			}
		})
	}
}

// The repro from the card: clicking an ordinary link. It used to print an error and
// exit 1 after the click had already worked; it now prints where the tab landed and
// exits 0. captureStdout would not see an exit, so the assertion is on the output —
// output.Error is the only path that exits, and reaching this branch means it did not.
func TestPrintActionResultANavigatingClickReportsWhereItLanded(t *testing.T) {
	navigated := map[string]any{
		"success": true,
		"result": map[string]any{
			"clicked":     true,
			"navigated":   true,
			"url":         "https://www.iana.org/help/example-domains",
			"previousUrl": "https://example.com/",
			"refsStale":   true,
		},
	}

	out := captureStdout(t, func() { printActionResult("click", navigated) })
	if !strings.Contains(out, "https://www.iana.org/help/example-domains") {
		t.Errorf("stdout = %q; the caller cannot see where the click landed", out)
	}

	hint := captureStderr(t, func() { printActionResult("click", navigated) })
	if !strings.Contains(hint, "snap") {
		t.Errorf("stderr = %q; nothing tells the caller its refs are dead", hint)
	}
}

// A click that did not move the page keeps the plain answer: the landed-URL line is
// the navigation report, not decoration on every click.
func TestPrintActionResultAClickThatDidNotNavigateStaysPlain(t *testing.T) {
	got := captureStdout(t, func() {
		printActionResult("click", map[string]any{
			"success": true,
			"result":  map[string]any{"clicked": true},
		})
	})
	if strings.Contains(got, "navigated") {
		t.Errorf("output = %q; a click that moved nothing reported a navigation", got)
	}
}

func TestPrintActionResultScrollSaysWhetherThePageMoved(t *testing.T) {
	tests := []struct {
		name   string
		result map[string]any
		want   string
	}{
		{
			name:   "at the end of the page",
			result: map[string]any{"scrolled": false, "scrollX": float64(0), "scrollY": float64(2300)},
			want:   "OK (did not move, scrollX 0, scrollY 2300)\n",
		},
		{
			name:   "moved",
			result: map[string]any{"scrolled": true, "scrollX": float64(0), "scrollY": float64(800)},
			want:   "OK\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := captureStdout(t, func() {
				printActionResult("scroll", map[string]any{"success": true, "result": tt.result})
			})
			if got != tt.want {
				t.Fatalf("scroll output = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestScrollCommandReportsAStuckPageInTextAndPassesJSONThrough(t *testing.T) {
	const stuck = `{"success":true,"result":{"scrolled":false,"scrollX":0,"scrollY":2300}}`
	tests := []struct {
		name string
		json bool
		want string
	}{
		{name: "text", want: "OK (did not move, scrollX 0, scrollY 2300)\n"},
		{name: "json", json: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newMockServer()
			defer m.close()
			m.setResponse("POST", "/action", 200, stuck)
			cmd := newSimpleCmd()
			cmd.Flags().Bool("json", tt.json, "")
			got := captureStdout(t, func() {
				ActionSimple(apiclient.Conn{Client: m.server.Client(), Base: m.base()}, "scroll", []string{"down"}, cmd)
			})
			if tt.json {
				var decoded map[string]any
				if err := json.Unmarshal([]byte(got), &decoded); err != nil {
					t.Fatalf("--json stdout is not JSON: %q", got)
				}
				if strings.Contains(got, "did not move") {
					t.Fatalf("--json stdout carries the text line: %q", got)
				}
				return
			}
			if got != tt.want {
				t.Fatalf("scroll stdout = %q, want %q", got, tt.want)
			}
		})
	}
}

// A submit that redirects keeps its own post-state headline — that says more than
// "it moved" — and reports the landing underneath it. The landing used to be absent
// from this form entirely.
func TestPrintActionResultASubmitThatNavigatedKeepsItsPostStateAndSaysWhereItLanded(t *testing.T) {
	navigatedSubmit := map[string]any{
		"success": true,
		"result": map[string]any{
			"clicked":     true,
			"postState":   map[string]any{"status": "succeeded", "signal": "url_changed"},
			"navigated":   true,
			"url":         "https://shop.example.com/order/confirmed",
			"previousUrl": "https://shop.example.com/checkout",
			"refsStale":   true,
		},
	}

	out := captureStdout(t, func() { printActionResult("click", navigatedSubmit) })
	if !strings.Contains(out, "SUCCEEDED url_changed") {
		t.Errorf("stdout = %q; the submit lost its observed post-state signal", out)
	}
	if !strings.Contains(out, "https://shop.example.com/order/confirmed") {
		t.Errorf("stdout = %q; the submit does not say where it landed", out)
	}

	hint := captureStderr(t, func() { printActionResult("click", navigatedSubmit) })
	if !strings.Contains(hint, "snap") {
		t.Errorf("stderr = %q; nothing tells the caller its refs are dead", hint)
	}
}

const popupClickResponse = `{"success":true,"result":{"clicked":true,"switchedToTab":"TAB_NEW"}}`

func TestPrintActionResultAClickThatOpenedATabNamesIt(t *testing.T) {
	opened := map[string]any{
		"success": true,
		"result":  map[string]any{"clicked": true, "switchedToTab": "TAB_NEW"},
	}

	out := captureStdout(t, func() { printActionResult("click", opened) })
	if strings.TrimSpace(out) != "OK opened tab TAB_NEW" {
		t.Errorf("stdout = %q, want OK opened tab TAB_NEW", out)
	}

	hint := captureStderr(t, func() { printActionResult("click", opened) })
	if !strings.Contains(hint, "pinchtab tab TAB_NEW") {
		t.Errorf("stderr = %q; nothing tells the caller how to switch to the new tab", hint)
	}
}

func TestPrintActionResultASubmitThatOpenedATabNamesItUnderItsHeadline(t *testing.T) {
	tests := []struct {
		postState map[string]any
		headline  string
	}{
		{map[string]any{"status": "pending"}, "PENDING\n"},
		{map[string]any{"status": "blocked", "signal": "validation_blocked"}, "BLOCKED validation_blocked\n"},
		{map[string]any{"status": "succeeded", "signal": "url_changed"}, "SUCCEEDED url_changed\n"},
	}
	for _, tt := range tests {
		t.Run(tt.postState["status"].(string), func(t *testing.T) {
			submit := func(actionResult map[string]any) map[string]any {
				actionResult["postState"] = tt.postState
				return map[string]any{"success": true, "result": actionResult}
			}
			withTab := submit(map[string]any{"clicked": true, "switchedToTab": "TAB_NEW"})
			withoutTab := submit(map[string]any{"clicked": true})

			if got := captureStdout(t, func() { printActionResult("click", withTab) }); got != tt.headline+"opened tab TAB_NEW\n" {
				t.Errorf("stdout = %q, want headline then opened tab TAB_NEW", got)
			}
			if hint := captureStderr(t, func() { printActionResult("click", withTab) }); !strings.Contains(hint, "pinchtab tab TAB_NEW") {
				t.Errorf("stderr = %q; nothing tells the caller how to switch to the new tab", hint)
			}
			if got := captureStdout(t, func() { printActionResult("click", withoutTab) }); got != tt.headline {
				t.Errorf("control stdout = %q, want %q", got, tt.headline)
			}
			if hint := captureStderr(t, func() { printActionResult("click", withoutTab) }); strings.Contains(hint, "pinchtab tab") {
				t.Errorf("control stderr = %q; a submit that opened no tab mentioned one", hint)
			}
		})
	}
}

func TestClickThatOpenedATabFetchesSnapAndTextFromTheNewTab(t *testing.T) {
	for _, flag := range []string{"snap", "snap-diff", "text"} {
		t.Run(flag, func(t *testing.T) {
			m := newMockServer()
			defer m.close()
			m.setResponse("POST", "/action", 200, popupClickResponse)

			cmd := newActionCmd()
			cmd.Flags().Bool("snap", false, "")
			cmd.Flags().Bool("snap-diff", false, "")
			cmd.Flags().Bool("text", false, "")
			_ = cmd.Flags().Set(flag, "true")
			captureStdout(t, func() {
				Action(apiclient.Conn{Client: m.server.Client(), Base: m.base()}, "click", "e3", cmd)
			})

			if len(m.requests) != 3 {
				t.Fatalf("expected 3 requests (action + settle + follow-up), got %d", len(m.requests))
			}
			if settle := m.requests[1]; settle.Path != "/tabs/TAB_NEW/wait" {
				t.Fatalf("settle path = %q, want /tabs/TAB_NEW/wait", settle.Path)
			}
			followUp := m.requests[2]
			if flag == "text" {
				if followUp.Path != "/tabs/TAB_NEW/text" {
					t.Fatalf("text path = %q, want /tabs/TAB_NEW/text", followUp.Path)
				}
				return
			}
			query, _ := url.ParseQuery(followUp.Query)
			if followUp.Path != "/snapshot" || query.Get("tabId") != "TAB_NEW" {
				t.Fatalf("snapshot request = %s?%s, want /snapshot with tabId=TAB_NEW", followUp.Path, followUp.Query)
			}
		})
	}
}

const dialogClickResponse = `{"success":true,"result":{"clicked":true,"dialog":{"tabId":"TAB1","dialogType":"confirm","dialogMessage":"Delete order 1?","remedy":"pinchtab dialog accept"}}}`

func TestPrintActionResultAClickThatOpenedADialogNamesItAndTheNextStep(t *testing.T) {
	var opened map[string]any
	if err := json.Unmarshal([]byte(dialogClickResponse), &opened); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() { printActionResult("click", opened) })
	if strings.TrimSpace(out) != `OK opened confirm dialog: "Delete order 1?"` {
		t.Errorf("stdout = %q, want the dialog type and message after OK", out)
	}

	hint := captureStderr(t, func() { printActionResult("click", opened) })
	for _, next := range []string{"pinchtab dialog accept", "pinchtab dialog dismiss"} {
		if !strings.Contains(hint, next) {
			t.Errorf("stderr = %q; nothing tells the caller to run %s", hint, next)
		}
	}
}

func TestClickThatOpenedADialogSkipsTheSnapAndTextTails(t *testing.T) {
	for _, flag := range []string{"snap", "snap-diff", "text"} {
		t.Run(flag, func(t *testing.T) {
			m := newMockServer()
			defer m.close()
			m.setResponse("POST", "/action", 200, dialogClickResponse)

			cmd := newActionCmd()
			cmd.Flags().Bool("snap", false, "")
			cmd.Flags().Bool("snap-diff", false, "")
			cmd.Flags().Bool("text", false, "")
			_ = cmd.Flags().Set(flag, "true")
			captureStdout(t, func() {
				Action(apiclient.Conn{Client: m.server.Client(), Base: m.base()}, "click", "e3", cmd)
			})

			if len(m.requests) != 1 {
				t.Fatalf("got %d requests, want the action alone: a read behind the open dialog answers 409", len(m.requests))
			}
		})
	}
}

func TestSnapDiffTailSettlesNetworkBeforeReading(t *testing.T) {
	m := newMockServer()
	defer m.close()
	m.setResponse("POST", "/wait", 200, `{"waited":true,"elapsed":12,"match":"network-idle"}`)
	m.setResponse("GET", "/snapshot", 200, "snapshot")

	cmd := newSimpleCmd()
	_ = cmd.Flags().Set("snap-diff", "true")
	out := captureStdout(t, func() {
		ActionSimple(apiclient.Conn{Client: m.server.Client(), Base: m.base()}, "press", []string{"Enter"}, cmd)
	})

	paths := make([]string, 0, len(m.requests))
	for _, r := range m.requests {
		paths = append(paths, r.Path)
	}
	if strings.Join(paths, " ") != "/action /wait /snapshot" {
		t.Fatalf("request order = %q, want action, wait, snapshot", paths)
	}
	var wait map[string]any
	_ = json.Unmarshal([]byte(m.requests[1].Body), &wait)
	if wait["load"] != "network-idle" || wait["timeout"] != float64(settleTimeoutMs) {
		t.Fatalf("wait body = %s, want load=network-idle timeout=%d", m.requests[1].Body, settleTimeoutMs)
	}
	if strings.Contains(out, "settle:") {
		t.Fatalf("settled read printed a settle header: %q", out)
	}
}

func TestTailSettleTimeoutPrintsHeaderThenReads(t *testing.T) {
	m := newMockServer()
	defer m.close()
	m.setResponse("POST", "/wait", 200, `{"waited":false,"elapsed":1500,"error":"timeout after 1500ms waiting for network-idle (inflight=2)"}`)
	m.setResponse("GET", "/text", 200, `{"text":"Showing 10 of 30"}`)

	cmd := newSimpleCmd()
	_ = cmd.Flags().Set("text", "true")
	out := captureStdout(t, func() {
		ActionSimple(apiclient.Conn{Client: m.server.Client(), Base: m.base()}, "press", []string{"Enter"}, cmd)
	})

	if !strings.Contains(out, "settle: timeout after 1500ms waiting for network-idle (inflight=2)\nShowing 10 of 30") {
		t.Fatalf("output = %q, want settle header before the text", out)
	}
}

func TestTailWithoutNetworkCaptureReadsSilently(t *testing.T) {
	m := newMockServer()
	defer m.close()
	m.setResponse("POST", "/wait", 200, `{"waited":false,"elapsed":1,"error":"network monitor unavailable for tab"}`)
	m.setResponse("GET", "/text", 200, `{"text":"page"}`)

	cmd := newSimpleCmd()
	_ = cmd.Flags().Set("text", "true")
	out := captureStdout(t, func() {
		ActionSimple(apiclient.Conn{Client: m.server.Client(), Base: m.base()}, "press", []string{"Enter"}, cmd)
	})

	if strings.Contains(out, "settle:") {
		t.Fatalf("unavailable monitor must read silently, got %q", out)
	}
}

func TestActionWithoutTailFlagsNeverWaits(t *testing.T) {
	m := newMockServer()
	defer m.close()

	ActionSimple(apiclient.Conn{Client: m.server.Client(), Base: m.base()}, "press", []string{"Enter"}, newSimpleCmd())

	if len(m.requests) != 1 || m.requests[0].Path != "/action" {
		t.Fatalf("expected the action alone, got %+v", m.requests)
	}
}

const clickExitChildEnv = "PINCHTAB_CLICK_EXIT_CHILD"

var clickExitResults = map[string]string{
	"blocked":       `{"success":true,"result":{"postState":{"status":"blocked","signal":"validation_blocked"}}}`,
	"succeeded":     `{"success":true,"result":{"postState":{"status":"succeeded","signal":"dialog_closed"}}}`,
	"pending":       `{"success":true,"result":{"postState":{"status":"pending"}}}`,
	"no-navigation": `{"success":true,"result":{"clicked":true,"navigated":false}}`,
}

func runClickExitChild(result, flag string) {
	m := newMockServer()
	defer m.close()
	m.setResponse(http.MethodPost, "/action", http.StatusOK, clickExitResults[result])
	cmd := newActionCmd()
	cmd.Flags().Bool("json", false, "")
	if flag != "" {
		_ = cmd.Flags().Set(flag, "true")
	}
	Action(apiclient.Conn{Client: m.server.Client(), Base: m.base()}, "click", "e11", cmd)
	os.Exit(0)
}

func TestClickExitCodeFollowsSubmitPostState(t *testing.T) {
	if mode := os.Getenv(clickExitChildEnv); mode != "" {
		result, flag, _ := strings.Cut(mode, ",")
		runClickExitChild(result, flag)
		return
	}

	for _, tc := range []struct {
		result, flag string
		wantExit     int
	}{
		{"blocked", "", cli.ExitRuntime},
		{"blocked", "wait-nav", cli.ExitRuntime},
		{"blocked", "submit", cli.ExitRuntime},
		{"blocked", "json", cli.ExitRuntime},
		{"succeeded", "", 0},
		{"succeeded", "json", 0},
		{"pending", "submit", 0},
		{"no-navigation", "wait-nav", 0},
	} {
		t.Run(tc.result+"/"+tc.flag, func(t *testing.T) {
			child := exec.Command(os.Args[0], "-test.run=TestClickExitCodeFollowsSubmitPostState", "-test.timeout=60s") // #nosec G204 -- re-executes this test binary.
			child.Env = append(os.Environ(), clickExitChildEnv+"="+tc.result+","+tc.flag)
			out, err := child.CombinedOutput()

			exitCode := 0
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				exitCode = exitErr.ExitCode()
			} else if err != nil {
				t.Fatalf("child err = %v (output: %s)", err, out)
			}
			if exitCode != tc.wantExit {
				t.Fatalf("exit = %d, want %d (output: %s)", exitCode, tc.wantExit, out)
			}
		})
	}
}

func TestFillPrintsTheServersWarningForAReactControlledInput(t *testing.T) {
	const warned = `{"success":true,"result":{"filled":true,"warning":"fill may not have been picked up by the page (e.g. React controlled input); try 'type' instead"}}`
	m := newMockServer()
	defer m.close()
	m.setResponse("POST", "/action", 200, warned)
	conn := apiclient.Conn{Client: m.server.Client(), Base: m.base()}
	var stdout string
	stderr := captureStderr(t, func() {
		stdout = captureStdout(t, func() { ActionSimple(conn, "fill", []string{"e5", "hello"}, newSimpleCmd()) })
	})
	if stdout != "OK\n" {
		t.Errorf("stdout = %q, want OK", stdout)
	}
	if !strings.Contains(stderr, "try 'type' instead") {
		t.Errorf("stderr = %q; the server's warning was dropped", stderr)
	}
}

func TestPrintActionResultWarnsOnEveryResultShape(t *testing.T) {
	const warning = "action may not have taken effect"
	tests := []struct {
		kind   string
		result map[string]any
	}{
		{"click", map[string]any{"clicked": true, "navigated": true, "url": "https://example.com/next"}},
		{"click", map[string]any{"clicked": true, "navigated": false}},
		{"click", map[string]any{"clicked": true, "switchedToTab": "tab-2"}},
		{"scroll", map[string]any{"scrolled": false, "scrollX": float64(0), "scrollY": float64(0)}},
		{"type", map[string]any{"typed": true}},
		{"click", map[string]any{"clicked": true, bridge.OpenedDialogResultKey: map[string]any{"dialogType": "alert", "dialogMessage": "hi"}}},
		{"submit", map[string]any{"submitted": true, "postState": map[string]any{"status": "succeeded", "signal": "navigation"}}},
	}
	for _, tt := range tests {
		tt.result["warning"] = warning
		stderr := captureStderr(t, func() {
			captureStdout(t, func() { printActionResult(tt.kind, map[string]any{"success": true, "result": tt.result}) })
		})
		if !strings.Contains(stderr, warning) {
			t.Errorf("%s %v: stderr = %q; the warning was dropped", tt.kind, tt.result, stderr)
		}
	}
}
