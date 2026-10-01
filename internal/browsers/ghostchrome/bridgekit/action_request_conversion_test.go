package bridgekit

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/browsers/ghostchrome"
)

// recordingBridgeAPI captures the bridge.ActionRequest that reaches Chrome. It is the only
// place the crossed request can be read: everything before it is the adapter's own copy.
type recordingBridgeAPI struct {
	bridge.BridgeAPI // nil — panics on anything this test does not drive
	got              bridge.ActionRequest
	calls            int
}

func (r *recordingBridgeAPI) ExecuteAction(_ context.Context, _ string, req bridge.ActionRequest) (map[string]any, error) {
	r.got = req
	r.calls++
	return map[string]any{"filled": true}, nil
}

// newProxyWithoutFallback builds the route the production caller currently hides: no static
// browser, so nothing is served statically, and no fallback closure, so the proxy converts
// the request back itself instead of replaying the original. That branch is unreachable at
// HEAD only because the single caller passes a closure; the signature invites a nil, and the
// closure reads as redundant to anyone tidying up.
func newProxyWithoutFallback(api *recordingBridgeAPI) *ghostchrome.BridgeProxy {
	return ghostchrome.NewBridgeProxy(&chromeActionAdapter{BridgeAPI: api}, nil, func() error { return nil })
}

// An explicit empty text is a request to CLEAR the field, and the bridge refuses a fill that
// carries no text at all. Both facts are only distinguishable by the presence bit, so a
// conversion that drops it turns a documented operation into a refusal.
func TestAnExplicitClearSurvivesTheAdapterWithNoFallbackClosure(t *testing.T) {
	api := &recordingBridgeAPI{}
	proxy := newProxyWithoutFallback(api)

	_, err := proxy.ExecuteAction(context.Background(), ghostchrome.ActionFill, ghostchrome.ActionRequest{
		TabID:   "tab-1",
		Kind:    ghostchrome.ActionFill,
		Ref:     "e5",
		Text:    "",
		HasText: true,
	}, nil)
	if err != nil {
		t.Fatalf("ExecuteAction: %v", err)
	}
	if api.calls != 1 {
		t.Fatalf("Chrome saw %d requests, want 1; the test is not driving the no-fallback branch", api.calls)
	}
	if !api.got.HasText {
		t.Error("the clear crossed the adapter with HasText false, so the bridge cannot tell it from a fill whose text never arrived")
	}
	if err := bridge.ValidateFillAction(ghostchrome.ActionFill, api.got); err != nil {
		t.Errorf("the crossed request is refused by the bridge: %v", err)
	}
}

// The other half of the pair. A conversion that hardcoded HasText true would pass the test
// above and destroy the refusal this bit exists to make possible.
func TestAFillWhoseTextNeverArrivedIsStillRefusedAfterCrossingTheAdapter(t *testing.T) {
	api := &recordingBridgeAPI{}
	proxy := newProxyWithoutFallback(api)

	_, err := proxy.ExecuteAction(context.Background(), ghostchrome.ActionFill, ghostchrome.ActionRequest{
		TabID: "tab-1",
		Kind:  ghostchrome.ActionFill,
		Ref:   "e5",
	}, nil)
	if err != nil {
		t.Fatalf("ExecuteAction: %v", err)
	}
	if api.got.HasText {
		t.Fatal("a request that supplied no text arrived as supplied")
	}
	if err := bridge.ValidateFillAction(ghostchrome.ActionFill, api.got); err == nil {
		t.Error("the bridge accepts a fill whose text never arrived, so the presence bit has stopped meaning anything")
	}
}

// Both directions: the adapter converts DOWN to the static subset on the way in and the
// proxy converts back UP on the way out, so a bit dropped by either conversion is lost.
//
// It drives the two conversions directly rather than BridgeAdapter.ExecuteAction, because
// that entry point cannot show the round trip: it always passes the fallback closure, which
// replays the ORIGINAL request and hides whatever the down-conversion dropped. That masking
// is the defect this card is about, so a test routed through it would prove nothing.
func TestTheClearSurvivesBothConversions(t *testing.T) {
	original := bridge.ActionRequest{TabID: "tab-1", Kind: ghostchrome.ActionFill, Ref: "e5", HasText: true}

	crossed := bridgeActionRequest(staticActionRequest(original))

	if !crossed.HasText {
		t.Error("the presence bit did not survive bridge -> static -> bridge")
	}
	if err := bridge.ValidateFillAction(ghostchrome.ActionFill, crossed); err != nil {
		t.Errorf("the round-tripped clear is refused by the bridge: %v", err)
	}
}

