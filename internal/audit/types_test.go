package audit

import (
	"encoding/json"
	"go/ast"
	"go/token"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/srccensus"
	"github.com/pinchtab/pinchtab/pkg/pinchtabaudit"
)

func roundTrip[T any](t *testing.T, name string, in T) {
	t.Helper()
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("%s: marshal: %v", name, err)
	}
	var out T
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("%s: unmarshal: %v", name, err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Errorf("%s: round-trip mismatch\n in: %#v\nout: %#v", name, in, out)
	}
}

var ts = time.Date(2026, 7, 4, 12, 30, 0, 0, time.UTC)

func sampleConsoleLogEntry() ConsoleLogEntry {
	return ConsoleLogEntry{Timestamp: ts, Level: "error", Message: "boom", Source: "https://example.com/app.js"}
}

func sampleNetworkRequest() NetworkRequest {
	return NetworkRequest{
		URL: "https://example.com/api", Method: "GET", Status: 500, StatusText: "Internal Server Error",
		ResourceType: "XHR", MimeType: "application/json", StartTime: ts,
		Duration: 12.5, Size: 2048, Failed: true, Error: "net::ERR_FAILED",
	}
}

func sampleBrokenAsset() BrokenAsset {
	return BrokenAsset{URL: "https://example.com/logo.png", ResourceType: "Image", Status: 404, Error: "not found"}
}

func sampleInteractiveElement() InteractiveElement {
	return InteractiveElement{Ref: "e5", Role: "button", Name: "Submit", Tag: "button", Label: "Submit form", Disabled: true}
}

func sampleTimingMetrics() BrowserTimingMetrics {
	return BrowserTimingMetrics{
		TimeToFirstByte: 80, DOMContentLoaded: 350, Load: 900,
		FirstContentfulPaint: 400, LargestContentfulPaint: 850, CumulativeLayoutShift: 0.05,
	}
}

func sampleVisualDiff() VisualDiffResult {
	return VisualDiffResult{
		BaselinePath: "base.png", CurrentPath: "cur.png", DiffPath: "diff.png",
		DiffPixels: 120, DiffRatio: 0.02, Changed: true,
	}
}

func sampleSecurityFinding() SecurityFinding {
	return SecurityFinding{RuleID: "mixed-content", Severity: "medium", Detail: "http resource on https page", URL: "https://example.com"}
}

func sampleBrowserPageData() BrowserPageData {
	diff := sampleVisualDiff()
	return BrowserPageData{
		ScreenshotPath: "page.png", FullPageScreenshot: true,
		ConsoleLogs:         []ConsoleLogEntry{sampleConsoleLogEntry()},
		NetworkRequests:     []NetworkRequest{sampleNetworkRequest()},
		BrokenAssets:        []BrokenAsset{sampleBrokenAsset()},
		InteractiveElements: []InteractiveElement{sampleInteractiveElement()},
		AccessibilityScore:  87,
		VisualDiff:          &diff,
		TimingMetrics:       sampleTimingMetrics(),
	}
}

func samplePageResult() PageResult {
	return PageResult{
		URL: "https://example.com", Title: "Example", StatusCode: 200,
		Seaportal: map[string]any{"group": "home", "wordCount": float64(1200)},
		Browser:   sampleBrowserPageData(),
	}
}

func sampleAuditInput() AuditInput {
	return AuditInput{URLs: []string{"https://example.com"}, SitemapURL: "https://example.com/sitemap.xml", SeaportalFile: "report.json"}
}

func sampleAuditOptions() AuditOptions {
	return AuditOptions{SampleSize: 5, Screenshot: true, NetworkMonitor: true, VisualDiff: true, Concurrency: 4, OutputDir: "out"}
}

func sampleAuditReport() AuditReport {
	r := NewAuditReport()
	r.GeneratedAt = ts
	r.Input = sampleAuditInput()
	r.Options = sampleAuditOptions()
	r.Pages = []PageResult{samplePageResult()}
	r.SummaryScore = 91
	r.SecurityFindings = []SecurityFinding{sampleSecurityFinding()}
	r.Recommendations = []string{"fix broken logo image"}
	return r
}

