package mcp

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/token"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/pinchtab/pinchtab/internal/srccensus"
)

// upstreamRecorder records every request that reaches PinchTab, so a test can
// assert a rejected call never got there — an error result alone cannot tell a
// pre-dispatch rejection from a request that went out and came back failing.
func upstreamRecorder(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	paths := &[]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*paths = append(*paths, r.Method+" "+r.URL.Path)
		resp := map[string]any{"path": r.URL.Path}
		if body, _ := io.ReadAll(r.Body); len(body) > 0 {
			var parsed map[string]any
			if json.Unmarshal(body, &parsed) == nil {
				resp["body"] = parsed
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	return srv, paths
}

// A malformed delta must be reported, not dropped. Dropping it degrades a wheel
// scroll into a bare scroll with no magnitude, because hasDeltaY gates the wheel
// branch.
func TestScrollRejectsAMalformedDeltaWithoutCallingUpstream(t *testing.T) {
	srv, paths := upstreamRecorder(t)

	for _, malformed := range []string{"-300px", "300 pixels", "three hundred"} {
		t.Run(malformed, func(t *testing.T) {
			*paths = nil
			result := callTool(t, "pinchtab_scroll", map[string]any{"deltaY": malformed}, srv)

			if !result.IsError {
				t.Fatalf("deltaY=%q was accepted: %s", malformed, resultText(t, result))
			}
			message := resultText(t, result)
			if !strings.Contains(message, "deltaY") {
				t.Errorf("error %q does not name the argument", message)
			}
			if !strings.Contains(message, malformed) {
				t.Errorf("error %q does not echo the received value", message)
			}
			if len(*paths) != 0 {
				t.Errorf("rejected call still reached upstream: %v", *paths)
			}
		})
	}
}

// The case a caller cannot detect: direction synthesises a magnitude precisely
// because the malformed deltaY was dropped, so the tool scrolls DOWN by 120 when
// asked to scroll UP by 300 — sign inverted, magnitude invented, no error.
func TestScrollRejectsAMalformedDeltaRatherThanLettingDirectionInventOne(t *testing.T) {
	srv, paths := upstreamRecorder(t)

	result := callTool(t, "pinchtab_scroll", map[string]any{
		"deltaY":    "-300px",
		"direction": "down",
	}, srv)

	if !result.IsError {
		body := resultText(t, result)
		if strings.Contains(body, "120") {
			t.Fatalf("direction invented a magnitude for a malformed deltaY: %s", body)
		}
		t.Fatalf("malformed deltaY with direction was accepted: %s", body)
	}
	if len(*paths) != 0 {
		t.Errorf("rejected call still reached upstream: %v", *paths)
	}
}

// withBounds is an opt-out, so a dropped "no" leaves bounds switched on and the
// response looks like the default rather than like the request.
func TestCaptureRejectsAMalformedBoolean(t *testing.T) {
	srv, paths := upstreamRecorder(t)

	result := callTool(t, "pinchtab_capture", map[string]any{"withBounds": "no"}, srv)

	if !result.IsError {
		t.Fatalf(`withBounds="no" was accepted: %s`, resultText(t, result))
	}
	message := resultText(t, result)
	if !strings.Contains(message, "withBounds") {
		t.Errorf("error %q does not name the argument", message)
	}
	if !strings.Contains(message, "no") {
		t.Errorf("error %q does not echo the received value", message)
	}
	if len(*paths) != 0 {
		t.Errorf("rejected call still reached upstream: %v", *paths)
	}
}

// Models emit "" for "not set"; turning that into a failure would be a
// regression, and so would rejecting an argument nobody passed.
func TestAbsentAndEmptyTypedArgumentsAreNotRejected(t *testing.T) {
	for _, tc := range []struct {
		name string
		args map[string]any
	}{
		{name: "absent", args: map[string]any{}},
		{name: "empty string", args: map[string]any{"deltaY": "", "steps": "", "x": ""}},
		{name: "explicit null", args: map[string]any{"deltaY": nil}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateTypedArgs("pinchtab_scroll", tc.args); err != nil {
				t.Errorf("validateTypedArgs(%v) = %v, want nil — not set must not become a failure", tc.args, err)
			}
		})
	}
}

// Every shape the accessors read today must still pass validation, or the fix
// would break working callers rather than malformed ones.
func TestReadableTypedArgumentsPassValidation(t *testing.T) {
	for _, args := range []map[string]any{
		{"deltaY": float64(-300)},
		{"deltaY": "-300"},
		{"deltaY": " -300 "},
		{"deltaY": "-300.5"},
		{"steps": "2", "x": "10", "y": float64(20)},
	} {
		if err := validateTypedArgs("pinchtab_scroll", args); err != nil {
			t.Errorf("validateTypedArgs(%v) = %v, want nil", args, err)
		}
	}
	for _, args := range []map[string]any{
		{"withBounds": true},
		{"withBounds": "true"},
		{"withBounds": "false"},
		{"withBounds": "1"},
	} {
		if err := validateTypedArgs("pinchtab_capture", args); err != nil {
			t.Errorf("validateTypedArgs(%v) = %v, want nil", args, err)
		}
	}
}

// The argument list is derived from the schemas, so a tool gaining a WithNumber
// argument is validated on arrival. This asserts the derivation actually found
// the declared types rather than silently returning an empty map, which would
// make every test above pass vacuously.
func TestTypedArgsAreDerivedFromTheToolSchemas(t *testing.T) {
	types := schemaArgTypesOnce()
	if len(types) == 0 {
		t.Fatal("no tool schemas parsed — validation would be a no-op for every tool")
	}

	declared := 0
	for _, tool := range allTools() {
		raw, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatalf("marshal %s schema: %v", tool.Name, err)
		}
		var schema struct {
			Properties map[string]struct {
				Type string `json:"type"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatalf("unmarshal %s schema: %v", tool.Name, err)
		}
		for name, property := range schema.Properties {
			switch property.Type {
			case "number", "integer", "boolean":
				declared++
				if got := types[tool.Name][name]; got != property.Type {
					t.Errorf("%s.%s typed %q by the schema but %q by the validator", tool.Name, name, property.Type, got)
				}
			default:
				if got, ok := types[tool.Name][name]; ok {
					t.Errorf("%s.%s is %q in the schema but the validator typed it %q", tool.Name, name, property.Type, got)
				}
			}
		}
	}
	if declared == 0 {
		t.Fatal("no numeric or boolean argument found in any schema — this guard is checking nothing")
	}
	t.Logf("validating %d schema-declared numeric/boolean arguments", declared)
}

// A handler with no schema would silently skip validation, so a new tool cannot
// be added to one side only.
func TestEveryHandlerHasASchemaAndEverySchemaAHandler(t *testing.T) {
	handlers := rawHandlerMap(NewClient("http://example.invalid", ""))
	schemas := map[string]struct{}{}
	for _, tool := range allTools() {
		schemas[tool.Name] = struct{}{}
	}

	for name := range handlers {
		if _, ok := schemas[name]; !ok {
			t.Errorf("handler %q has no tool schema, so its arguments are never validated", name)
		}
	}
	for name := range schemas {
		if _, ok := handlers[name]; !ok {
			t.Errorf("tool %q has a schema but no handler", name)
		}
	}
}

// Declaring humanize is only half the fix: it was previously read solely to raise
// the mutual-exclusion error and never forwarded, so humanized input was
// unreachable from MCP even for a caller who guessed the name.
func TestPointerToolsForwardHumanizeToUpstream(t *testing.T) {
	for _, tool := range []string{"pinchtab_click", "pinchtab_hover"} {
		t.Run(tool, func(t *testing.T) {
			srv, _ := upstreamRecorder(t)

			result := callTool(t, tool, map[string]any{"selector": "#b", "humanize": true}, srv)
			if result.IsError {
				t.Fatalf("humanize=true was rejected: %s", resultText(t, result))
			}
			body, _ := resultJSON(t, result)["body"].(map[string]any)
			if got, ok := body["humanize"]; !ok || got != true {
				t.Errorf("outbound body humanize = %v (present: %v), want true — the argument must be usable, not just accepted", got, ok)
			}
		})
	}
}

// An explicit false is an opt-OUT and must travel, because the wire field is a
// per-request override of the instance default rather than a flag.
func TestPointerToolsForwardAnExplicitHumanizeFalse(t *testing.T) {
	srv, _ := upstreamRecorder(t)

	result := callTool(t, "pinchtab_click", map[string]any{"selector": "#b", "humanize": false}, srv)
	body, _ := resultJSON(t, result)["body"].(map[string]any)
	if got, ok := body["humanize"]; !ok || got != false {
		t.Errorf("outbound body humanize = %v (present: %v), want false to travel as an opt-out", got, ok)
	}
}

// Forwarding must not become an unconditional default: a call that omits humanize
// has to leave the instance config in charge.
func TestOmittedHumanizeIsNotForwarded(t *testing.T) {
	for _, tool := range []string{"pinchtab_click", "pinchtab_hover"} {
		t.Run(tool, func(t *testing.T) {
			srv, _ := upstreamRecorder(t)

			result := callTool(t, tool, map[string]any{"selector": "#b"}, srv)
			body, _ := resultJSON(t, result)["body"].(map[string]any)
			if got, ok := body["humanize"]; ok {
				t.Errorf("outbound body carries humanize = %v with none requested; the instance default must stay in charge", got)
			}
		})
	}
}

// The third defect: before humanize was declared, the schema-derived validator
// could not see it, so "yes" was silently dropped and the mutual-exclusion guard
// it feeds was bypassed — the guard fired for true and "true" but not "yes".
// argcheck.go is untouched; the declaration alone brings this under validation.
func TestMalformedHumanizeIsRejectedFromTheDeclarationAlone(t *testing.T) {
	srv, paths := upstreamRecorder(t)

	result := callTool(t, "pinchtab_click", map[string]any{"selector": "#b", "mode": "dom", "humanize": "yes"}, srv)
	if !result.IsError {
		t.Fatalf(`humanize="yes" was accepted: %s`, resultText(t, result))
	}
	message := resultText(t, result)
	if !strings.Contains(message, "humanize") {
		t.Errorf("error %q does not name the argument", message)
	}
	if len(*paths) != 0 {
		t.Errorf("rejected call still reached upstream: %v", *paths)
	}
}

// The mutual-exclusion rule the CLI enforces stays enforced, for the parsed string
// form too, and only on the tool that has both arguments.
func TestModeAndHumanizeRemainMutuallyExclusiveOnClick(t *testing.T) {
	for _, humanize := range []any{true, "true"} {
		srv, _ := upstreamRecorder(t)
		result := callTool(t, "pinchtab_click", map[string]any{"selector": "#b", "mode": "dom", "humanize": humanize}, srv)
		if !result.IsError {
			t.Errorf("mode+humanize=%v was accepted: %s", humanize, resultText(t, result))
		}
	}
}

// Only the pointer tools declare it. A tool without a pointer path must not, or
// the schema would advertise an argument its handler ignores.
func TestOnlyPointerToolsDeclareHumanize(t *testing.T) {
	declaring := map[string]bool{}
	for _, tool := range allTools() {
		if _, ok := typedArgsOf(tool)["humanize"]; ok {
			declaring[tool.Name] = true
		}
	}
	want := map[string]bool{"pinchtab_click": true, "pinchtab_hover": true}
	if !reflect.DeepEqual(declaring, want) {
		t.Errorf("tools declaring humanize = %v, want %v (the MCP members of the CLI addPointerActionFlags set)", declaring, want)
	}
	for kind := range humanizeAction {
		if !declaring["pinchtab_"+kind] {
			t.Errorf("handler forwards humanize for %q but pinchtab_%s does not declare it", kind, kind)
		}
	}
}

// argumentProbe is what this guard sends: the typed arguments to probe together,
// plus fixed-value companions that make them reachable at all.
//
// A probe is a SET rather than a name because some arguments cannot show an effect
// alone — resolveXY needs both x and y, and steps is folded in only under a
// direction, and only by multiplying into deltaY. Companions may be of any type:
// direction is a string, and this guard deliberately never probes strings (the
// sibling census records why — no coercion, so nothing is silently dropped), so a
// string can only ever be a companion. A combination-only argument sent as a
// singleton cannot produce an effect on any tool, which is a vacuous pass; the
// positive control below is what forces it to be declared here instead.
type argumentProbe struct {
	args       []string
	companions map[string]any
}

func (p argumentProbe) label() string { return strings.Join(p.args, "+") }

// combinationProbes are the probes whose arguments the handler only reads as a set.
// Everything else is derived as a singleton from the tools' own declarations.
//
// THE BOUNDARY, and it is deliberate. Each probe is sent alone or with the companions
// listed here, so the guard cannot see a leak that fires ONLY alongside an argument it
// does not pair with — pixels gated on direction reaches every tool while both halves
// of this test stay green, because the control only asks whether pixels can ever have
// an effect (it can, on scroll, alone) and the sweep never sends direction with it.
// The singleton form of the same leak reds immediately; the mutations recorded on the
// card bracket that difference on both sides.
//
// Enumerating the gap is combinatorial: the escape is per (probe, companion) PAIR, not
// per argument, so it grows with every typed argument added. The cheap alternative —
// one maximal probe per tool with every typed argument set at once — was considered and
// declined, because it does not catch this class either: the companion is frequently a
// STRING (direction here, mode elsewhere), and this guard never probes strings by
// design, since nothing is coerced and so nothing is silently dropped. Extending the
// maximal probe to strings needs a plausible value per string argument, which is a new
// hand-maintained map — the staleness this guard is built to avoid, in a different
// denomination.
//
// THE PRECONDITION is what makes that acceptable rather than ignored. A leak of this
// shape needs a typed argument read BEFORE the kind switch in handleAction that is
// neither declared by every action tool nor gated by a kind set. All three current
// pre-switch reads are covered by one of those, measured rather than assumed:
//
//	x, y    gated by xyAction, whose members are exactly the three action tools that
//	        declare x/y (click, hover, scroll); the other six never reach the read
//	nodeId  ungated, but declared by all nine action tools, so there is no tool it
//	        can leak to
//	tabId   read pre-switch and declared on every action tool, but as a string, which
//	        this guard's alphabet excludes for the reason above
//
// Every other typed argument is read inside its own case, where kind gating prevents a
// cross-tool leak by construction. So a reviewer's trigger is specific: a FOURTH
// pre-switch typed read, with neither mechanism, is what would make this gap reachable.
// Until then it is a hole in front of a shape the file does not contain, and this is
// where the chain of cards over this guard stops — not for lack of a next step, but
// because the next step costs more than the hazard.
var combinationProbes = []argumentProbe{
	{args: []string{"x", "y"}},
	{args: []string{"steps"}, companions: map[string]any{"direction": "down"}},
}

// callOutcome is everything this guard can observe about one tool call. The request
// list is recorded before the body is parsed: snap's effect is a second /snapshot
// GET, which makes the result text two concatenated JSON objects that resultJSON
// cannot parse — so a two-request call keeps its raw text and is compared on that.
type callOutcome struct {
	requests []string
	body     map[string]any
	text     string
}

func observeToolCall(t *testing.T, tool string, args map[string]any) callOutcome {
	t.Helper()
	srv, paths := upstreamRecorder(t)
	result := callTool(t, tool, args, srv)
	if result.IsError {
		t.Fatalf("%s rejected %v outright (%s); this guard reasons about arguments that are ignored, not rejected", tool, args, resultText(t, result))
	}
	outcome := callOutcome{requests: append([]string(nil), *paths...), text: resultText(t, result)}
	if len(outcome.requests) == 1 {
		outcome.body, _ = resultJSON(t, result)["body"].(map[string]any)
	}
	return outcome
}

// describeDifference reports how two outcomes differ, or "" when they do not. This
// is the guard's whole oracle: an argument had an observable effect if ANYTHING the
// caller can see changed. Matching the probe's own value instead — which is what
// this replaced — was blind to a derived effect, and needed hasXY hand-added to the
// condition as the tell.
func describeDifference(baseline, probed callOutcome) string {
	if !reflect.DeepEqual(baseline.requests, probed.requests) {
		return fmt.Sprintf("upstream requests %v -> %v", baseline.requests, probed.requests)
	}
	if baseline.body == nil || probed.body == nil {
		if baseline.text != probed.text {
			return "the response text changed"
		}
		return ""
	}
	var changes []string
	for field, want := range baseline.body {
		got, present := probed.body[field]
		switch {
		case !present:
			changes = append(changes, fmt.Sprintf("%s=%v dropped", field, want))
		case !reflect.DeepEqual(want, got):
			changes = append(changes, fmt.Sprintf("%s %v -> %v", field, want, got))
		}
	}
	for field, got := range probed.body {
		if _, present := baseline.body[field]; !present {
			changes = append(changes, fmt.Sprintf("%s=%v added", field, got))
		}
	}
	sort.Strings(changes)
	return strings.Join(changes, ", ")
}

// The per-tool half the name-level census above cannot see. A typed argument read
// for a kind whose tool does not declare it is undiscoverable AND unvalidated,
// because validateTypedArgs keys its type map per tool — the name being declared
// somewhere else does not help the tool being called. handleAction reads its
// arguments before switching on kind, so this is where that goes wrong.
//
// Behavioural rather than structural: the positive control observes reachability as a
// BASELINE DIFF on a tool that declares the probe, and the sweep then requires every
// tool that does NOT declare it to refuse the probe by name before upstream is called.
// The structural half — which keys each handler reads — is
// TestEveryArgumentAHandlerReadsIsDeclaredOnItsTool.
func TestNoActionToolIsSentATypedArgumentItDoesNotDeclare(t *testing.T) {
	probes := append([]argumentProbe(nil), combinationProbes...)
	grouped := map[string]bool{}
	for _, probe := range probes {
		for _, name := range probe.args {
			grouped[name] = true
		}
	}
	for _, tc := range actionToolTargets {
		for name := range schemaArgTypesOnce()[tc.tool] {
			if !grouped[name] {
				probes = append(probes, argumentProbe{args: []string{name}})
				grouped[name] = true
			}
		}
	}
	sort.Slice(probes, func(i, j int) bool { return probes[i].label() < probes[j].label() })

	// The declared type of each name, taken from whichever action tool declares it,
	// so the probe value is one the accessor would actually read.
	typeOf := map[string]string{}
	for _, tc := range actionToolTargets {
		for name, kind := range schemaArgTypesOnce()[tc.tool] {
			typeOf[name] = kind
		}
	}

	const sentinel = 424242.0
	probeArgs := func(tool string, probe argumentProbe, withProbe bool) map[string]any {
		extra := map[string]any{}
		for name, value := range probe.companions {
			if _, declared := schemaPropertiesOnce()[tool][name]; declared {
				extra[name] = value
			}
		}
		if withProbe {
			for _, name := range probe.args {
				if typeOf[name] == "boolean" {
					extra[name] = true
					continue
				}
				extra[name] = sentinel
			}
		}
		return targetedActionArgs(tool, extra)
	}

	// A diff oracle is only as trustworthy as the calls it compares. If anything in
	// an outcome varied between two identical calls, every future card would inherit
	// a red here — so this fails loudly and names the fields rather than normalising
	// them away, and an exclusion has to be added deliberately.
	for _, tc := range actionToolTargets {
		for _, probe := range probes {
			first := observeToolCall(t, tc.tool, probeArgs(tc.tool, probe, false))
			second := observeToolCall(t, tc.tool, probeArgs(tc.tool, probe, false))
			if diff := describeDifference(first, second); diff != "" {
				t.Fatalf("%s is not stable across two identical calls (%s baseline): %s — the diff oracle below would read this as an effect, so exclude the varying field explicitly before trusting it",
					tc.tool, probe.label(), diff)
			}
		}
	}

	// The positive control, and the half that keeps the probe list self-maintaining:
	// every probe must be demonstrably capable of an effect on a tool that DOES
	// declare it. A combination-only argument fails this as a singleton, which is
	// what forces it into combinationProbes with the companions it needs instead of
	// sitting in the undeclared sweep proving nothing.
	for _, probe := range probes {
		declaring := ""
		for _, tc := range actionToolTargets {
			declared := schemaArgTypesOnce()[tc.tool]
			all := true
			for _, name := range probe.args {
				if _, ok := declared[name]; !ok {
					all = false
				}
			}
			if all {
				declaring = tc.tool
				break
			}
		}
		if declaring == "" {
			t.Errorf("no action tool declares %s, so the sweep below can never distinguish reachable from ignored for it", probe.label())
			continue
		}
		baseline := observeToolCall(t, declaring, probeArgs(declaring, probe, false))
		probed := observeToolCall(t, declaring, probeArgs(declaring, probe, true))
		if diff := describeDifference(baseline, probed); diff == "" {
			t.Errorf("%s has no observable effect on %s, which DECLARES it — so its rows in the sweep below prove nothing. Give it the companions that make it reachable (see combinationProbes) rather than leaving it a singleton.",
				probe.label(), declaring)
		}
	}

	checked := 0
	for _, tc := range actionToolTargets {
		declared := schemaArgTypesOnce()[tc.tool]
		for _, probe := range probes {
			kinds := map[string]int{}
			for _, name := range probe.args {
				kinds[declared[name]]++
			}
			if len(kinds) != 1 {
				t.Errorf("%s declares %s inconsistently (%v); the group must be all-or-nothing", tc.tool, probe.label(), kinds)
				continue
			}
			if _, isDeclared := declared[probe.args[0]]; isDeclared {
				continue
			}
			checked++

			srv, paths := upstreamRecorder(t)
			result := callTool(t, tc.tool, probeArgs(tc.tool, probe, true), srv)
			message := resultText(t, result)
			if !result.IsError {
				t.Errorf("%s: %s is undeclared but was accepted (%s); an undeclared argument must be refused, never dropped or acted on", tc.tool, probe.label(), message)
				continue
			}
			for _, name := range probe.args {
				if !strings.Contains(message, "unknown argument "+strconv.Quote(name)) {
					t.Errorf("%s: refusal %q does not name the undeclared %q", tc.tool, message, name)
				}
			}
			if len(*paths) != 0 {
				t.Errorf("%s: a refused %s still reached upstream: %v", tc.tool, probe.label(), *paths)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no undeclared (tool, argument) pair exercised — this guard is checking nothing")
	}
	t.Logf("checked %d undeclared (tool, probe) pairs across %d action tools and %d probes", checked, len(actionToolTargets), len(probes))
}

// A wrapper index used to mean document order for css:/xpath: and semantic rank
// for text:, so nth:1 could resolve earlier in the page than nth:0. The rule is
// one rule now, and a schema that offers first/last/nth without stating it leaves
// an agent to infer the grammar from trial and error. Derived over the schemas:
// any tool that gains a wrapper-accepting selector inherits the requirement.
func TestSelectorSchemasThatOfferWrappersStateWhatAnIndexMeans(t *testing.T) {
	checked := 0
	for _, tool := range allTools() {
		raw, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatalf("marshal %s schema: %v", tool.Name, err)
		}
		var schema struct {
			Properties map[string]struct {
				Description string `json:"description"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(raw, &schema); err != nil {
			t.Fatalf("unmarshal %s schema: %v", tool.Name, err)
		}
		for name, property := range schema.Properties {
			if !strings.Contains(property.Description, "first/last/nth") {
				continue
			}
			checked++
			if !strings.Contains(property.Description, "document order") {
				t.Errorf("%s.%s offers first/last/nth without saying an index follows document order: %q", tool.Name, name, property.Description)
			}
			if !strings.Contains(property.Description, "text:X and first:text:X can differ") {
				t.Errorf("%s.%s does not warn that a bare text: selector ranks rather than indexes: %q", tool.Name, name, property.Description)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no schema offers first/last/nth, so this guard checked nothing")
	}
}

func TestEveryToolRefusesAnUndeclaredArgumentBeforeCallingUpstream(t *testing.T) {
	const undeclared = "notAnArgumentOfAnyTool"
	tools := allTools()
	if len(tools) == 0 {
		t.Fatal("allTools() is empty, so this table checks nothing")
	}
	for _, tool := range tools {
		t.Run(tool.Name, func(t *testing.T) {
			srv, paths := upstreamRecorder(t)
			result := callTool(t, tool.Name, map[string]any{undeclared: "x"}, srv)
			if !result.IsError {
				t.Fatalf("an undeclared argument was accepted: %s", resultText(t, result))
			}
			message := resultText(t, result)
			if !strings.Contains(message, `unknown argument "`+undeclared+`"`) {
				t.Errorf("refusal %q does not name the undeclared key", message)
			}
			if !strings.Contains(message, "declared arguments: "+declaredArgList(schemaPropertiesOnce()[tool.Name])) {
				t.Errorf("refusal %q does not list the tool's declared arguments", message)
			}
			if len(*paths) != 0 {
				t.Errorf("a refused call still reached upstream: %v", *paths)
			}
		})
	}
}

func TestUndeclaredArgumentsNameTheNearestDeclaredOne(t *testing.T) {
	for _, tc := range []struct {
		tool     string
		args     map[string]any
		wantHint string
	}{
		{tool: "pinchtab_snapshot", args: map[string]any{"filter": "interactive"}, wantHint: `unknown argument "filter" (did you mean "interactive": true?)`},
		{tool: "pinchtab_wait", args: map[string]any{"for": "selector", "value": "#nope", "timeoutSeconds": float64(2)}, wantHint: `unknown argument "timeoutSeconds" (did you mean "timeoutMs"?)`},
		{tool: "pinchtab_scrape", args: map[string]any{"url": "https://example.com", "timeout": float64(5)}, wantHint: `unknown argument "timeout" (did you mean "timeoutSeconds"?)`},
		{tool: "pinchtab_click", args: map[string]any{"selecter": "#a"}, wantHint: `unknown argument "selecter" (did you mean "selector"?)`},
		{tool: "pinchtab_navigate", args: map[string]any{"url": "https://example.com", "tabid": "t1"}, wantHint: `unknown argument "tabid" (did you mean "tabId"?)`},
		{tool: "pinchtab_eval", args: map[string]any{"expression": "1", "zzz": true}, wantHint: `unknown argument "zzz";`},
	} {
		t.Run(tc.tool, func(t *testing.T) {
			srv, paths := upstreamRecorder(t)
			result := callTool(t, tc.tool, tc.args, srv)
			if !result.IsError {
				t.Fatalf("%v was accepted: %s", tc.args, resultText(t, result))
			}
			if message := resultText(t, result); !strings.Contains(message, tc.wantHint) {
				t.Errorf("refusal %q, want it to contain %q", message, tc.wantHint)
			}
			if len(*paths) != 0 {
				t.Errorf("a refused call still reached upstream: %v", *paths)
			}
		})
	}
}

func TestWaitSendsTimeoutMsAndItsDeprecatedAliasAsTheWaitBudget(t *testing.T) {
	for _, tc := range []struct {
		name string
		args map[string]any
		want float64
	}{
		{name: "timeoutMs", args: map[string]any{"timeoutMs": float64(1500)}, want: 1500},
		{name: "deprecated timeout", args: map[string]any{"timeout": float64(1200)}, want: 1200},
		{name: "timeoutMs wins over timeout", args: map[string]any{"timeoutMs": float64(1500), "timeout": float64(9000)}, want: 1500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := upstreamRecorder(t)
			args := map[string]any{"for": "selector", "value": "#nope"}
			for name, value := range tc.args {
				args[name] = value
			}
			result := callTool(t, "pinchtab_wait", args, srv)
			if result.IsError {
				t.Fatalf("%v was refused: %s", tc.args, resultText(t, result))
			}
			body, _ := resultJSON(t, result)["body"].(map[string]any)
			if got := body["timeout"]; got != tc.want {
				t.Errorf("posted timeout = %v, want %v (body %v)", got, tc.want, body)
			}
		})
	}
}

func TestWaitDeclaresTimeoutAsADeprecatedAliasOfTimeoutMs(t *testing.T) {
	var wait mcp.Tool
	for _, tool := range allTools() {
		if tool.Name == "pinchtab_wait" {
			wait = tool
		}
	}
	properties := wait.InputSchema.Properties
	canonical, _ := properties["timeoutMs"].(map[string]any)
	if canonical["type"] != "number" {
		t.Fatalf("pinchtab_wait timeoutMs = %v, want a declared number", properties["timeoutMs"])
	}
	alias, _ := properties["timeout"].(map[string]any)
	description, _ := alias["description"].(string)
	if !strings.Contains(description, "deprecated") || !strings.Contains(description, "timeoutMs") {
		t.Errorf("pinchtab_wait timeout description %q must mark it deprecated and point at timeoutMs", description)
	}
}

func TestEverySelectorAliasStillTargetsEveryActionTool(t *testing.T) {
	for _, tc := range actionToolTargets {
		if _, declared := schemaPropertiesOnce()[tc.tool]["selector"]; !declared {
			continue
		}
		for _, key := range selectorArgKeys {
			t.Run(tc.tool+"/"+key, func(t *testing.T) {
				srv, _ := upstreamRecorder(t)
				result := callTool(t, tc.tool, actionArgs(tc.tool, map[string]any{key: "e5"}), srv)
				if result.IsError {
					t.Fatalf("%s {%q: \"e5\"} was refused: %s", tc.tool, key, resultText(t, result))
				}
				body, _ := resultJSON(t, result)["body"].(map[string]any)
				if got := body["selector"]; got != "e5" {
					t.Errorf("posted selector = %v, want e5 (body %v)", got, body)
				}
			})
		}
	}
}

var dynamicArgumentReads = map[string][]string{
	"handleKeyboard": {"key", "text"},
}

type argumentRead struct {
	key   string
	typed bool
	fn    string
}

type argumentReader struct {
	keyIndex int
	variadic bool
	typed    bool
}

type paramBinding struct {
	param string
	value string
}

type argumentReadCensus struct {
	funcs        map[string]*ast.FuncDecl
	readers      map[string]argumentReader
	stringLists  map[string][]string
	boolSets     map[string]map[string]bool
	dynamicSites map[string]int
	memo         map[string][]argumentRead
}

var requestAccessorMethod = regexp.MustCompile(`^(Require|Get)(String|Int|Float|Bool)(Slice)?$`)

func newArgumentReadCensus(t *testing.T) *argumentReadCensus {
	t.Helper()
	census := &argumentReadCensus{
		funcs:        map[string]*ast.FuncDecl{},
		readers:      map[string]argumentReader{},
		stringLists:  map[string][]string{},
		boolSets:     map[string]map[string]bool{},
		dynamicSites: map[string]int{},
		memo:         map[string][]argumentRead{},
	}
	_, files := srccensus.Load(t, ".", 18).Syntax()
	for _, file := range files {
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Recv != nil || d.Body == nil {
					continue
				}
				census.funcs[d.Name.Name] = d
				if reader, ok := readerOf(d); ok {
					census.readers[d.Name.Name] = reader
				}
			case *ast.GenDecl:
				census.recordPackageVars(d)
			}
		}
	}
	if len(census.funcs) == 0 || len(census.readers) == 0 {
		t.Fatal("no package functions or argument readers found, so this census checks nothing")
	}
	return census
}

func readerOf(fn *ast.FuncDecl) (argumentReader, bool) {
	if !takesToolRequest(fn) {
		return argumentReader{}, false
	}
	index := 0
	for _, field := range fn.Type.Params.List {
		for _, name := range field.Names {
			if name.Name == "key" || name.Name == "keys" {
				_, variadic := field.Type.(*ast.Ellipsis)
				return argumentReader{keyIndex: index, variadic: variadic, typed: returnsTypedValue(fn)}, true
			}
			index++
		}
	}
	return argumentReader{}, false
}

func takesToolRequest(fn *ast.FuncDecl) bool {
	for _, field := range fn.Type.Params.List {
		sel, ok := field.Type.(*ast.SelectorExpr)
		if ok && sel.Sel.Name == "CallToolRequest" {
			return true
		}
	}
	return false
}

func returnsTypedValue(fn *ast.FuncDecl) bool {
	if fn.Type.Results == nil || len(fn.Type.Results.List) == 0 {
		return false
	}
	ident, ok := fn.Type.Results.List[0].Type.(*ast.Ident)
	return ok && (ident.Name == "float64" || ident.Name == "int" || ident.Name == "bool")
}

func (c *argumentReadCensus) recordPackageVars(decl *ast.GenDecl) {
	for _, spec := range decl.Specs {
		value, ok := spec.(*ast.ValueSpec)
		if !ok || len(value.Names) != len(value.Values) {
			continue
		}
		for i, name := range value.Names {
			literal, ok := value.Values[i].(*ast.CompositeLit)
			if !ok {
				continue
			}
			switch literal.Type.(type) {
			case *ast.ArrayType:
				if list, ok := stringLiterals(literal.Elts); ok {
					c.stringLists[name.Name] = list
				}
			case *ast.MapType:
				members := map[string]bool{}
				for _, elt := range literal.Elts {
					kv, ok := elt.(*ast.KeyValueExpr)
					if !ok {
						continue
					}
					key, keyOK := stringLiteral(kv.Key)
					flag, flagOK := kv.Value.(*ast.Ident)
					if keyOK && flagOK && flag.Name == "true" {
						members[key] = true
					}
				}
				c.boolSets[name.Name] = members
			}
		}
	}
}

func stringLiteral(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(lit.Value)
	return value, err == nil
}

func stringLiterals(exprs []ast.Expr) ([]string, bool) {
	values := make([]string, 0, len(exprs))
	for _, expr := range exprs {
		value, ok := stringLiteral(expr)
		if !ok {
			return nil, false
		}
		values = append(values, value)
	}
	return values, true
}

func (c *argumentReadCensus) keysAt(call *ast.CallExpr, reader argumentReader, rangeKeys map[string][]string) ([]string, bool) {
	if reader.keyIndex >= len(call.Args) {
		return nil, true
	}
	args := call.Args[reader.keyIndex : reader.keyIndex+1]
	if reader.variadic {
		args = call.Args[reader.keyIndex:]
	}
	if call.Ellipsis.IsValid() && len(args) == 1 {
		ident, ok := args[0].(*ast.Ident)
		if !ok {
			return nil, false
		}
		list, ok := c.stringLists[ident.Name]
		return list, ok
	}
	var keys []string
	for _, arg := range args {
		if literal, ok := stringLiteral(arg); ok {
			keys = append(keys, literal)
			continue
		}
		ident, ok := arg.(*ast.Ident)
		if !ok || rangeKeys[ident.Name] == nil {
			return nil, false
		}
		keys = append(keys, rangeKeys[ident.Name]...)
	}
	return keys, true
}

func literalRangeKeys(body *ast.BlockStmt) map[string][]string {
	keys := map[string][]string{}
	ast.Inspect(body, func(n ast.Node) bool {
		loop, ok := n.(*ast.RangeStmt)
		if !ok {
			return true
		}
		literal, ok := loop.X.(*ast.CompositeLit)
		if !ok {
			return true
		}
		switch literal.Type.(type) {
		case *ast.ArrayType:
			ident, isIdent := loop.Value.(*ast.Ident)
			values, allStrings := stringLiterals(literal.Elts)
			if isIdent && allStrings {
				keys[ident.Name] = values
			}
		case *ast.MapType:
			ident, isIdent := loop.Key.(*ast.Ident)
			if !isIdent {
				return true
			}
			for _, elt := range literal.Elts {
				if kv, ok := elt.(*ast.KeyValueExpr); ok {
					if key, ok := stringLiteral(kv.Key); ok {
						keys[ident.Name] = append(keys[ident.Name], key)
					}
				}
			}
		}
		return true
	})
	return keys
}

func (c *argumentReadCensus) readsOf(fn string, binding *paramBinding, visiting map[string]bool) []argumentRead {
	if binding == nil {
		if cached, ok := c.memo[fn]; ok {
			return cached
		}
	}
	decl, ok := c.funcs[fn]
	if !ok || visiting[fn] {
		return nil
	}
	visiting[fn] = true
	defer delete(visiting, fn)

	_, isReader := c.readers[fn]
	requestMaps := argumentMapIdents(decl.Body)
	rangeKeys := literalRangeKeys(decl.Body)
	var reads []argumentRead
	callees := map[string]bool{}
	record := func(keys []string, resolved, typed bool) {
		if !resolved {
			if !isReader {
				c.dynamicSites[fn]++
			}
			return
		}
		for _, key := range keys {
			reads = append(reads, argumentRead{key: key, typed: typed, fn: fn})
		}
	}

	var visit func(ast.Node) bool
	visit = func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.SwitchStmt:
			if binding == nil || !isIdent(node.Tag, binding.param) {
				return true
			}
			if node.Init != nil {
				ast.Inspect(node.Init, visit)
			}
			for _, clause := range boundCaseClauses(node, binding.value) {
				for _, stmt := range clause.Body {
					ast.Inspect(stmt, visit)
				}
			}
			return false
		case *ast.IfStmt:
			if binding == nil {
				return true
			}
			if holds, known := c.evalCondition(node.Cond, binding); known && !holds {
				if node.Init != nil {
					ast.Inspect(node.Init, visit)
				}
				if node.Else != nil {
					ast.Inspect(node.Else, visit)
				}
				return false
			}
		case *ast.CallExpr:
			switch fun := node.Fun.(type) {
			case *ast.Ident:
				if reader, ok := c.readers[fun.Name]; ok {
					keys, resolved := c.keysAt(node, reader, rangeKeys)
					record(keys, resolved, reader.typed)
				}
				if _, ok := c.funcs[fun.Name]; ok {
					callees[fun.Name] = true
				}
			case *ast.SelectorExpr:
				if match := requestAccessorMethod.FindStringSubmatch(fun.Sel.Name); match != nil && len(node.Args) > 0 {
					key, resolved := stringLiteral(node.Args[0])
					record([]string{key}, resolved, match[2] != "String")
				}
			}
		case *ast.IndexExpr:
			if readsArgumentMap(node.X, requestMaps) {
				key, resolved := stringLiteral(node.Index)
				record([]string{key}, resolved, false)
			}
		}
		return true
	}
	ast.Inspect(decl.Body, visit)

	for callee := range callees {
		reads = append(reads, c.readsOf(callee, nil, visiting)...)
	}
	for _, key := range dynamicArgumentReads[fn] {
		reads = append(reads, argumentRead{key: key, fn: fn})
	}
	if binding == nil {
		c.memo[fn] = reads
	}
	return reads
}

