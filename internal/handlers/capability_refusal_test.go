package handlers

import (
	"encoding/json"
	"go/ast"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/routes"
	"github.com/pinchtab/pinchtab/internal/srccensus"
)

// decodeRefusal reads one recorded 403 into its message, code and details. Both
// drives below share it: the endpoint drive, which proves a real handler refuses
// this way, and the writer drive, which sweeps every catalogued capability.
func decodeRefusal(t *testing.T, subject string, w *httptest.ResponseRecorder) (string, string, map[string]any) {
	t.Helper()

	if w.Code != http.StatusForbidden {
		t.Fatalf("%s: status = %d, want 403: %s", subject, w.Code, w.Body.String())
	}
	var resp struct {
		Error   string         `json:"error"`
		Code    string         `json:"code"`
		Details map[string]any `json:"details"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("%s: decode: %v (body=%s)", subject, err, w.Body.String())
	}
	if setting, _ := resp.Details["setting"].(string); setting == "" {
		t.Fatalf("%s: refusal dropped details.setting: %v", subject, resp.Details)
	}
	return resp.Error, resp.Code, resp.Details
}

func capabilityRefusal(t *testing.T, method, path string, call func(*Handlers, http.ResponseWriter, *http.Request)) (string, string, map[string]any) {
	t.Helper()

	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	w := httptest.NewRecorder()
	call(h, w, httptest.NewRequest(method, path, nil))
	return decodeRefusal(t, method+" "+path, w)
}

// remedyFor is the one place the expected remedy is spelled, so every assertion
// below compares against the same contract rather than its own copy. The remedy is
// the mode-neutral config write only: `pinchtab server restart` would destroy a
// bridge, which answers these same gates, so the restart guidance lives in the hint
// instead (see restartHint).
func remedyFor(setting string) string {
	return "pinchtab config set " + setting + " true"
}

// restartHint is the mode-neutral prose the hint carries so a caller applies the
// change with the restart correct for their own mode.
const restartHint = "restart pinchtab"

// /storage is gated by the stateExport capability, so the old wording sent the
// reader looking for a "stateExport endpoint" that does not exist. /cookies is
// the shape whose label already matched, and must still read correctly.
func TestCapabilityRefusalNamesTheCapabilityAndKeepsTheCode(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		call       func(*Handlers, http.ResponseWriter, *http.Request)
		capability string
		setting    string
		wantCode   string
	}{
		{
			name:       "storage is gated by another endpoint's capability",
			method:     "GET",
			path:       "/storage",
			call:       func(h *Handlers, w http.ResponseWriter, r *http.Request) { h.HandleStorage(w, r) },
			capability: "stateExport",
			setting:    "security.allowStateExport",
			wantCode:   "state_export_disabled",
		},
		{
			name:       "cookies label already matched its endpoint",
			method:     "GET",
			path:       "/cookies",
			call:       func(h *Handlers, w http.ResponseWriter, r *http.Request) { h.HandleGetCookies(w, r) },
			capability: "cookies",
			setting:    "security.allowCookies",
			wantCode:   "cookies_disabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message, code, _ := capabilityRefusal(t, tt.method, tt.path, tt.call)

			if code != tt.wantCode {
				t.Fatalf("code = %q, want %q", code, tt.wantCode)
			}
			if strings.Contains(message, tt.capability+" endpoint") {
				t.Fatalf("message calls the capability an endpoint: %q", message)
			}
			if !strings.Contains(message, tt.capability+" capability") {
				t.Fatalf("message does not name the required capability: %q", message)
			}
			if !strings.Contains(message, tt.setting) {
				t.Fatalf("message does not name the setting to change: %q", message)
			}
		})
	}
}

// Every capability refusal must carry the mode-neutral config-write remedy AND a
// hint that names the restart — never `pinchtab server restart` in the executable
// remedy, which would kill a bridge answering this same gate. Writing the setting
// is only half the fix (the security block is read at boot), so the restart the
// caller still needs is stated in the hint as prose they apply for their own mode.
//
// The scope is derived from the route catalogue rather than listed here, so a
// capability added there is covered the day it is added. Recording and clipboard
// are appended because a catalogue census cannot reach them: recording gates
// in-handler on another capability's setting, clipboard has no catalogue entry.
// Those two are exactly where the guidance drifted before.
func TestEveryCapabilityRefusalCarriesTheRunnableRemedy(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)

	type gate struct {
		name    string
		setting string
		write   func(w *httptest.ResponseRecorder)
	}
	var gates []gate

	caps := routes.CapabilityEndpoints()
	if len(caps) == 0 {
		t.Fatal("the route catalogue reports no capability-gated endpoints; this census would pass vacuously")
	}
	for capability := range caps {
		meta, ok := routes.Meta(capability)
		if !ok {
			t.Errorf("capability %q gates endpoints but has no metadata, so its refusal can name no setting", capability)
			continue
		}
		gates = append(gates, gate{
			name:    string(capability),
			setting: meta.Setting,
			write:   func(w *httptest.ResponseRecorder) { h.writeCapabilityDisabled(w, capability) },
		})
	}

	screencast, _ := routes.Meta(routes.CapScreencast)
	gates = append(gates,
		gate{
			name:    "recording",
			setting: screencast.Setting,
			write: func(w *httptest.ResponseRecorder) {
				h.HandleRecordStart(w, httptest.NewRequest(http.MethodPost, "/record/start", nil))
			},
		},
		gate{
			name:    "clipboard",
			setting: clipboardSetting,
			write:   func(w *httptest.ResponseRecorder) { writeClipboardDisabled(w) },
		},
	)

	for _, g := range gates {
		t.Run(g.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			g.write(w)
			_, _, details := decodeRefusal(t, g.name, w)

			if got, _ := details["setting"].(string); got != g.setting {
				t.Errorf("details.setting = %q, want %q", got, g.setting)
			}
			remedy, _ := details["remedy"].(string)
			if remedy == "" {
				t.Fatalf("%s refusal carries no remedy at all, so the caller is told nothing to try: %v", g.name, details)
			}
			if remedy != remedyFor(g.setting) {
				t.Errorf("remedy = %q, want %q; a gate building its own string drifts from every other capability", remedy, remedyFor(g.setting))
			}
			if strings.Contains(remedy, "pinchtab server restart") {
				t.Errorf("%s remedy names `pinchtab server restart`, which destroys a bridge if run verbatim: %q", g.name, remedy)
			}
			hint, _ := details["hint"].(string)
			if !strings.Contains(strings.ToLower(hint), restartHint) {
				t.Errorf("%s refusal hint does not carry the mode-neutral restart guidance: %q", g.name, hint)
			}
		})
	}
}

// /wait is CapNone in the catalogue, so no route lock ever answers for it and the
// in-handler gate on mode "fn" is the only thing refusing it. It used to build its
// own details, carrying the setting but neither the hint nor the restart — and
// writing the setting without restarting is a successful no-op, so a caller who
// worked the config command out of `setting` alone got the identical 403 back.
//
// The two refusals are compared to EACH OTHER rather than to copied literals: same
// capability, same guidance, and only this shape reds if the pair drifts.
func TestWaitFnRefusesExactlyAsEvaluateDoesForTheSameCapability(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux, func() {})

	refuse := func(path, body string) (string, map[string]any) {
		t.Helper()
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		mux.ServeHTTP(w, r)
		_, code, details := decodeRefusal(t, "POST "+path, w)
		return code, details
	}

	waitCode, waitDetails := refuse("/wait", `{"fn":"1==1"}`)
	evalCode, evalDetails := refuse("/evaluate", `{"expression":"1"}`)

	if waitCode != evalCode {
		t.Errorf("code = %q on /wait but %q on /evaluate, for one capability", waitCode, evalCode)
	}
	for _, key := range []string{"setting", "hint", "remedy"} {
		got, _ := waitDetails[key].(string)
		want, _ := evalDetails[key].(string)
		if want == "" {
			t.Fatalf("/evaluate carries no %s, so this comparison would pass vacuously: %v", key, evalDetails)
		}
		if got != want {
			t.Errorf("details.%s = %q on /wait, %q on /evaluate", key, got, want)
		}
	}
	// Writing the setting is only half the fix; the security block is read at boot,
	// so the restart the caller still needs is named in the hint (mode-neutral, since
	// a bridge cannot run `pinchtab server restart`).
	if hint, _ := waitDetails["hint"].(string); !strings.Contains(strings.ToLower(hint), restartHint) {
		t.Errorf("hint = %q, want it to name the restart", hint)
	}
}

// The census above derives its scope from the route catalogue and drives
// writeCapabilityDisabled, so a gate that calls httpx.ErrorCode inline is invisible
// to it — which is exactly how /wait shipped a refusal with no hint and no remedy.
// This keys off the OPERATION instead: any site emitting a _disabled code must hand
// it the shared details builder, whatever route or capability it belongs to.
func TestEveryHandRolledDisabledRefusalUsesTheSharedDetailsBuilder(t *testing.T) {
	fset, files := srccensus.Load(t, ".", handlersSourceFileFloor).Syntax()
	found := 0
	for name, file := range files {
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "ErrorCode" || len(call.Args) < 6 {
				return true
			}
			code, isDisabled := disabledCodeSpelling(call.Args[2])
			if !isDisabled {
				return true
			}
			found++
			if !callsDisabledEndpointDetails(call.Args[5]) {
				t.Errorf("%s: %s builds its own details for %s; pass httpx.DisabledEndpointDetails(setting) instead, or the refusal ships without the hint and the restart",
					fset.Position(call.Pos()), name, code)
			}
			return true
		})
	}

	if found == 0 {
		t.Fatal("found no disabled-code ErrorCode call in either spelling; the shape moved and this census now passes over nothing")
	}
}

// Both spellings of the code argument count. A literal "x_disabled" was the only form when
// this census was written; deriving it from routes.Meta is the house form now, so a
// predicate matching only the literal would discriminate on the form the package had just
// moved away from — and its floor would rest on the last remaining literal, which the next
// conversion removes for the opposite of the reason the floor exists.
func disabledCodeSpelling(arg ast.Expr) (string, bool) {
	switch code := arg.(type) {
	case *ast.BasicLit:
		value := strings.Trim(code.Value, `"`)
		return value, strings.HasSuffix(value, "_disabled")
	case *ast.SelectorExpr:
		return code.Sel.Name, code.Sel.Name == "DisabledCode"
	}
	return "", false
}

