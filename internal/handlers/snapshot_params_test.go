package handlers

import (
	"context"
	"encoding/json"
	"go/ast"
	"go/token"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/srccensus"
	"gopkg.in/yaml.v3"
)

func parseQuery(t *testing.T, raw string) (SnapshotCostControls, error) {
	t.Helper()
	q, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatalf("bad probe query %q: %v", raw, err)
	}
	return ParseSnapshotCostControls(q)
}

// Every cost control used to fail OPEN toward the expensive answer: an unrecognised format
// fell through to json, a mis-cased filter returned the whole tree instead of the
// interactive subset, and an unparseable budget or depth was dropped. A caller that
// mistyped its own cost control was not told, only charged more.
func TestInvalidCostControlsAreRejectedRatherThanIgnored(t *testing.T) {
	for _, tc := range []struct {
		name      string
		query     string
		wantNamed []string
	}{
		{
			name:      "unknown format does not silently become json",
			query:     "format=compct",
			wantNamed: []string{"json", "compact", "text", "yaml"},
		},
		{
			name:      "format that is a real word but not ours",
			query:     "format=xml",
			wantNamed: []string{"compact"},
		},
		{
			name:      "unknown filter does not silently become the whole tree",
			query:     "filter=interactives",
			wantNamed: []string{"all", "interactive"},
		},
		{
			name:      "unparseable maxTokens does not silently drop the budget",
			query:     "maxTokens=lots",
			wantNamed: []string{"positive"},
		},
		{
			name:      "zero maxTokens is not a budget",
			query:     "maxTokens=0",
			wantNamed: []string{"positive"},
		},
		{
			name:      "negative maxTokens is not a budget",
			query:     "maxTokens=-5",
			wantNamed: []string{"positive"},
		},
		{
			name:      "unparseable depth does not silently drop the limit",
			query:     "depth=deep",
			wantNamed: []string{"-1"},
		},
		{
			name:      "depth below the no-limit sentinel is meaningless",
			query:     "depth=-2",
			wantNamed: []string{"-1"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseQuery(t, tc.query)
			if err == nil {
				t.Fatalf("%s was accepted; the caller gets an answer to a question it did not ask", tc.query)
			}
			for _, want := range tc.wantNamed {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("the rejection does not name %q, so the caller cannot tell what to send instead: %v", want, err)
				}
			}
			// The rejection has to quote what was actually sent, or a caller with a
			// generated query cannot find which value was wrong.
			sent := strings.SplitN(tc.query, "=", 2)[1]
			if !strings.Contains(err.Error(), sent) {
				t.Errorf("the rejection does not quote the offending value %q: %v", sent, err)
			}
		})
	}
}

// A single capital letter used to turn an interactive snapshot into a full-tree one.
func TestFormatAndFilterAreCaseAndWhitespaceInsensitive(t *testing.T) {
	for _, query := range []string{
		"format=COMPACT&filter=INTERACTIVE",
		"format=Compact&filter=Interactive",
		"format=+compact+&filter=+interactive+",
		"format=%09compact%0A&filter=%20interactive%20",
	} {
		t.Run(query, func(t *testing.T) {
			got, err := parseQuery(t, query)
			if err != nil {
				t.Fatalf("rejected a value that differs only in case or spacing: %v", err)
			}
			if got.Format != "compact" {
				t.Errorf("format = %q, want compact", got.Format)
			}
			if got.Filter != bridge.FilterInteractive {
				t.Errorf("filter = %q, want %q — a mis-cased filter used to return the whole tree", got.Filter, bridge.FilterInteractive)
			}
		})
	}
}