func boundCaseClauses(node *ast.SwitchStmt, value string) []*ast.CaseClause {
	var fallback *ast.CaseClause
	for _, stmt := range node.Body.List {
		clause := stmt.(*ast.CaseClause)
		if clause.List == nil {
			fallback = clause
			continue
		}
		for _, expr := range clause.List {
			if literal, ok := stringLiteral(expr); ok && literal == value {
				return []*ast.CaseClause{clause}
			}
		}
	}
	if fallback != nil {
		return []*ast.CaseClause{fallback}
	}
	return nil
}

func (c *argumentReadCensus) evalCondition(expr ast.Expr, binding *paramBinding) (holds, known bool) {
	switch cond := expr.(type) {
	case *ast.ParenExpr:
		return c.evalCondition(cond.X, binding)
	case *ast.UnaryExpr:
		if cond.Op == token.NOT {
			holds, known := c.evalCondition(cond.X, binding)
			return !holds, known
		}
	case *ast.BinaryExpr:
		left, leftKnown := c.evalCondition(cond.X, binding)
		right, rightKnown := c.evalCondition(cond.Y, binding)
		switch cond.Op {
		case token.LAND:
			if (leftKnown && !left) || (rightKnown && !right) {
				return false, true
			}
			return true, leftKnown && rightKnown
		case token.LOR:
			if (leftKnown && left) || (rightKnown && right) {
				return true, true
			}
			return false, leftKnown && rightKnown
		case token.EQL, token.NEQ:
			literal, ok := stringLiteral(cond.Y)
			if !ok || !isIdent(cond.X, binding.param) {
				return false, false
			}
			return (literal == binding.value) == (cond.Op == token.EQL), true
		}
	case *ast.IndexExpr:
		set, ok := cond.X.(*ast.Ident)
		if !ok || !isIdent(cond.Index, binding.param) {
			return false, false
		}
		members, ok := c.boolSets[set.Name]
		if !ok {
			return false, false
		}
		return members[binding.value], true
	}
	return false, false
}