func TestJSONRoundTrip(t *testing.T) {
	roundTrip(t, "ConsoleLogEntry", sampleConsoleLogEntry())
	roundTrip(t, "NetworkRequest", sampleNetworkRequest())
	roundTrip(t, "BrokenAsset", sampleBrokenAsset())
	roundTrip(t, "InteractiveElement", sampleInteractiveElement())
	roundTrip(t, "BrowserTimingMetrics", sampleTimingMetrics())
	roundTrip(t, "VisualDiffResult", sampleVisualDiff())
	roundTrip(t, "SecurityFinding", sampleSecurityFinding())
	roundTrip(t, "BrowserPageData", sampleBrowserPageData())
	roundTrip(t, "PageResult", samplePageResult())
	roundTrip(t, "AuditInput", sampleAuditInput())
	roundTrip(t, "AuditOptions", sampleAuditOptions())
	roundTrip(t, "AuditReport", sampleAuditReport())
}

func TestNewAuditReportSchemaVersion(t *testing.T) {
	r := NewAuditReport()
	if r.SchemaVersion != SchemaVersion {
		t.Fatalf("SchemaVersion = %q, want %q", r.SchemaVersion, SchemaVersion)
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := raw["schemaVersion"]; got != SchemaVersion {
		t.Fatalf("schemaVersion JSON field = %v, want %q", got, SchemaVersion)
	}
}

// TestNewAuditReportSchemaVersion above compares the report to the constant that
// stamped it, so it proves the report is stamped and the JSON field is spelled
// right but cannot notice the constant changing. The literal here is the second
// copy that can, and it is the point of the test.
//
// The audit version happens to be rendered into the report goldens, so a bump
// also reds internal/audit/report — but incidentally, and only because those
// goldens compare content: 1.0 and 9.9 are the same width, so a length-only
// golden would miss it. This assertion does not depend on that.
func TestSchemaVersionIsPinnedSoABumpMustBeAcknowledged(t *testing.T) {
	const pinned = "1.0"

	if SchemaVersion != pinned {
		t.Fatalf("audit SchemaVersion = %q, pinned at %q.\n"+
			"An audit schema bump changes what report consumers receive. If it is intended, update all three:\n"+
			"  1. this literal,\n"+
			"  2. the audit report goldens — run `go test ./internal/audit/report -update` and review the diff,\n"+
			"  3. schemaVersion in tests/e2e/fixtures/audit-site/golden-report.json.\n"+
			"The audit e2e scenarios assert the field EXISTS rather than pinning a value, so there is no EXPECTED_*_SCHEMA to change — unlike scrape.",
			SchemaVersion, pinned)
	}
}

// The audit snapshot path never measures layout (no bounds pass, no on-screen
// test), so the report must make no visibility claim at all. The positive
// assertions are here because an empty payload would satisfy the negative.
func TestReportJSONHasNoVisibleKeyForInteractiveElements(t *testing.T) {
	data, err := json.Marshal(sampleAuditReport())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	report := string(data)

	if strings.Contains(report, "visible") {
		t.Errorf("audit report JSON carries a visible key:\n%s", report)
	}
	for _, want := range []string{`"interactiveElements":[{`, `"ref":"e5"`, `"role":"button"`, `"tag":"button"`, `"label":"Submit form"`, `"disabled":true`} {
		if !strings.Contains(report, want) {
			t.Errorf("audit report JSON lost %s:\n%s", want, report)
		}
	}
}

// The SDK mirrors these structs by hand so the public surface never imports
// internal packages, which means nothing but this guard keeps a field removed from
// one side from surviving on the other.
//
// Every pair is compared on name + TYPE + tag, because a changed field type is a
// decode failure — the one divergence that actually breaks a consumer. The reason
// this once needed a weaker second tier was package qualification: a field reads as
// audit.ConsoleLogEntry on one side and pinchtabaudit.ConsoleLogEntry on the other,
// which is not divergence. normaliseMirrorType removes exactly that difference, so
// the tier is gone. Tier membership was a hand-maintained axis nothing guarded, and
// dropping to the weak tier disabled type comparison on a struct's OTHER fields as
// collateral — PageResult.StatusCode int was unguarded that way.
type mirrorPair struct {
	// internalName and sdkName are the declared type names. They are carried
	// explicitly rather than derived because two pairs are not name-equal, and the
	// census below credits a shared name only when a pair claims it on both sides.
	internalName string
	sdkName      string
	internal     any
	sdk          any
}

var mirrorPairs = []mirrorPair{
	{"InteractiveElement", "InteractiveElement", InteractiveElement{}, pinchtabaudit.InteractiveElement{}},
	{"ConsoleLogEntry", "ConsoleLogEntry", ConsoleLogEntry{}, pinchtabaudit.ConsoleLogEntry{}},
	{"JSError", "JSError", JSError{}, pinchtabaudit.JSError{}},
	{"NetworkRequest", "NetworkRequest", NetworkRequest{}, pinchtabaudit.NetworkRequest{}},
	{"BrokenAsset", "BrokenAsset", BrokenAsset{}, pinchtabaudit.BrokenAsset{}},
	{"SecurityFinding", "SecurityFinding", SecurityFinding{}, pinchtabaudit.SecurityFinding{}},
	{"A11yFinding", "A11yFinding", A11yFinding{}, pinchtabaudit.A11yFinding{}},
	{"VisualDiffResult", "VisualDiffResult", VisualDiffResult{}, pinchtabaudit.VisualDiffResult{}},
	{"AuditOptions", "AuditOptions", AuditOptions{}, pinchtabaudit.AuditOptions{}},
	// Not name-equal: the SDK drops the Browser prefix it has no other timings to
	// distinguish from. A name-equality census would report both as unguarded.
	{"BrowserTimingMetrics", "TimingMetrics", BrowserTimingMetrics{}, pinchtabaudit.TimingMetrics{}},
	// Also not name-equal, and for a sharper reason: the SDK's AuditInput is the
	// REQUEST shape, so the report's input block needed its own mirror.
	{"AuditInput", "AuditReportInput", AuditInput{}, pinchtabaudit.AuditReportInput{}},
	// The container shapes: their fields reference the mirrors above, which is what
	// normaliseMirrorType exists for. AuditReport.Input is the sharpest case — it is
	// internal AuditInput against SDK AuditReportInput, so stripping the qualifier
	// alone is not enough and the rename derived from this table is load-bearing.
	{"BrowserPageData", "BrowserPageData", BrowserPageData{}, pinchtabaudit.BrowserPageData{}},
	{"PageResult", "PageResult", PageResult{}, pinchtabaudit.PageResult{}},
	{"PageAudit", "PageAudit", PageAudit{}, pinchtabaudit.PageAudit{}},
	{"AuditReport", "AuditReport", AuditReport{}, pinchtabaudit.AuditReport{}},
}

// unmirroredSharedNames are type names both packages export that are deliberately NOT
// mirrors, each with its own reason. One blanket rationale is what let a live
// divergence hide here before: it covered a deliberate difference, two accidental
// omissions and an unchosen tag at once.
var unmirroredSharedNames = map[string]string{
	"AuditInput": "the two same-named types describe opposite directions: internal AuditInput is the report's input block, mirrored by the SDK's AuditReportInput and guarded as that pair, while the SDK's AuditInput carries SeaportalResults — raw bytes a caller sends and the server never echoes back",
	"PageOptions": "internal resolves each collector to a bool; the SDK uses *bool so nil means " +
		"keep the server default, which is a deliberately different wire contract rather than a drifted mirror",
	"RunOptions": "an in-process options struct on both sides, never marshalled: internal carries no json tags at all and the SDK nests *PageOptions",
}

// unmirroredSDKNames are SDK-exported structs that are not payload mirrors, and the
// list is one-sided on purpose. The shared-name census below cannot see a mirror added
// under a name internal/audit does not share — and that is not hypothetical: two of
// the pairs above are exactly that shape, so a third one added later would be claimed
// by nothing and reported by nothing. The SDK is the public surface, so this side is
// the one worth enumerating; internal/audit legitimately exports many types the SDK
// never mirrors.
var unmirroredSDKNames = map[string]string{
	"Client":      "the SDK's HTTP client, not a payload shape: it carries a base URL, a token and an *http.Client, none of which crosses the wire",
	"AuditInput":  "the REQUEST shape a caller sends; the report's input block is internal AuditInput paired with AuditReportInput",
	"PageOptions": "*bool per collector so nil means keep the server default, a deliberately different wire contract",
	"RunOptions":  "an in-process options struct, never marshalled",
}

// The two package qualifiers reflect prints in front of a mirrored field type.
const (
	internalQualifier = "audit"
	sdkQualifier      = "pinchtabaudit"
)

// qualifierPattern matches one package qualifier and the type name it qualifies.
// The leading \b is what keeps "audit" from matching inside "pinchtabaudit".
func qualifierPattern(qualifier string) *regexp.Regexp {
	return regexp.MustCompile(`\b` + regexp.QuoteMeta(qualifier) + `\.([A-Za-z_][A-Za-z0-9_]*)`)
}

var (
	internalQualifierRE = qualifierPattern(internalQualifier)
	sdkQualifierRE      = qualifierPattern(sdkQualifier)
)

// sdkToInternalNames maps each SDK type name to the internal name it mirrors. It is
// derived from mirrorPairs rather than written out again, so the two non-name-equal
// mirrors are recorded in exactly one place — the table that already carries both
// names.
func sdkToInternalNames() map[string]string {
	out := make(map[string]string, len(mirrorPairs))
	for _, pair := range mirrorPairs {
		out[pair.sdkName] = pair.internalName
	}
	return out
}

// normaliseMirrorType removes one package qualifier from a printed type and renames
// the type it qualified. Nothing else is touched: time.Time and map[string]any pass
// through verbatim, so the normaliser cannot equate two genuinely different types.
func normaliseMirrorType(typ string, re *regexp.Regexp, rename map[string]string) string {
	return re.ReplaceAllStringFunc(typ, func(match string) string {
		name := match[strings.IndexByte(match, '.')+1:]
		if renamed, ok := rename[name]; ok {
			return renamed
		}
		return name
	})
}

func mirrorShape(v any, re *regexp.Regexp, rename map[string]string) []string {
	rt := reflect.TypeOf(v)
	var out []string
	for f := range rt.Fields() {
		out = append(out, f.Name+" "+normaliseMirrorType(f.Type.String(), re, rename)+" "+f.Tag.Get("json"))
	}
	return out
}

func TestAuditPayloadTypesMatchTheirSDKMirrors(t *testing.T) {
	rename := sdkToInternalNames()
	for _, pair := range mirrorPairs {
		t.Run(pair.internalName, func(t *testing.T) {
			got := mirrorShape(pair.internal, internalQualifierRE, nil)
			want := mirrorShape(pair.sdk, sdkQualifierRE, rename)
			if len(got) == 0 {
				t.Fatalf("%s has no fields; the guard would pass vacuously", pair.internalName)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%s diverges from its SDK mirror %s\n internal: %v\n      sdk: %v",
					pair.internalName, pair.sdkName, got, want)
			}
		})
	}
}