// The single production caller passes a closure carrying the ORIGINAL request, which is why
// the lossy branch is unreached today. Nothing asserted that, so a caller passing nil — or a
// tidy-up deleting the closure as redundant — silently narrowed every request to the static
// subset. This asserts the property rather than relying on it.
func TestTheFallbackClosureDeliversTheWholeRequest(t *testing.T) {
	api := &recordingBridgeAPI{}
	adapter := &BridgeAdapter{
		BridgeAPI: api,
		proxy:     ghostchrome.NewBridgeProxy(&chromeActionAdapter{BridgeAPI: api}, nil, func() error { return nil }),
	}

	original := bridge.ActionRequest{
		TabID:   "tab-1",
		Kind:    ghostchrome.ActionFill,
		Ref:     "e5",
		Text:    "hello",
		HasText: true,
		NodeID:  42,
		Submit:  true,
		Owner:   "agent-1",
		HasXY:   true,
		X:       10,
		Y:       20,
	}
	if _, err := adapter.ExecuteAction(context.Background(), ghostchrome.ActionFill, original); err != nil {
		t.Fatalf("ExecuteAction: %v", err)
	}
	if !reflect.DeepEqual(api.got, original) {
		t.Errorf("Chrome received %+v, want the original request %+v; the fallback no longer replays what the caller sent, so every field outside the static subset is lost", api.got, original)
	}
}

// notCarriedToTheStaticRequest names every bridge.ActionRequest field the static subset
// deliberately does not carry, with the reason at the entry. It is checked in BOTH
// directions: a field that is neither carried nor listed fails, and a listed field that no
// longer exists fails too.
//
// This is the guard the hand-written literals lacked. HasXY and then HasText were each
// dropped from a five-field copy and nobody noticed until presence started mattering; a new
// field now has to be classified before it can land.
var notCarriedToTheStaticRequest = map[string]string{
	"Selector":                  reasonRefOnly,
	"Key":                       reasonChromeOnly,
	"NodeID":                    reasonChromeOnly,
	"X":                         reasonChromeOnly,
	"Y":                         reasonChromeOnly,
	"HasXY":                     reasonChromeOnly,
	"Button":                    reasonChromeOnly,
	"Mode":                      reasonChromeOnly,
	"FrameW":                    reasonChromeOnly,
	"FrameH":                    reasonChromeOnly,
	"Modifiers":                 reasonChromeOnly,
	"ScrollX":                   reasonChromeOnly,
	"ScrollY":                   reasonChromeOnly,
	"HasScroll":                 reasonChromeOnly,
	"DeltaX":                    reasonChromeOnly,
	"DeltaY":                    reasonChromeOnly,
	"HasDelta":                  reasonChromeOnly,
	"DragX":                     reasonChromeOnly,
	"DragY":                     reasonChromeOnly,
	"ToSelector":                reasonChromeOnly,
	"ToNodeID":                  reasonChromeOnly,
	"ToX":                       reasonChromeOnly,
	"ToY":                       reasonChromeOnly,
	"HasToXY":                   reasonChromeOnly,
	"WaitNav":                   reasonNoLiveRenderer,
	"Fast":                      reasonNoLiveRenderer,
	"Owner":                     reasonHandlerOnly,
	"Submit":                    reasonNoLiveRenderer,
	"DismissBanners":            reasonHandlerOnly,
	"DismissKnownInterstitials": reasonHandlerOnly,
	"Humanize":                  reasonNoLiveRenderer,
	"AutoSwitch":                reasonNoLiveRenderer,
	"DialogAction":              reasonNoLiveRenderer,
	"DialogText":                reasonNoLiveRenderer,
	"Browser":                   reasonRoutingDecided,
	"Vocab":                     reasonHandlerOnly,
	"VocabTab":                  reasonHandlerOnly,
}