func argumentMapIdents(body *ast.BlockStmt) map[string]bool {
	idents := map[string]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != len(assign.Rhs) {
			return true
		}
		for i, rhs := range assign.Rhs {
			if isGetArgumentsCall(rhs) {
				if ident, ok := assign.Lhs[i].(*ast.Ident); ok {
					idents[ident.Name] = true
				}
			}
		}
		return true
	})
	return idents
}

func isGetArgumentsCall(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "GetArguments"
}

func readsArgumentMap(expr ast.Expr, requestMaps map[string]bool) bool {
	if isGetArgumentsCall(expr) {
		return true
	}
	ident, ok := expr.(*ast.Ident)
	return ok && requestMaps[ident.Name]
}

func toolRegistrations(t *testing.T) map[string]*ast.CallExpr {
	t.Helper()
	pkg := srccensus.Load(t, ".", 18)
	var registry *ast.FuncDecl
	_, files := pkg.Syntax()
	for _, file := range files {
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "rawHandlerMap" && fn.Body != nil {
				registry = fn
			}
		}
	}
	if registry == nil {
		t.Fatalf("no rawHandlerMap function found in %s, so this census has no tool registrations to read", pkg.Dir())
	}
	registrations := map[string]*ast.CallExpr{}
	ast.Inspect(registry.Body, func(n ast.Node) bool {
		kv, ok := n.(*ast.KeyValueExpr)
		if !ok {
			return true
		}
		name, nameOK := stringLiteral(kv.Key)
		call, callOK := kv.Value.(*ast.CallExpr)
		if nameOK && callOK {
			registrations[name] = call
		}
		return true
	})
	if len(registrations) == 0 {
		t.Fatal("found no tool registrations in rawHandlerMap, so this census checks nothing")
	}
	return registrations
}