// The normaliser is the whole reason one tier can cover every pair, so its narrowness
// is the property to pin: it must collapse the mirrored qualifier and nothing else.
// The floor below keeps the verbatim half of that claim load-bearing — without a real
// field typed time.Time and one typed map[string]any, it would be a claim over
// nothing.
func TestMirrorTypeNormaliserTouchesOnlyTheMirroredQualifier(t *testing.T) {
	rename := sdkToInternalNames()
	cases := []struct {
		typ  string
		re   *regexp.Regexp
		want string
	}{
		{"int", sdkQualifierRE, "int"},
		{"[]string", sdkQualifierRE, "[]string"},
		{"time.Time", sdkQualifierRE, "time.Time"},
		{"map[string]interface {}", sdkQualifierRE, "map[string]interface {}"},
		{"[]uint8", sdkQualifierRE, "[]uint8"},
		{"pinchtabaudit.PageResult", sdkQualifierRE, "PageResult"},
		{"[]pinchtabaudit.ConsoleLogEntry", sdkQualifierRE, "[]ConsoleLogEntry"},
		{"*pinchtabaudit.VisualDiffResult", sdkQualifierRE, "*VisualDiffResult"},
		{"pinchtabaudit.TimingMetrics", sdkQualifierRE, "BrowserTimingMetrics"},
		{"pinchtabaudit.AuditReportInput", sdkQualifierRE, "AuditInput"},
		{"audit.BrowserPageData", internalQualifierRE, "BrowserPageData"},
		{"time.Time", internalQualifierRE, "time.Time"},
		// The SDK qualifier ends in the internal one; stripping the internal
		// qualifier must leave an SDK-qualified type alone.
		{"pinchtabaudit.PageResult", internalQualifierRE, "pinchtabaudit.PageResult"},
	}
	for _, tc := range cases {
		if got := normaliseMirrorType(tc.typ, tc.re, rename); got != tc.want {
			t.Errorf("normaliseMirrorType(%q) = %q, want %q", tc.typ, got, tc.want)
		}
	}

	verbatim := map[string]bool{"time.Time": false, "map[string]interface {}": false}
	for _, pair := range mirrorPairs {
		rt := reflect.TypeOf(pair.sdk)
		for field := range rt.Fields() {
			typ := field.Type.String()
			if _, tracked := verbatim[typ]; tracked {
				verbatim[typ] = true
			}
		}
	}
	for typ, found := range verbatim {
		if !found {
			t.Errorf("no mirrored field is typed %s, so comparing it verbatim is a claim over nothing", typ)
		}
	}
}