// The census above still keys on how the CODE argument is spelled, so a gate whose code is
// a named constant or a local is invisible to it — the one shape neither of its two arms
// matches. This keys on the DEFECT instead and needs no spelling at all: a details map
// literal carrying "setting" is precisely what httpx.DisabledEndpointDetails exists to
// build, and hand-writing it is how a refusal loses the hint and the restart. Its steady
// state is zero matches, so its non-vacuity is the file count, not a found floor.
func TestNoHandlerHandBuildsADetailsMapCarryingTheCapabilitySetting(t *testing.T) {
	fset, files := srccensus.Load(t, ".", handlersSourceFileFloor).Syntax()
	for name, file := range files {
		ast.Inspect(file, func(node ast.Node) bool {
			lit, ok := node.(*ast.CompositeLit)
			if !ok || !hasStringKey(lit, "setting") {
				return true
			}
			t.Errorf("%s: %s hand-builds a details map carrying \"setting\"; call httpx.DisabledEndpointDetails(setting) instead, or the refusal ships without the hint and the restart",
				fset.Position(lit.Pos()), name)
			return true
		})
	}
}

func hasStringKey(lit *ast.CompositeLit, want string) bool {
	for _, element := range lit.Elts {
		kv, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.BasicLit)
		if ok && strings.Trim(key.Value, `"`) == want {
			return true
		}
	}
	return false
}

