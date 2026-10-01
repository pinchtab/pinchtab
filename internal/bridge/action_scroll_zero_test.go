package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/token"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/srccensus"
)

func decodeScrollRequest(t *testing.T, body string) ActionRequest {
	t.Helper()

	var req ActionRequest
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	return req
}

// An absent delta and an explicit zero are different requests, and only the JSON key
// presence can tell them apart — both leave ScrollX and ScrollY at 0.
func TestHasScrollSeparatesAnAbsentDeltaFromAnExplicitZero(t *testing.T) {
	for _, tc := range []struct {
		body string
		want bool
	}{
		{`{"kind":"scroll"}`, false},
		{`{"kind":"scroll","selector":"#footer"}`, false},
		{`{"kind":"scroll","scrollY":0}`, true},
		{`{"kind":"scroll","scrollX":0}`, true},
		{`{"kind":"scroll","scrollY":800}`, true},
	} {
		if got := decodeScrollRequest(t, tc.body).HasScroll; got != tc.want {
			t.Errorf("%s: HasScroll = %v, want %v", tc.body, got, tc.want)
		}
	}
}

// presenceFlagOf names, for each JSON key whose meaning is inferred from its presence, the
// flag that carries that meaning once the key is gone. The census below derives the key set
// from the hasJSONKey call sites, so a new presence-inferred field fails here until it is
// listed and round-tripped rather than being silently uncovered.
var presenceFlagOf = map[string]string{
	"x":       "HasXY",
	"y":       "HasXY",
	"toX":     "HasToXY",
	"toY":     "HasToXY",
	"scrollX": "HasScroll",
	"scrollY": "HasScroll",
	"text":    "HasText",
	"value":   "HasText",
	"deltaX":  "HasDelta",
	"deltaY":  "HasDelta",
}

func presenceInferredKeys(t *testing.T) []string {
	t.Helper()

	_, files := srccensus.Load(t, ".", 20).Syntax()
	var keys []string
	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			fn, ok := call.Fun.(*ast.Ident)
			if !ok || fn.Name != "hasJSONKey" || len(call.Args) != 2 {
				return true
			}
			lit, ok := call.Args[1].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				t.Fatalf("hasJSONKey is called with a computed key, so the census cannot enumerate it")
			}
			key, uErr := strconv.Unquote(lit.Value)
			if uErr != nil {
				t.Fatalf("unquote %s: %v", lit.Value, uErr)
			}
			keys = append(keys, key)
			return true
		})
	}
	if len(keys) == 0 {
		t.Fatal("no hasJSONKey call sites found; the census is reading the wrong package")
	}
	return keys
}

func jsonFieldByName(t *testing.T, name string) reflect.StructField {
	t.Helper()

	for field := range reflect.TypeFor[ActionRequest]().Fields() {
		if jsonTagName(field) == name {
			return field
		}
	}
	t.Fatalf("no ActionRequest field is marshaled as %q", name)
	return reflect.StructField{}
}

func jsonTagName(field reflect.StructField) string {
	name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
	return name
}

func omitsEmpty(field reflect.StructField) bool {
	_, opts, _ := strings.Cut(field.Tag.Get("json"), ",")
	return slices.Contains(strings.Split(opts, ","), "omitempty")
}

func zeroJSONLiteral(field reflect.StructField) string {
	if field.Type.Kind() == reflect.String {
		return `""`
	}
	return "0"
}

func roundTrip(t *testing.T, req ActionRequest) ActionRequest {
	t.Helper()

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return decodeScrollRequest(t, string(data))
}

func presenceFlag(t *testing.T, req ActionRequest, name string) bool {
	t.Helper()

	value := reflect.ValueOf(req).FieldByName(name)
	if !value.IsValid() {
		t.Fatalf("ActionRequest has no field %s", name)
	}
	return value.Bool()
}