func TestCostControlDefaultsWhenUnset(t *testing.T) {
	got, err := parseQuery(t, "tabId=abc")
	if err != nil {
		t.Fatal(err)
	}
	if got.Format != "json" {
		t.Errorf("format = %q, want json", got.Format)
	}
	if got.Filter != "" {
		t.Errorf("filter = %q, want the whole tree", got.Filter)
	}
	if got.MaxTokens != -1 || got.MaxDepth != -1 {
		t.Errorf("maxTokens = %d, depth = %d, want -1 for both", got.MaxTokens, got.MaxDepth)
	}
	if len(got.Ignored) != 0 {
		t.Errorf("ignored = %v, want none", got.Ignored)
	}
}

// "all" is the documented spelling of the default and has to be accepted explicitly, not
// merely tolerated by falling through the unknown-filter branch.
func TestFilterAllIsTheWholeTree(t *testing.T) {
	got, err := parseQuery(t, "filter=all")
	if err != nil {
		t.Fatalf("filter=all was rejected: %v", err)
	}
	if got.Filter != "" {
		t.Errorf("filter = %q, want the whole tree", got.Filter)
	}
}

// The decision on this card: unknown NAMES are reported, not rejected, so version skew in
// the normal direction — a newer client sending a parameter an older server has not learned
// — keeps working while a typo stops being invisible.
func TestUnknownParamsAreReportedNotRejected(t *testing.T) {
	got, err := parseQuery(t, "tabId=abc&compact=true&interactve=true&format=compact")
	if err != nil {
		t.Fatalf("an unknown parameter was rejected; version skew would break: %v", err)
	}
	if want := []string{"compact", "interactve"}; !equalStrings(got.Ignored, want) {
		t.Errorf("ignored = %v, want %v — this is the disclosure that would have caught `compact=true` in the CLI", got.Ignored, want)
	}
	if got.Format != "compact" {
		t.Errorf("format = %q, want the valid parameter still honoured", got.Format)
	}
}

// `interactive` is documented on /snapshot as the boolean alias for filter=interactive, and
// a shipped example still sends it. It was advertised without ever being read, so a raw HTTP
// caller that followed the docs paid for the whole tree and was told nothing. The alias has
// to be honoured on the wire, not merely disclosed as ignored.
func TestInteractiveAliasSelectsTheInteractiveSubset(t *testing.T) {
	for _, tc := range []struct {
		query string
		want  string
	}{
		{query: "tabId=abc&interactive=true", want: bridge.FilterInteractive},
		{query: "tabId=abc&interactive=TRUE", want: bridge.FilterInteractive},
		{query: "tabId=abc&interactive=+true+", want: bridge.FilterInteractive},
		{query: "tabId=abc&interactive=1", want: bridge.FilterInteractive},
		{query: "tabId=abc&interactive=false", want: ""},
		{query: "tabId=abc&interactive=0", want: ""},
		{query: "tabId=abc&interactive=", want: ""},
		{query: "tabId=abc&filter=interactive&interactive=true", want: bridge.FilterInteractive},
		{query: "tabId=abc&filter=all&interactive=false", want: ""},
	} {
		t.Run(tc.query, func(t *testing.T) {
			got, err := parseQuery(t, tc.query)
			if err != nil {
				t.Fatalf("the documented alias was rejected: %v", err)
			}
			if got.Filter != tc.want {
				t.Errorf("filter = %q, want %q — the alias resolved to the wrong subset", got.Filter, tc.want)
			}
			if len(got.Ignored) != 0 {
				t.Errorf("ignored = %v, but interactive is a parameter the server reads", got.Ignored)
			}
		})
	}
}

// A bad alias value must fail the same way every other cost control does. Falling through to
// the whole tree is the exact failure this parser exists to end: `interactive=yes` would
// otherwise buy the expensive answer silently.
func TestInteractiveAliasRefusesValuesItCannotRead(t *testing.T) {
	for _, query := range []string{
		"interactive=yes",
		"interactive=on",
		"interactive=interactive",
		"interactive=2",
	} {
		t.Run(query, func(t *testing.T) {
			_, err := parseQuery(t, query)
			if err == nil {
				t.Fatalf("%s was accepted; a caller that mistyped the alias silently gets the whole tree", query)
			}
			sent := strings.SplitN(query, "=", 2)[1]
			if !strings.Contains(err.Error(), sent) {
				t.Errorf("the rejection does not quote the offending value %q: %v", sent, err)
			}
			for _, want := range []string{"interactive", "true", "false"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("the rejection does not name %q, so the caller cannot tell what to send instead: %v", want, err)
				}
			}
		})
	}
}