func callsDisabledEndpointDetails(arg ast.Expr) bool {
	call, ok := arg.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "DisabledEndpointDetails"
}

// The bridge mux registers HandleRecordStart directly, with no route-level lock, so
// this in-handler gate is the only thing refusing /record/start on that surface —
// deleting it would let a disabled capability record. The orchestrator front locks
// the same route and answers screencast_disabled, so the code here has to match it
// or one capability answers two ways depending on which port the client reached.
//
// Driving the mux rather than the handler is the point: a handler-only assertion
// passes whether or not the route is shadowed, which is how the previous code
// pinned a wire contract no client received.
func TestRecordStartRefusesThroughTheMuxWithTheSharedScreencastCode(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux, func() {})

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/record/start", nil))
	message, code, details := decodeRefusal(t, "POST /record/start", w)

	screencast, _ := routes.Meta(routes.CapScreencast)
	if code != screencast.DisabledCode {
		t.Errorf("code = %q, want %q — the code the orchestrator front returns for this same route", code, screencast.DisabledCode)
	}
	if !strings.Contains(message, screencast.Setting) {
		t.Errorf("message = %q, want it to name the setting to change", message)
	}
	if got, _ := details["remedy"].(string); got != remedyFor(screencast.Setting) {
		t.Errorf("remedy = %q, want the shared one", got)
	}
}