// The rename half of the normaliser is derived from mirrorPairs. If every pair were
// name-equal the map would be an identity no-op that could be deleted without any
// test noticing — and AuditReport.Input, internal AuditInput against SDK
// AuditReportInput, is exactly the field that needs it.
func TestMirrorRenamesAreDerivedAndNotAnIdentityMap(t *testing.T) {
	renamed := map[string]string{}
	for sdk, internal := range sdkToInternalNames() {
		if sdk != internal {
			renamed[sdk] = internal
		}
	}
	if len(renamed) == 0 {
		t.Fatal("no pair is non-name-equal, so the rename map is an identity no-op; drop it or the pair table lost a mirror")
	}
	for sdk, internal := range renamed {
		if normaliseMirrorType(sdkQualifier+"."+sdk, sdkQualifierRE, sdkToInternalNames()) != internal {
			t.Errorf("rename %s -> %s is not applied by the normaliser", sdk, internal)
		}
	}
}

// The census that closes the class. One hardcoded struct became nine, and nothing
// stopped a tenth mirrored struct from being added to both packages and covered by
// neither. Every type name exported by BOTH packages must be claimed by a pair on
// both sides or carry a reason in unmirroredSharedNames; a name in neither fails.
func TestEverySharedAuditTypeNameIsClaimedOrExcused(t *testing.T) {
	internalNames := exportedStructNames(t, ".", 10)
	sdkNames := exportedStructNames(t, filepath.Join("..", "..", "pkg", "pinchtabaudit"), 2)

	claimedInternal := map[string]bool{}
	claimedSDK := map[string]bool{}
	for _, pair := range mirrorPairs {
		if !internalNames[pair.internalName] {
			t.Errorf("pair names internal type %q, which internal/audit does not declare", pair.internalName)
		}
		if !sdkNames[pair.sdkName] {
			t.Errorf("pair names SDK type %q, which pkg/pinchtabaudit does not declare", pair.sdkName)
		}
		claimedInternal[pair.internalName] = true
		claimedSDK[pair.sdkName] = true
	}

	var shared int
	for name := range internalNames {
		if !sdkNames[name] {
			continue
		}
		shared++
		if claimedInternal[name] && claimedSDK[name] {
			continue
		}
		if unmirroredSharedNames[name] != "" {
			continue
		}
		t.Errorf("%s is exported by both internal/audit and pkg/pinchtabaudit but is in no mirror pair "+
			"and has no reason in unmirroredSharedNames; add it to one", name)
	}
	if shared == 0 {
		t.Fatal("no shared type names found; this census would pass vacuously")
	}

	for name := range unmirroredSharedNames {
		if !internalNames[name] || !sdkNames[name] {
			t.Errorf("unmirroredSharedNames excuses %q, which is no longer exported by both packages", name)
		}
		// An excuse that also names a guarded pair is the next version of the blanket
		// rationale: it reads as covered from either table, so deleting the pair later
		// looks safe. AuditInput is excused for its NAME while internal AuditInput is
		// paired with the SDK's AuditReportInput, which is why the check is symmetry.
		if claimedInternal[name] && claimedSDK[name] {
			t.Errorf("%q is both excused in unmirroredSharedNames and compared as a pair; keep one, or deleting the pair will read as covered", name)
		}
	}
}