// An alias only stays honest while it cannot disagree with the thing it aliases. Picking a
// winner by precedence would mean one of the two parameters the caller sent did nothing,
// which is the silence this endpoint stopped keeping.
func TestFilterAndInteractiveMayNotContradictEachOther(t *testing.T) {
	for _, query := range []string{
		"filter=all&interactive=true",
		"filter=interactive&interactive=false",
	} {
		t.Run(query, func(t *testing.T) {
			_, err := parseQuery(t, query)
			if err == nil {
				t.Fatalf("%s was accepted, so one of the two parameters was silently discarded", query)
			}
			for _, want := range []string{"filter", "interactive"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("the rejection does not name %q, so the caller cannot tell which pair conflicts: %v", want, err)
				}
			}
		})
	}
}

// The failure this guard exists for is a parameter the request path DOES read being
// reported as ignored: `browser` is consumed by the routing prelude rather than by
// HandleSnapshot, so a known-params set derived from one file alone gets it wrong and every
// browser-routed call grows a false disclosure.
func TestParamsTheRequestPathReadsAreNotReportedAsIgnored(t *testing.T) {
	got, err := parseQuery(t, "tabId=abc&browser=chrome&filter=interactive&interactive=true&format=compact&depth=3&maxTokens=500&diff=true&noAnimations=true&selector=%23main&output=file&path=out.txt")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Ignored) != 0 {
		t.Errorf("ignored = %v, but every one of those is read on the snapshot request path", got.Ignored)
	}
}

// snapshotPathOwners are the declarations whose files' query reads can run for a /snapshot
// request; each is located wherever it now lives, so the scan follows a move. Listed rather
// than derived because the call graph decides membership. ParseSnapshotCostControls is on it
// because that is where the cost controls moved: a scan of the handler alone would no longer
// see `filter`, `format`, `depth`, `maxTokens` or the `interactive` alias at all.
var snapshotPathOwners = []string{"HandleSnapshot", "resolveReadRouting", "ParseSnapshotCostControls"}

func snapshotPathSources(t *testing.T) ([]string, map[string]*ast.File) {
	t.Helper()
	pkg := srccensus.Load(t, ".", handlersSourceFileFloor)
	_, files := pkg.Syntax()
	var names []string
	seen := map[string]bool{}
	for _, owner := range snapshotPathOwners {
		fn, ok := pkg.Func(owner)
		if !ok {
			t.Fatalf("%s is no longer declared anywhere in %s; re-point snapshotPathOwners at whatever now reads its parameters rather than deleting it", owner, pkg.Dir())
		}
		if !seen[fn.File] {
			seen[fn.File] = true
			names = append(names, fn.File)
		}
	}
	return names, files
}

func queryGetLiterals(file *ast.File) []string {
	var params []string
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 1 {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); !ok || sel.Sel.Name != "Get" {
			return true
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		if param, err := strconv.Unquote(lit.Value); err == nil && param != "" {
			params = append(params, param)
		}
		return true
	})
	return params
}

// notOnTheSnapshotPath are parameters those files read from a function /snapshot never
// calls. They belong OUT of snapshotKnownParams: /snapshot genuinely ignores them, so
// reporting them back is the correct answer rather than a false disclosure. Checked in
// both directions below, because an exemption for something the source no longer reads is
// how a guard quietly stops guarding.
var notOnTheSnapshotPath = map[string]string{
	"frameId": "read by resolveTargetFrameID, which /inspect and /text call and HandleSnapshot does not; /snapshot scopes frames from tab state instead",
}