func (c *argumentReadCensus) bindingFor(call *ast.CallExpr) (string, *paramBinding) {
	handler, ok := call.Fun.(*ast.Ident)
	if !ok {
		return "", nil
	}
	decl, ok := c.funcs[handler.Name]
	if !ok {
		return handler.Name, nil
	}
	index := 0
	for _, field := range decl.Type.Params.List {
		for _, name := range field.Names {
			if index < len(call.Args) {
				if value, ok := stringLiteral(call.Args[index]); ok {
					return handler.Name, &paramBinding{param: name.Name, value: value}
				}
			}
			index++
		}
	}
	return handler.Name, nil
}

func TestEveryArgumentAHandlerReadsIsDeclaredOnItsTool(t *testing.T) {
	census := newArgumentReadCensus(t)
	registrations := toolRegistrations(t)
	properties := schemaPropertiesOnce()

	total := 0
	for tool, call := range registrations {
		handler, binding := census.bindingFor(call)
		reads := census.readsOf(handler, binding, map[string]bool{})
		if len(reads) == 0 && len(properties[tool]) > 0 {
			t.Errorf("%s declares %d arguments but the census found no read in %s; the walk no longer follows how this handler reads arguments", tool, len(properties[tool]), handler)
		}
		for _, read := range reads {
			total++
			kind, declared := properties[tool][read.key]
			if !declared {
				t.Errorf("%s reads %q (in %s) but the tool does not declare it, so the argument is refused before the handler can see it and is undiscoverable in tools/list", tool, read.key, read.fn)
				continue
			}
			if read.typed && kind != "number" && kind != "integer" && kind != "boolean" {
				t.Errorf("%s reads %q (in %s) with a typed accessor but declares it as %q, so validateTypedArgs cannot reject a malformed value", tool, read.key, read.fn, kind)
			}
		}
	}
	if total == 0 {
		t.Fatal("no argument read found in any handler, so this census checks nothing")
	}

	for fn, count := range census.dynamicSites {
		if _, recorded := dynamicArgumentReads[fn]; !recorded {
			t.Errorf("%s reads %d argument(s) under a name the census cannot resolve; use a literal key or record the names it can take in dynamicArgumentReads", fn, count)
		}
	}
	for fn := range dynamicArgumentReads {
		if census.dynamicSites[fn] == 0 {
			t.Errorf("dynamicArgumentReads records %s, which no longer reads an argument under a computed name; drop the entry", fn)
		}
	}
	t.Logf("checked %d argument reads across %d tools", total, len(registrations))
}