// The read and write clipboard gates were byte-identical bodies, which is how one
// of them could have been fixed alone. They now share a writer; this asserts the
// two refusals cannot diverge again.
func TestClipboardReadAndWriteRefusalsAreIdentical(t *testing.T) {
	read, readCode, readDetails := capabilityRefusal(t, http.MethodGet, "/clipboard/read",
		func(h *Handlers, w http.ResponseWriter, r *http.Request) { h.HandleClipboardRead(w, r) })
	write, writeCode, writeDetails := capabilityRefusal(t, http.MethodPost, "/clipboard/write",
		func(h *Handlers, w http.ResponseWriter, r *http.Request) { h.HandleClipboardWrite(w, r) })

	if read != write || readCode != writeCode {
		t.Errorf("clipboard read and write refuse differently:\n read: %q / %q\nwrite: %q / %q", read, readCode, write, writeCode)
	}
	for _, key := range []string{"setting", "hint", "remedy"} {
		if readDetails[key] != writeDetails[key] {
			t.Errorf("clipboard details[%q] differs: read=%v write=%v", key, readDetails[key], writeDetails[key])
		}
	}
	if got, _ := readDetails["remedy"].(string); got != remedyFor(clipboardSetting) {
		t.Errorf("clipboard remedy = %q, want %q", got, remedyFor(clipboardSetting))
	}
}

// Refuse, follow the remedy, retry. The config write and the restart are both
// modelled: SetConfigValue is what `pinchtab config set` performs, and
// ApplyFileConfigToRuntime is what a restart does with the resulting file — the
// running config is built from it at boot and never rebuilt on an edit, which is
// precisely why the hint has to name a restart. Doing only the config write leaves
// the gate shut, which is the loop the caller was stuck in.
func TestFollowingTheRemedyClearsTheRefusal(t *testing.T) {
	cfg := &config.RuntimeConfig{}
	h := New(&mockBridge{}, cfg, nil, nil, nil)

	w := httptest.NewRecorder()
	h.HandleGetCookies(w, httptest.NewRequest(http.MethodGet, "/cookies", nil))
	_, code, details := decodeRefusal(t, "GET /cookies", w)
	if code != "cookies_disabled" {
		t.Fatalf("code = %q, want cookies_disabled", code)
	}
	setting, _ := details["setting"].(string)
	remedy, _ := details["remedy"].(string)
	hint, _ := details["hint"].(string)

	fc := config.FileConfig{}
	if err := config.SetConfigValue(&fc, setting, "true"); err != nil {
		t.Fatalf("the remedy's config write fails: %v (remedy=%q)", err, remedy)
	}
	if !strings.Contains(strings.ToLower(hint), restartHint) {
		t.Fatalf("hint = %q names no restart, so the retry below would model a step the caller was never told to take", hint)
	}
	config.ApplyFileConfigToRuntime(cfg, &fc)

	retry := httptest.NewRecorder()
	h.HandleGetCookies(retry, httptest.NewRequest(http.MethodGet, "/cookies", nil))
	if retry.Code == http.StatusForbidden && strings.Contains(retry.Body.String(), "cookies_disabled") {
		t.Fatalf("after following the remedy the identical refusal came back: %s", retry.Body.String())
	}
}

// The config write ALONE must not clear the gate. If it did, the restart in the
// hint would be noise and this whole card would be wrong — so the negative is
// what makes the test above evidence for naming the restart rather than against.
func TestTheConfigWriteAloneDoesNotClearTheRefusal(t *testing.T) {
	cfg := &config.RuntimeConfig{}
	h := New(&mockBridge{}, cfg, nil, nil, nil)

	fc := config.FileConfig{}
	if err := config.SetConfigValue(&fc, "security.allowCookies", "true"); err != nil {
		t.Fatalf("config set: %v", err)
	}

	w := httptest.NewRecorder()
	h.HandleGetCookies(w, httptest.NewRequest(http.MethodGet, "/cookies", nil))
	if _, code, _ := decodeRefusal(t, "GET /cookies after config write only", w); code != "cookies_disabled" {
		t.Fatalf("code = %q; the running config picked the edit up without a restart, so the remedy should stop naming one", code)
	}
}