// The axis the shared-name census structurally cannot cover: a mirror added to the SDK
// under a name internal/audit does not share is in no pair, is not a shared name, and
// so is reported by nobody. Two of the pairs above are already non-name-equal, which is
// what makes this the realistic shape rather than a contrived one. Enumerating the SDK
// side closes it — a new SDK struct must be paired or excused however it is named.
func TestEverySDKStructIsMirroredOrExcused(t *testing.T) {
	sdkNames := exportedStructNames(t, filepath.Join("..", "..", "pkg", "pinchtabaudit"), 2)

	claimed := map[string]bool{}
	for _, pair := range mirrorPairs {
		claimed[pair.sdkName] = true
	}

	for name := range sdkNames {
		if claimed[name] || unmirroredSDKNames[name] != "" {
			continue
		}
		t.Errorf("pkg/pinchtabaudit exports %s, which no mirror pair claims and unmirroredSDKNames does not excuse. "+
			"If it mirrors an internal type — under any name — add the pair; if it is not a payload shape, record why", name)
	}

	// An excuse for a type the SDK no longer exports, or for one a pair also claims,
	// is the blanket-rationale failure this guard's lineage keeps closing: it reads as
	// covered from either table, so deleting the pair later looks safe.
	for name := range unmirroredSDKNames {
		if !sdkNames[name] {
			t.Errorf("unmirroredSDKNames excuses %q, which pkg/pinchtabaudit no longer exports", name)
		}
		if claimed[name] {
			t.Errorf("%q is both excused and compared as a pair; keep one", name)
		}
	}
	if len(sdkNames) == 0 {
		t.Fatal("no SDK structs found; this census would pass vacuously")
	}
}

// exportedStructNames scans a package directory for `type X struct` declarations.
// Reflection cannot enumerate a package, and the mirror lives in two of them.
func exportedStructNames(t *testing.T, dir string, minFiles int) map[string]bool {
	t.Helper()
	_, files := srccensus.Load(t, dir, minFiles).Syntax()

	names := map[string]bool{}
	for _, file := range files {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok || !ts.Name.IsExported() {
					continue
				}
				if _, ok := ts.Type.(*ast.StructType); ok {
					names[ts.Name.Name] = true
				}
			}
		}
	}
	return names
}