// A hand-maintained allowlist drifts the moment a parameter is added, and it drifts
// SILENTLY — the only symptom is a caller told its valid parameter was ignored. This reads
// the parameters straight out of the source instead of trusting the list to have kept up.
func TestKnownParamsCoversEveryParamTheHandlerReads(t *testing.T) {
	var missing []string
	seen := 0
	reported := map[string]bool{}
	exemptionsUsed := map[string]bool{}
	sources, files := snapshotPathSources(t)
	for _, name := range sources {
		for _, param := range queryGetLiterals(files[name]) {
			seen++
			if _, exempt := notOnTheSnapshotPath[param]; exempt {
				exemptionsUsed[param] = true
				continue
			}
			if !snapshotKnownParams[param] && !reported[name+param] {
				reported[name+param] = true
				missing = append(missing, name+": "+param)
			}
		}
	}
	if seen < len(sources) {
		t.Fatalf("found %d query reads across %v; the scan matched almost nothing and would pass vacuously", seen, sources)
	}
	if len(missing) > 0 {
		t.Errorf("these parameters are read on the snapshot request path but are absent from snapshotKnownParams, so a caller sending them is wrongly told they were ignored:\n  %s",
			strings.Join(missing, "\n  "))
	}
	for param, why := range notOnTheSnapshotPath {
		if !exemptionsUsed[param] {
			t.Errorf("%q is exempted (%s) but no longer read by %v; drop the exemption, or re-point it, rather than leaving it to excuse nothing", param, why, sources)
		}
		if snapshotKnownParams[param] {
			t.Errorf("%q is both exempted and listed as known; one of the two is wrong", param)
		}
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

var (
	tsFormatEnum  = regexp.MustCompile(`(?m)^\s*format:\s*\{[^}\n]*\benum:\s*\[([^\]\n]*)\]`)
	quotedLiteral = regexp.MustCompile(`"([^"]*)"`)
)

func tsSnapshotFormatEnum(t *testing.T, src []byte) []string {
	t.Helper()
	matches := tsFormatEnum.FindAllSubmatch(src, -1)
	if len(matches) != 1 {
		t.Fatalf("found %d `format: { ... enum: [...] }` properties, want exactly one", len(matches))
	}
	var enum []string
	for _, lit := range quotedLiteral.FindAllSubmatch(matches[0][1], -1) {
		enum = append(enum, string(lit[1]))
	}
	return enum
}

func manifestSnapshotFormatEnum(t *testing.T, src []byte) []string {
	t.Helper()
	var manifest struct {
		ConfigSchema struct {
			Properties struct {
				DefaultSnapshotFormat struct {
					Enum []string `json:"enum"`
				} `json:"defaultSnapshotFormat"`
			} `json:"properties"`
		} `json:"configSchema"`
	}
	if err := json.Unmarshal(src, &manifest); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
	return manifest.ConfigSchema.Properties.DefaultSnapshotFormat.Enum
}

func TestOpenClawPluginAdvertisesExactlyTheServedSnapshotFormats(t *testing.T) {
	want := slices.Sorted(slices.Values(snapshotFormats))
	for _, source := range []struct {
		file    string
		extract func(*testing.T, []byte) []string
	}{
		{"tools/pinchtab.ts", tsSnapshotFormatEnum},
		{"tools/browser.ts", tsSnapshotFormatEnum},
		{"openclaw.plugin.json", manifestSnapshotFormatEnum},
	} {
		t.Run(source.file, func(t *testing.T) {
			src, err := os.ReadFile(filepath.Join("..", "..", "plugins", "openclaw", filepath.FromSlash(source.file)))
			if err != nil {
				t.Fatal(err)
			}
			got := source.extract(t, src)
			if len(got) == 0 {
				t.Fatalf("plugins/openclaw/%s yields no snapshot format enum, so this guard is checking nothing", source.file)
			}
			slices.Sort(got)
			if !equalStrings(got, want) {
				t.Errorf("plugins/openclaw/%s advertises snapshot formats %v, the server serves %v (snapshotFormats)", source.file, got, want)
			}
		})
	}
}

type snapshotStubBridge struct {
	*mockBridge
	gotFilter string
}

func (b *snapshotStubBridge) Snapshot(_ context.Context, _ string, filter string, _ bridge.ContentParams) (*bridge.SnapshotResult, error) {
	b.gotFilter = filter
	return &bridge.SnapshotResult{
		Nodes: []bridge.A11yNode{{Role: "button", Name: "Buy"}},
		URL:   "http://127.0.0.1:1/fixture",
		Title: "Fixture",
	}, nil
}

func (b *snapshotStubBridge) GetRefCache(string) *bridge.RefCache {
	return &bridge.RefCache{Nodes: []bridge.A11yNode{{Role: "link", Name: "Home"}}}
}

func serveSnapshot(t *testing.T, query string) (*httptest.ResponseRecorder, *snapshotStubBridge) {
	t.Helper()
	stub := &snapshotStubBridge{mockBridge: &mockBridge{}}
	h := New(stub, &config.RuntimeConfig{
		ActionTimeout:     5 * time.Second,
		DefaultBrowser:    config.BrowserGhostChrome,
		BrowsersAvailable: []string{config.BrowserGhostChrome},
		StateDir:          t.TempDir(),
	}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/snapshot?"+query, nil)
	w := httptest.NewRecorder()
	h.HandleSnapshot(w, req)
	return w, stub
}

func snapshotBodyFor(t *testing.T, query string) string {
	t.Helper()
	w, _ := serveSnapshot(t, query)
	if w.Code != http.StatusOK {
		t.Fatalf("snapshot?%s: status %d body=%s", query, w.Code, w.Body.String())
	}
	return w.Body.String()
}

// The parser resolving the alias is not the same claim as the request path carrying it: the
// regression this guards is wire-level, so it is checked at the wire.
func TestTheInteractiveAliasReachesTheBridgeAsAFilter(t *testing.T) {
	for _, tc := range []struct {
		query string
		want  string
	}{
		{query: "tabId=tab1&interactive=true", want: bridge.FilterInteractive},
		{query: "tabId=tab1&interactive=false", want: ""},
		{query: "tabId=tab1&filter=interactive", want: bridge.FilterInteractive},
		{query: "tabId=tab1", want: ""},
	} {
		t.Run(tc.query, func(t *testing.T) {
			w, stub := serveSnapshot(t, tc.query)
			if w.Code != http.StatusOK {
				t.Fatalf("status %d body=%s", w.Code, w.Body.String())
			}
			if stub.gotFilter != tc.want {
				t.Errorf("the bridge was asked for filter %q, want %q — the caller pays for a tree it did not request", stub.gotFilter, tc.want)
			}
			if strings.Contains(w.Body.String(), "ignoredParams") {
				t.Errorf("a parameter the request path honoured was disclosed as ignored:\n%s", w.Body.String())
			}
		})
	}
}

// A refused cost control has to reach the caller as a 400 naming it, not as an expensive 200.
func TestContradictoryFilterAndInteractiveAnswer400(t *testing.T) {
	w, stub := serveSnapshot(t, "tabId=tab1&filter=all&interactive=true")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400: %s", w.Code, w.Body.String())
	}
	if stub.gotFilter != "" {
		t.Errorf("the snapshot was captured anyway (filter %q); a refused request must not be charged", stub.gotFilter)
	}
	for _, want := range []string{"filter", "interactive"} {
		if !strings.Contains(w.Body.String(), want) {
			t.Errorf("the 400 body does not name %q: %s", want, w.Body.String())
		}
	}
}

func jsonEnvelope(t *testing.T, body string) []string {
	t.Helper()
	return ignoredFromEnvelope(t, body, func(b []byte, v any) error { return json.Unmarshal(b, v) })
}

func yamlEnvelope(t *testing.T, body string) []string {
	t.Helper()
	return ignoredFromEnvelope(t, body, func(b []byte, v any) error { return yaml.Unmarshal(b, v) })
}

func ignoredFromEnvelope(t *testing.T, body string, unmarshal func([]byte, any) error) []string {
	t.Helper()
	var envelope struct {
		IgnoredParams []string `json:"ignoredParams" yaml:"ignoredParams"`
	}
	if err := unmarshal([]byte(body), &envelope); err != nil {
		t.Fatalf("cannot read the response envelope: %v\n%s", err, body)
	}
	return envelope.IgnoredParams
}

func ignoredFromComment(t *testing.T, body string) []string {
	t.Helper()
	for line := range strings.SplitSeq(body, "\n") {
		if rest, found := strings.CutPrefix(line, "# ignored params: "); found {
			return strings.Split(rest, ", ")
		}
	}
	t.Fatalf("no ignored-params comment in the response:\n%s", body)
	return nil
}

func TestTheDisclosureReachesTheCaller(t *testing.T) {
	const mistyped = "tabId=tab1&compact=true&maxtokens=50"
	want := []string{"compact", "maxtokens"}

	for _, tc := range []struct {
		name    string
		format  string
		extra   string
		witness string
		extract func(*testing.T, string) []string
	}{
		{format: "json", witness: "vocabularyToken", extract: jsonEnvelope},
		{format: "yaml", witness: "nodes:", extract: yamlEnvelope},
		{format: "compact", witness: "| 1 nodes\n", extract: ignoredFromComment},
		{format: "text", witness: "# Fixture\n#", extract: ignoredFromComment},
		{name: "json diff", format: "json", extra: "&diff=true", witness: "counts", extract: jsonEnvelope},
		{name: "compact diff", format: "compact", extra: "&diff=true", witness: "| +", extract: ignoredFromComment},
		{name: "file output", format: "json", extra: "&output=file", witness: "timestamp", extract: jsonEnvelope},
	} {
		name := tc.name
		if name == "" {
			name = tc.format
		}
		t.Run(name, func(t *testing.T) {
			body := snapshotBodyFor(t, mistyped+"&format="+tc.format+tc.extra)
			if !strings.Contains(body, tc.witness) {
				t.Fatalf("this case no longer reaches the %s branch (nothing matched %q), so whatever it asserts is about some other response:\n%s", name, tc.witness, body)
			}
			if got := tc.extract(t, body); !equalStrings(got, want) {
				t.Errorf("the %s response discloses %v, want %v — a caller whose cost control did nothing is told nothing", name, got, want)
			}
		})
	}
}

func TestAResponseWithoutIgnoredParamsCarriesNoDisclosure(t *testing.T) {
	for _, format := range []string{"json", "yaml", "compact", "text"} {
		t.Run(format, func(t *testing.T) {
			body := snapshotBodyFor(t, "tabId=tab1&filter=interactive&format="+format)
			if strings.Contains(body, "ignoredParams") || strings.Contains(body, "ignored params") {
				t.Errorf("a clean query is told something was ignored:\n%s", body)
			}
		})
	}
}

// snapshotFraming belongs here for the same reason attachIgnoredParams does: it
// discloses.
// It emits ignoredParamsComment unconditionally as part of the framing it returns,
// so a text branch that writes its framing has told the caller what was ignored.
//
// This is the scan being taught the helper that replaced writeIgnoredParamsComment,
// not being relaxed. Every response
// branch must still call one of these, and a branch that calls none still fails —
// which is what caught the text branches when the disclosure moved inside the
// framing and the scan could no longer see it by name.
var disclosureHelpers = map[string]bool{
	"attachIgnoredParams": true,
	"snapshotFraming":     true,
}

// The framing helper only counts as a disclosure while it actually makes one, so
// this fails if ignoredParamsComment ever stops being part of what it returns.
func TestSnapshotFramingIsWhyItCountsAsADisclosure(t *testing.T) {
	framing := snapshotFraming("compact", "# T | u | 1 nodes", "", []string{"bogusParam"}, false, 0)
	if !strings.Contains(framing, "bogusParam") {
		t.Fatalf("snapshotFraming is trusted by disclosureHelpers to disclose ignored params, but %q carries none; drop it from that set or restore the disclosure", framing)
	}
}

func successResponseBlocks(fn *ast.FuncDecl) []ast.Node {
	var stack, blocks []ast.Node
	seen := map[ast.Node]bool{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return false
		}
		if call, ok := n.(*ast.CallExpr); ok && writesSuccess(call) {
			if block := innermostBlock(stack); block != nil && !seen[block] {
				seen[block] = true
				blocks = append(blocks, block)
			}
		}
		stack = append(stack, n)
		return true
	})
	return blocks
}