func TestTheReadCensusAttributesSharedHandlerReadsToTheBoundTool(t *testing.T) {
	census := newArgumentReadCensus(t)
	registrations := toolRegistrations(t)
	readsFor := func(tool string) map[string]bool {
		handler, binding := census.bindingFor(registrations[tool])
		keys := map[string]bool{}
		for _, read := range census.readsOf(handler, binding, map[string]bool{}) {
			keys[read.key] = true
		}
		return keys
	}

	for _, tc := range []struct {
		tool    string
		reads   []string
		ignores []string
	}{
		{tool: "pinchtab_click", reads: []string{"x", "humanize", "onDialog", "element", "target", "snap", "browser"}, ignores: []string{"pixels", "option"}},
		{tool: "pinchtab_type", reads: []string{"text", "value", "ref"}, ignores: []string{"x", "humanize", "snap"}},
		{tool: "pinchtab_wait", reads: []string{"timeoutMs", "timeout", "for", "value", "state"}},
		{tool: "pinchtab_key", reads: []string{"key", "text", "action"}},
	} {
		keys := readsFor(tc.tool)
		for _, key := range tc.reads {
			if !keys[key] {
				t.Errorf("census finds no read of %q on %s (found %v); it has lost track of an argument idiom", key, tc.tool, keys)
			}
		}
		for _, key := range tc.ignores {
			if keys[key] {
				t.Errorf("census attributes %q to %s, whose kind never reads it; the shared-handler narrowing is broken", key, tc.tool)
			}
		}
	}
}