const (
	reasonRefOnly        = "the static browser targets by ref only; a selector request routes to Chrome"
	reasonChromeOnly     = "meaningless to the static DOM: it addresses or dispatches through CDP"
	reasonNoLiveRenderer = "the static browser has no live renderer to wait on, navigate or dialog"
	reasonHandlerOnly    = "the handler layer reads it; the bridge and the static path never do"
	reasonRoutingDecided = "already decided upstream — this request has been routed to ghost-chrome"
)

// Every field of ActionRequest is either carried by the conversions or classified above.
// The failure names the field, which is the information a count guard cannot carry: "21
// fields, expected 20" sends the reader hunting, "HasText is not carried" does not.
func TestEveryActionRequestFieldIsCarriedOrClassified(t *testing.T) {
	carried := fieldNames(reflect.TypeFor[ghostchrome.ActionRequest]())
	all := fieldNames(reflect.TypeFor[bridge.ActionRequest]())

	var unclassified []string
	for name := range all {
		if carried[name] || notCarriedToTheStaticRequest[name] != "" {
			continue
		}
		unclassified = append(unclassified, name)
	}
	if len(unclassified) > 0 {
		sort.Strings(unclassified)
		t.Errorf("ActionRequest.%s is neither carried to the static request nor listed as deliberately dropped; add the field to both conversions or classify it with a reason",
			strings.Join(unclassified, ", ActionRequest."))
	}

	for name := range notCarriedToTheStaticRequest {
		if !all[name] {
			t.Errorf("%q is listed as deliberately dropped but ActionRequest has no such field; the entry is stale", name)
		}
		if carried[name] {
			t.Errorf("%q is listed as deliberately dropped and is also carried by the static request; one of the two is wrong", name)
		}
	}
}

// The classification above says which fields cross. This says they actually DO, in both
// directions, by driving the conversions with a value in every field — the check the
// hand-written literals never had, and the one that reds the moment a field is added to the
// static subset and forgotten in a copy.
func TestBothConversionsCarryEveryStaticField(t *testing.T) {
	static := ghostchrome.ActionRequest{}
	fillEveryField(t, reflect.ValueOf(&static).Elem())

	up := bridgeActionRequest(static)
	assertStaticFieldsEqual(t, "static -> bridge", reflect.ValueOf(static), reflect.ValueOf(up))

	down := staticActionRequest(up)
	assertStaticFieldsEqual(t, "bridge -> static", reflect.ValueOf(up), reflect.ValueOf(down))
	if !reflect.DeepEqual(static, down) {
		t.Errorf("round trip = %+v, want %+v", down, static)
	}
}

// fillEveryField sets a distinct non-zero value in each field, so a field the conversion
// forgets arrives as a zero the comparison can see. A shared value would let a copy of the
// WRONG field pass.
func fillEveryField(t *testing.T, v reflect.Value) {
	t.Helper()
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		switch field.Kind() {
		case reflect.String:
			field.SetString(v.Type().Field(i).Name + "-value")
		case reflect.Bool:
			field.SetBool(true)
		case reflect.Int, reflect.Int64:
			field.SetInt(int64(i + 1))
		case reflect.Float64:
			field.SetFloat(float64(i + 1))
		default:
			t.Fatalf("field %s has kind %s, which this test does not know how to populate; extend it rather than skipping the field",
				v.Type().Field(i).Name, field.Kind())
		}
	}
}

// assertStaticFieldsEqual compares the fields of the static subset, whichever side is the
// richer struct: those are exactly the fields both conversions are required to carry, and
// the ones outside it are classified by the table above.
func assertStaticFieldsEqual(t *testing.T, direction string, from, to reflect.Value) {
	t.Helper()
	subset := reflect.TypeFor[ghostchrome.ActionRequest]()
	for field := range subset.Fields() {
		name := field.Name
		source, target := from.FieldByName(name), to.FieldByName(name)
		if !source.IsValid() || !target.IsValid() {
			t.Fatalf("%s: %s is missing from one side of the conversion", direction, name)
		}
		if !reflect.DeepEqual(source.Interface(), target.Interface()) {
			t.Errorf("%s: %s = %v, want %v; the conversion does not copy it", direction, name, target.Interface(), source.Interface())
		}
	}
}

func fieldNames(t reflect.Type) map[string]bool {
	names := map[string]bool{}
	for field := range t.Fields() {
		names[field.Name] = true
	}
	return names
}