// The absent-versus-explicit-zero rule is inferred from JSON key presence, which survives a
// round trip only if the payload field omits its zero AND the flag that records the explicit
// case is itself marshaled. Marshal-then-decode is not on any path today; this closes the
// hazard by construction so that a proxy hop, replay or request log cannot reopen it.
func TestPresenceInferredFieldsSurviveAMarshalRoundTrip(t *testing.T) {
	covered := map[string]bool{}

	for _, key := range presenceInferredKeys(t) {
		flagName, ok := presenceFlagOf[key]
		if !ok {
			t.Fatalf("%q infers its meaning from key presence but no round trip covers it", key)
		}
		covered[key] = true

		field := jsonFieldByName(t, key)
		if !omitsEmpty(field) {
			t.Errorf("%s lacks omitempty, so a re-marshal of an absent %s re-introduces it as an explicit zero", field.Name, key)
		}
		flag, ok := reflect.TypeFor[ActionRequest]().FieldByName(flagName)
		if !ok {
			t.Fatalf("ActionRequest has no field %s", flagName)
		}
		if name := jsonTagName(flag); name == "" || name == "-" {
			t.Errorf("%s is not marshaled, so an explicit zero cannot survive the omission of %s", flagName, key)
		}

		absent := roundTrip(t, decodeScrollRequest(t, `{"kind":"scroll"}`))
		if presenceFlag(t, absent, flagName) {
			t.Errorf("%s: a request with no %s round-tripped into %s=true, so the default was refused as an explicit zero", key, key, flagName)
		}

		body := fmt.Sprintf(`{"kind":"scroll","%s":%s}`, key, zeroJSONLiteral(field))
		explicit := roundTrip(t, decodeScrollRequest(t, body))
		if !presenceFlag(t, explicit, flagName) {
			t.Errorf("%s: an explicit zero round-tripped into %s=false, so it decayed into the default step", body, flagName)
		}
	}

	for key := range presenceFlagOf {
		if !covered[key] {
			t.Errorf("%q is listed as presence-inferred but no hasJSONKey call site reads it", key)
		}
	}
}

// The defect this pins: scrollX==0 && scrollY==0 was answered with a default 120px step
// whatever the caller meant, so an explicit zero scrolled DOWN. A caller computing a
// remaining distance reaches zero exactly when it wants no movement, so that is the wrong
// direction reported as success. The refusal must not fire for an absent delta, which has
// always meant one step.
func TestScrollRefusesAnExplicitZeroDeltaButKeepsTheDefaultWhenAbsent(t *testing.T) {
	b := &Bridge{}

	_, err := b.actionScroll(context.Background(), decodeScrollRequest(t, `{"kind":"scroll","scrollY":0}`))
	if err == nil {
		t.Fatal("an explicit zero delta was accepted, so it scrolled a default step in a direction nobody asked for")
	}
	if !strings.Contains(err.Error(), "zero delta") {
		t.Errorf("error = %v, want it to name the zero delta", err)
	}
	// Scroll and wheel share one resolver, so each must still name its OWN spelling:
	// telling a scroll caller to pass deltaX names a field its surface does not accept.
	if !strings.Contains(err.Error(), "scrollX/scrollY") {
		t.Errorf("error = %v, want it to name the spelling scroll accepts", err)
	}

	// An absent delta must reach the default step. Resolving the viewport needs a browser
	// this test does not have, so the assertion is that it got PAST the zero-delta branch
	// and failed later — a refusal here would mean the default step was lost.
	_, err = b.actionScroll(context.Background(), decodeScrollRequest(t, `{"kind":"scroll"}`))
	if err != nil && strings.Contains(err.Error(), "zero delta") {
		t.Errorf("a bare scroll was refused as a zero delta: %v", err)
	}
}