// recordRoutes is the whole record family, driven together on purpose: all three are
// CapScreencast in the catalogue, so the orchestrator front locks all three, and the
// asymmetry this pins is that only start once carried the in-handler guard the bridge
// mux relies on. Asserting them as a set is what stops a fourth route arriving guarded
// on one front only.
//
// If this is ever widened into a guard-PRESENCE census over the package, it must follow
// ensureCookiesEnabled and ensureStateExportEnabled as well: those two wrappers delegate
// to writeCapabilityDisabled, so grepping for the owner alone reports the cookies and
// state families — eleven routes — as unguarded when they are not.
var recordRoutes = []struct {
	method string
	path   string
	body   string
}{
	{http.MethodPost, "/record/start", `{}`},
	{http.MethodPost, "/record/stop", `{}`},
	{http.MethodGet, "/record/status", ``},
}

func recordMux(t *testing.T, allowScreencast bool, stateDir string) *http.ServeMux {
	t.Helper()

	h := New(&mockBridge{}, &config.RuntimeConfig{AllowScreencast: allowScreencast, StateDir: stateDir}, nil, nil, nil)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux, func() {})
	return mux
}

func serveRecord(mux *http.ServeMux, method, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	mux.ServeHTTP(w, r)
	return w
}

// The bridge mux registers these three handlers directly, with no route-level lock, so an
// in-handler guard is the only enforcement on that surface — and stop and status had none
// while start refused. Wire change this pins: both move from 200/400 to 403 when
// screencast is disabled.
func TestEveryRecordRouteRefusesWithTheSharedScreencastCodeThroughTheMux(t *testing.T) {
	mux := recordMux(t, false, t.TempDir())
	screencast, _ := routes.Meta(routes.CapScreencast)

	for _, route := range recordRoutes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			w := serveRecord(mux, route.method, route.path, route.body)
			_, code, details := decodeRefusal(t, route.method+" "+route.path, w)

			if code != screencast.DisabledCode {
				t.Errorf("code = %q, want %q — one capability must not answer two ways across its own routes", code, screencast.DisabledCode)
			}
			if got, _ := details["remedy"].(string); got != remedyFor(screencast.Setting) {
				t.Errorf("remedy = %q, want the shared one", got)
			}
		})
	}
}

// Distinct from TestARefusedRecordStopLeavesNoReservedFile, which pins the RELEASE path: a
// stop refused for having no active recording reserves a name and gives it back. This one
// pins that a capability-refused stop never reserves at all.
//
// Stop reserved a real output path BEFORE discovering there was no active recording, so a
// disabled capability drove a create-then-remove in a server-controlled directory. The
// guard has to sit above that work, which only the directory can show — a 403 alone is
// also produced by a guard placed after the reservation.
func TestAStopRefusedByTheCapabilityGuardNeverReservesAFile(t *testing.T) {
	stateDir := t.TempDir()

	if w := serveRecord(recordMux(t, false, stateDir), http.MethodPost, "/record/stop", `{}`); w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: %s", w.Code, w.Body.String())
	}

	entries, err := os.ReadDir(filepath.Join(stateDir, "recordings"))
	if err == nil && len(entries) > 0 {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Errorf("a refused stop left %v in the recordings directory; the guard must sit above recordingsOutputPath, not after it", names)
	}
}

// The positive control, without which a guard that refuses everything would pass every
// assertion above. With screencast enabled none of the three may answer with the
// capability refusal — what they answer instead is browser/state dependent and not this
// test's business.
func TestRecordRoutesAreNotRefusedWhenScreencastIsEnabled(t *testing.T) {
	mux := recordMux(t, true, t.TempDir())
	screencast, _ := routes.Meta(routes.CapScreencast)

	for _, route := range recordRoutes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			w := serveRecord(mux, route.method, route.path, route.body)
			if strings.Contains(w.Body.String(), screencast.DisabledCode) {
				t.Errorf("status = %d body = %s; screencast is enabled, so the capability guard must not fire", w.Code, w.Body.String())
			}
		})
	}
}