func writesSuccess(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	switch sel.Sel.Name {
	case "JSON":
		return len(call.Args) > 1 && isStatusOK(call.Args[1])
	case "WriteHeader":
		return len(call.Args) == 1 && isStatusOK(call.Args[0])
	}
	return false
}

func isStatusOK(arg ast.Expr) bool {
	switch v := arg.(type) {
	case *ast.BasicLit:
		return v.Value == "200"
	case *ast.SelectorExpr:
		return v.Sel.Name == "StatusOK"
	}
	return false
}

func innermostBlock(stack []ast.Node) ast.Node {
	for i := len(stack) - 1; i >= 0; i-- {
		switch stack[i].(type) {
		case *ast.BlockStmt, *ast.CaseClause:
			return stack[i]
		}
	}
	return nil
}

func disclosesIgnoredParams(block ast.Node) bool {
	found := false
	ast.Inspect(block, func(n ast.Node) bool {
		if n == nil || found {
			return false
		}
		if n != block && opensANestedBranch(n) {
			return false
		}
		if call, ok := n.(*ast.CallExpr); ok {
			if name, ok := call.Fun.(*ast.Ident); ok && disclosureHelpers[name.Name] {
				found = true
			}
		}
		return !found
	})
	return found
}

func opensANestedBranch(n ast.Node) bool {
	switch n.(type) {
	case *ast.BlockStmt, *ast.CaseClause, *ast.CommClause:
		return true
	}
	return false
}

var snapshotResponseWriters = []string{"HandleSnapshot", "writeSnapshotPlain", "writeSnapshotFile", "writeSnapshotDiff", "writeSnapshot"}

func TestEveryResponseBranchDisclosesIgnoredParams(t *testing.T) {
	pkg := srccensus.Load(t, ".", handlersSourceFileFloor)
	fset, _ := pkg.Syntax()
	var blocks []ast.Node
	for _, name := range snapshotResponseWriters {
		fn, _ := handlerFuncDecl(t, pkg, name)
		blocks = append(blocks, successResponseBlocks(fn)...)
	}
	if len(blocks) < 5 {
		t.Fatalf("found %d response branches across %v; there are five (file, diff, plain text, yaml, json), so the scan is matching the wrong thing — lower this floor only for a branch deliberately removed", len(blocks), snapshotResponseWriters)
	}
	for _, block := range blocks {
		if !disclosesIgnoredParams(block) {
			t.Errorf("%s: this response branch calls neither attachIgnoredParams nor snapshotFraming, so a caller served by it is never told which of its parameters did nothing; the helpers make the disclosure easy to add, and only this scan makes it hard to forget",
				fset.Position(block.Pos()))
		}
	}
}