// mouse-wheel is the sibling spelling of the same question, and it reaches further: the MCP
// scroll tool routes to wheel semantics whenever deltaX/deltaY or a coordinate is given, so
// an agent passing a computed deltaY of 0 lands here rather than on actionScroll. Scroll
// refusing a zero while wheel silently scrolled a notch would be one rule with two answers.
func TestWheelDeltaRefusesAnExplicitZeroInEitherSpelling(t *testing.T) {
	for _, tc := range []struct {
		body     string
		wantErr  bool
		wantX    int
		wantY    int
		whyNotOK string
	}{
		{body: `{"kind":"mouse-wheel"}`, wantY: 120, whyNotOK: "a bare wheel has always meant one notch down"},
		{body: `{"kind":"mouse-wheel","deltaY":0}`, wantErr: true},
		{body: `{"kind":"mouse-wheel","deltaX":0}`, wantErr: true},
		{body: `{"kind":"mouse-wheel","deltaX":0,"deltaY":0}`, wantErr: true},
		{body: `{"kind":"mouse-wheel","scrollY":0}`, wantErr: true, whyNotOK: "the legacy spelling asks the same question"},
		{body: `{"kind":"mouse-wheel","deltaX":0,"deltaY":500}`, wantX: 0, wantY: 500, whyNotOK: "a zero on one axis is a real scroll on the other"},
		{body: `{"kind":"mouse-wheel","deltaY":-300}`, wantY: -300},
		{body: `{"kind":"mouse-wheel","deltaY":0,"scrollY":300}`, wantY: 300, whyNotOK: "an explicit zero delta still carries a non-zero legacy delta"},
	} {
		gotX, gotY, err := wheelDelta(decodeScrollRequest(t, tc.body))

		if tc.wantErr {
			if err == nil {
				t.Errorf("%s: accepted, resolving to (%d,%d) — a zero delta scrolls a default notch nobody asked for", tc.body, gotX, gotY)
				continue
			}
			if !strings.Contains(err.Error(), "deltaX/deltaY") {
				t.Errorf("%s: error = %v, want it to name the spelling wheel accepts", tc.body, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: refused (%v); %s", tc.body, err, tc.whyNotOK)
			continue
		}
		if gotX != tc.wantX || gotY != tc.wantY {
			t.Errorf("%s: delta = (%d,%d), want (%d,%d); %s", tc.body, gotX, gotY, tc.wantX, tc.wantY, tc.whyNotOK)
		}
	}
}

// The two scrolling kinds accept each other's spelling, and they must accept it the same
// way: the wheel already fell back to scrollX/scrollY, while a scroll discarded a supplied
// deltaX/deltaY and substituted the notch — a caller was answered success for a 120px move
// it never asked for, on every surface, with no error to learn from. Every row is asserted
// for BOTH kinds so a fix to one cannot leave the other behind.
func TestBothScrollKindsAcceptEitherSpelling(t *testing.T) {
	const (
		notch   = defaultScrollNotch
		refused = -1
	)

	for _, tc := range []struct {
		name        string
		req         ActionRequest
		wantScrollY int
		wantWheelY  int
	}{
		{
			name:        "own spelling",
			req:         ActionRequest{ScrollY: 400, HasScroll: true},
			wantScrollY: 400,
			wantWheelY:  400,
		},
		{
			name:        "the other spelling",
			req:         ActionRequest{DeltaY: 400, HasDelta: true},
			wantScrollY: 400,
			wantWheelY:  400,
		},
		{
			name:        "an explicit zero in the scroll spelling",
			req:         ActionRequest{ScrollY: 0, HasScroll: true},
			wantScrollY: refused,
			wantWheelY:  refused,
		},
		{
			name:        "an explicit zero in the wheel spelling",
			req:         ActionRequest{DeltaY: 0, HasDelta: true},
			wantScrollY: refused,
			wantWheelY:  refused,
		},
		{
			name:        "no delta at all keeps the notch",
			req:         ActionRequest{},
			wantScrollY: notch,
			wantWheelY:  notch,
		},
		{
			name:        "a horizontal delta in the other spelling",
			req:         ActionRequest{DeltaX: -250, HasDelta: true},
			wantScrollY: 0,
			wantWheelY:  0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sx, sy, sErr := scrollDelta(tc.req)
			assertDeltaY(t, "scroll", tc.wantScrollY, sx, sy, sErr)

			wx, wy, wErr := wheelDelta(tc.req)
			assertDeltaY(t, "mouse-wheel", tc.wantWheelY, wx, wy, wErr)
		})
	}
}

// Tolerating both spellings raises a question neither kind answered before: which one wins
// when both arrive with different non-zero values. The answer is the kind's own spelling,
// and it is pinned here rather than left to fall out of the fallback's argument order.
func TestEachScrollKindPrefersItsOwnSpelling(t *testing.T) {
	req := ActionRequest{ScrollY: 400, DeltaY: 200, HasScroll: true, HasDelta: true}

	if _, y, err := scrollDelta(req); err != nil || y != 400 {
		t.Errorf("scroll resolved to %d (err %v), want the scroll spelling's 400", y, err)
	}
	if _, y, err := wheelDelta(req); err != nil || y != 200 {
		t.Errorf("mouse-wheel resolved to %d (err %v), want the wheel spelling's 200", y, err)
	}
}

// A horizontal-only delta must not be read as "nothing supplied": the fallback triggers on
// both components being zero, so a row that only sets Y cannot see a bug in that condition.
func TestAHorizontalOnlyDeltaIsNotTreatedAsAbsent(t *testing.T) {
	x, y, err := scrollDelta(ActionRequest{DeltaX: -250, HasDelta: true})

	if err != nil {
		t.Fatalf("refused a horizontal scroll: %v", err)
	}
	if x != -250 || y != 0 {
		t.Errorf("resolved to x=%d y=%d, want x=-250 y=0 — the notch was substituted for a supplied horizontal delta", x, y)
	}
}

func assertDeltaY(t *testing.T, kind string, want int, x, y int, err error) {
	t.Helper()

	if want == -1 {
		if err == nil {
			t.Errorf("%s: an explicit zero resolved to x=%d y=%d instead of being refused; a caller is told a scroll happened", kind, x, y)
			return
		}
		if !strings.Contains(err.Error(), "a zero delta is not a scroll") {
			t.Errorf("%s: refusal is %q, want the zero-delta rule's own message", kind, err)
		}
		return
	}
	if err != nil {
		t.Fatalf("%s: resolve failed: %v", kind, err)
	}
	if y != want {
		t.Errorf("%s: resolved deltaY = %d, want %d", kind, y, want)
	}
}

// The defect was never in the zero rule — three cards have now closed that at its owner.
// It was a CALLER dropping the value before the owner saw it, so the guard that matters is
// on the wiring: each kind's action must resolve through its own named helper, and neither
// may reach resolveScrollDelta directly and rebuild the tolerance on the way.
func TestEachScrollActionResolvesThroughItsOwnHelper(t *testing.T) {
	pkg := srccensus.Load(t, ".", 20)

	for _, tc := range []struct{ action, helper string }{
		{"actionScroll", "scrollDelta"},
		{"actionMouseWheel", "wheelDelta"},
	} {
		fn, ok := pkg.Func(tc.action)
		if !ok {
			t.Fatalf("%s not found; this census is reading the wrong package", tc.action)
		}

		found := false
		for _, site := range pkg.CallsAllowingNone(tc.helper) {
			if pkg.Contains(fn, site) {
				found = true
			}
		}
		if !found {
			t.Errorf("%s does not call %s, so it resolves its delta some other way and the spelling tolerance is not shared", tc.action, tc.helper)
		}
		for _, site := range pkg.CallsAllowingNone("resolveScrollDelta") {
			if pkg.Contains(fn, site) {
				t.Errorf("%s calls resolveScrollDelta directly at %s — that bypasses the spelling fallback, which is exactly how one kind came to ignore the other's spelling", tc.action, site)
			}
		}
	}

	if len(pkg.Calls(t, "scrollDeltaFromRequest")) < 2 {
		t.Error("fewer than two callers reach the shared tolerance, so it is no longer shared by both kinds")
	}
}
