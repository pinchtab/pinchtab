package types

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/srccensus"
)

const (
	moduleRoot     = "../../.."
	minModuleFiles = 300
	headerHomeFile = "internal/api/types/headers.go"
)

// exemptHeaders are the X-PinchTab-* names that legitimately live outside the wire-header
// home. Each is already single-homed and belongs to a different contract, so the census
// allows its one spelling to sit in the named file rather than in api/types. A new wire
// header must go in headers.go instead of being added here.
var exemptHeaders = map[string]string{
	"X-PinchTab-Internal-Token":      "internal/handlers/trust.go: server-only trusted-hop token, never part of the CLI/MCP wire contract",
	"X-PinchTab-Event":               "internal/scheduler/webhook.go: outbound event-webhook contract, separate from the wire headers",
	"X-PinchTab-Task-ID":             "internal/scheduler/webhook.go: outbound event-webhook contract, separate from the wire headers",
	"X-PinchTab-Session-Id":          "internal/activity/context.go: activity-only identity header, already single-homed",
	"X-PinchTab-Instance-Id":         "internal/activity/context.go: activity-only identity header, already single-homed",
	"X-PinchTab-Profile-Id":          "internal/activity/context.go: activity-only identity header, already single-homed",
	"X-PinchTab-Profile-Name":        "internal/activity/context.go: activity-only identity header, already single-homed",
	"X-PinchTab-Tab-Created":         "internal/activity/context.go: activity-only identity header, already single-homed",
	"X-Pinchtab-Failure-Code":        "internal/httpx/httpx.go: failure reason stamped by an instance for the front door and stripped at the public boundary",
	"X-Pinchtab-Failure-Message":     "internal/httpx/httpx.go: failure reason stamped by an instance for the front door and stripped at the public boundary",
	"X-Pinchtab-Proxy-Authorization": "internal/proxy/proxy_ws.go: backend credential on the server-only websocket proxy hop",
	"X-Pinchtab-":                    "internal/handlers/trust.go: the prefix the ingress strip matches, not a header name; a second copy is a header assembled from parts",
	"x-pinchtab-enabled":             "internal/handlers/openapi.go: OpenAPI vendor extension key, not an HTTP header",
	"x-pinchtab-security":            "internal/handlers/openapi.go: OpenAPI vendor extension key, not an HTTP header",
}

func headerKey(name string) string {
	return strings.ToLower(name)
}

func isExempt(header string) bool {
	for name := range exemptHeaders {
		if headerKey(name) == header {
			return true
		}
	}
	return false
}

// pinchtabHeaderLiterals maps each X-PinchTab-* string literal to the module-relative files
// it is spelled in. It reads AST string literals only, so a comment naming a header (this
// package's own doc comment does) is not counted.
func pinchtabHeaderLiterals(t *testing.T, files []srccensus.SourceFile) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	for _, f := range files {
		parsed, err := parser.ParseFile(token.NewFileSet(), f.Name, f.Text, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", f.Name, err)
		}
		ast.Inspect(parsed, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			val, err := strconv.Unquote(lit.Value)
			if err == nil && (strings.HasPrefix(headerKey(val), "x-pinchtab-") || strings.HasPrefix(headerKey(val), "x-idpi-")) {
				out[headerKey(val)] = append(out[headerKey(val)], f.Name)
			}
			return true
		})
	}
	return out
}

func singleHomeViolations(headers map[string][]string) []string {
	var violations []string
	for header, locs := range headers {
		if len(locs) != 1 {
			sort.Strings(locs)
			violations = append(violations, fmt.Sprintf("%s is spelled in %d places, want exactly one so a one-character drift is a compile error, not a silent no-op: %v", header, len(locs), locs))
			continue
		}
		if isExempt(header) {
			if locs[0] == headerHomeFile {
				violations = append(violations, fmt.Sprintf("%s is listed as exempt but sits in the wire-header home %s", header, headerHomeFile))
			}
			continue
		}
		if locs[0] != headerHomeFile {
			violations = append(violations, fmt.Sprintf("%s is a wire header spelled in %s; move it to %s, or if it is not a wire header add it to exemptHeaders with a reason", header, locs[0], headerHomeFile))
		}
	}
	sort.Strings(violations)
	return violations
}

func TestEachWireHeaderLiteralIsSingleHomed(t *testing.T) {
	headers := pinchtabHeaderLiterals(t, srccensus.Tree(t, moduleRoot, minModuleFiles))

	for _, wire := range []string{HeaderVocab, HeaderTabID, HeaderSource, HeaderIDPIWarning, HeaderIDPIPattern} {
		locs := headers[headerKey(wire)]
		if len(locs) != 1 || locs[0] != headerHomeFile {
			t.Errorf("wire header %s must be spelled exactly once, in %s; found %v", wire, headerHomeFile, locs)
		}
	}

	if v := singleHomeViolations(headers); len(v) > 0 {
		t.Errorf("X-PinchTab-* header names are not single-homed:\n%s", strings.Join(v, "\n"))
	}
}

// The guard bites: a second spelling of a wire header is reported as a violation.
func TestCensusFlagsADuplicateSpelling(t *testing.T) {
	planted := []srccensus.SourceFile{
		{Name: headerHomeFile, Text: "package types\nconst A = \"X-PinchTab-Vocab\"\n"},
		{Name: "internal/somewhere/dup.go", Text: "package somewhere\nconst B = \"X-PinchTab-Vocab\"\n"},
	}
	headers := pinchtabHeaderLiterals(t, planted)
	if len(headers[headerKey(HeaderVocab)]) != 2 {
		t.Fatalf("the AST census did not see the planted duplicate: %v", headers[headerKey(HeaderVocab)])
	}
	if len(singleHomeViolations(headers)) == 0 {
		t.Fatal("a duplicated wire-header spelling passed the census; a typo on one side would silently disable the feature")
	}
}

func TestCensusFlagsADuplicateInAnotherCase(t *testing.T) {
	for _, spelling := range []string{"X-Pinchtab-Vocab", "x-pinchtab-vocab"} {
		planted := []srccensus.SourceFile{
			{Name: headerHomeFile, Text: "package types\nconst A = \"X-PinchTab-Vocab\"\n"},
			{Name: "internal/somewhere/dup.go", Text: "package somewhere\nconst B = \"" + spelling + "\"\n"},
		}
		if len(singleHomeViolations(pinchtabHeaderLiterals(t, planted))) == 0 {
			t.Errorf("%q passed the census as a new header, but HTTP header names are case-insensitive, so it names the same wire header", spelling)
		}
	}
}

// A comment that names a header is not a spelling: the AST walk sees literals only.
func TestCensusIgnoresHeadersNamedInComments(t *testing.T) {
	commentOnly := []srccensus.SourceFile{
		{Name: "internal/foo/foo.go", Text: "package foo\n// mentions X-PinchTab-Vocab in prose only\nconst X = 1\n"},
	}
	if got := pinchtabHeaderLiterals(t, commentOnly)[headerKey(HeaderVocab)]; len(got) != 0 {
		t.Fatalf("a header named only in a comment was counted as a spelling: %v", got)
	}
}

const internalTokenHeaderValue = "X-PinchTab-Internal-Token"

// knownHeaderConsts maps the X-PinchTab-* header constants (by their identifier name) to the
// wire value, so a Set call using a constant is resolved the same as one using a literal.
var knownHeaderConsts = map[string]string{
	"HeaderVocab":         "X-PinchTab-Vocab",
	"HeaderTabID":         "X-PinchTab-Tab-Id",
	"HeaderSource":        "X-PinchTab-Source",
	"HeaderPTSessionID":   "X-PinchTab-Session-Id",
	"HeaderPTSource":      "X-PinchTab-Source",
	"HeaderPTInstance":    "X-PinchTab-Instance-Id",
	"HeaderPTProfileID":   "X-PinchTab-Profile-Id",
	"HeaderPTProfile":     "X-PinchTab-Profile-Name",
	"HeaderPTTabID":       "X-PinchTab-Tab-Id",
	"HeaderPTTabCreated":  "X-PinchTab-Tab-Created",
	"InternalTokenHeader": internalTokenHeaderValue,
}

// publicClientPackages originate outbound requests to a public PinchTab listener (bearer or
// session auth, not orchestrator-proxied). An X-PinchTab-* request header they set is dropped
// by the ingress strip layer unless the request also carries the internal token, so setting
// one is a silent no-op — the failure PIN-376 and PIN-384 both hit. The server, the identity
// helper (activity) and the trusted orchestrator hops are deliberately absent.
var publicClientPackages = map[string]string{
	"internal/cli/apiclient": "the CLI's HTTP transport",
	"internal/mcp":           "the MCP server's client to the front door",
	"cmd/pinchtab":           "CLI commands that build their own requests",
	"internal/scheduler":     "the scheduler's action executor posting to an instance",
}

// publicClientExemptHeaders are the (package, header) pairs a public client may still set:
// the CLI's Source is harmless because its bearer credential fallback records the same
// 'client' label, so a stripped Source changes nothing. MCP is deliberately NOT here — its
// Source set was removed (PIN-384), and the census forbids re-adding it.
var publicClientExemptHeaders = map[string]map[string]string{
	"internal/cli/apiclient": {"x-pinchtab-source": "harmless: bearer credential fallback records the same 'client' label"},
	"cmd/pinchtab":           {"x-pinchtab-source": "harmless: same as the CLI apiclient — bearer fallback yields 'client'"},
	// The scheduler's action executor routes hop auth through the orchestrator
	// (AuthorizeTabRequest), which marks internal/scheduler authorized below, so its
	// Source/Tab-Id survive ingress and need no exemption. The webhook headers stay exempt:
	// they go to an EXTERNAL receiver, not a PinchTab listener, and would be flagged if the
	// executor ever stopped routing through the authorizer.
	"internal/scheduler": {
		"x-pinchtab-event":   "webhook.go: outbound event-webhook to an EXTERNAL receiver that reads these; not a PinchTab listener behind the strip layer",
		"x-pinchtab-task-id": "webhook.go: outbound event-webhook to an EXTERNAL receiver that reads these; not a PinchTab listener behind the strip layer",
	},
}

func resolveHeaderName(arg ast.Expr) (string, bool) {
	switch a := arg.(type) {
	case *ast.BasicLit:
		if a.Kind == token.STRING {
			if v, err := strconv.Unquote(a.Value); err == nil {
				return v, true
			}
		}
	case *ast.Ident:
		if v, ok := knownHeaderConsts[a.Name]; ok {
			return v, true
		}
	case *ast.SelectorExpr:
		if v, ok := knownHeaderConsts[a.Sel.Name]; ok {
			return v, true
		}
	}
	return "", false
}

type headerSetSite struct{ pkg, header, site string }

// pinchtabRequestHeaderSets finds request-side `x.Header.Set(<X-PinchTab-* header>, …)` calls
// (field-access .Header, so `w.Header().Set` response writes are not counted) and, separately,
// the packages that set the internal token — a trusted hop whose headers survive ingress.
func pinchtabRequestHeaderSets(t *testing.T, files []srccensus.SourceFile) (sets []headerSetSite, tokenPkgs map[string]bool) {
	t.Helper()
	tokenPkgs = map[string]bool{}
	for _, f := range files {
		parsed, err := parser.ParseFile(token.NewFileSet(), f.Name, f.Text, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", f.Name, err)
		}
		pkg := path.Dir(f.Name)
		ast.Inspect(parsed, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			// A package that routes a request through the orchestrator's hop-auth owner is
			// authorized the same as one that sets the internal token itself: the token is
			// applied there, not here.
			if sel.Sel.Name == "AuthorizeTabRequest" {
				tokenPkgs[pkg] = true
				return true
			}
			if sel.Sel.Name != "Set" {
				return true
			}
			inner, ok := sel.X.(*ast.SelectorExpr)
			if !ok || inner.Sel.Name != "Header" {
				return true
			}
			if len(call.Args) == 0 {
				return true
			}
			name, ok := resolveHeaderName(call.Args[0])
			if !ok {
				return true
			}
			key := headerKey(name)
			if key == headerKey(internalTokenHeaderValue) {
				tokenPkgs[pkg] = true
				return true
			}
			if strings.HasPrefix(key, "x-pinchtab-") {
				sets = append(sets, headerSetSite{pkg: pkg, header: key, site: f.Name})
			}
			return true
		})
	}
	return sets, tokenPkgs
}

func strippedHeaderViolations(sets []headerSetSite, tokenPkgs map[string]bool) []string {
	var violations []string
	for _, s := range sets {
		if _, isClient := publicClientPackages[s.pkg]; !isClient {
			continue
		}
		if tokenPkgs[s.pkg] {
			continue
		}
		if hdrs, ok := publicClientExemptHeaders[s.pkg]; ok {
			if _, ok := hdrs[s.header]; ok {
				continue
			}
		}
		violations = append(violations, fmt.Sprintf("%s: %s sets request header %s, but the package sends no internal token, so ingress strips it (a silent no-op); carry the value in the body, send the internal token on a trusted hop, or exempt it with a reason", s.site, s.pkg, s.header))
	}
	sort.Strings(violations)
	return violations
}

func TestPublicClientsDoNotSetStrippedRequestHeaders(t *testing.T) {
	sets, tokenPkgs := pinchtabRequestHeaderSets(t, srccensus.Tree(t, moduleRoot, minModuleFiles))
	if v := strippedHeaderViolations(sets, tokenPkgs); len(v) > 0 {
		t.Errorf("public-client packages set X-PinchTab-* request headers the ingress strip layer drops:\n%s", strings.Join(v, "\n"))
	}
}

func TestStrippedHeaderCensusFlagsAPublicClientSet(t *testing.T) {
	planted := []srccensus.SourceFile{
		{Name: "internal/mcp/evil.go", Text: "package mcp\nimport \"net/http\"\nfunc f(req *http.Request) { req.Header.Set(\"X-PinchTab-Source\", \"mcp\") }\n"},
	}
	sets, tokenPkgs := pinchtabRequestHeaderSets(t, planted)
	if len(strippedHeaderViolations(sets, tokenPkgs)) == 0 {
		t.Fatal("a public client setting a stripped X-PinchTab-* request header passed the census")
	}
}

func TestStrippedHeaderCensusAllowsATrustedHopThatSendsTheToken(t *testing.T) {
	planted := []srccensus.SourceFile{
		{Name: "internal/scheduler/x.go", Text: "package scheduler\nimport \"net/http\"\nfunc f(req *http.Request) { req.Header.Set(\"X-PinchTab-Internal-Token\", \"s\"); req.Header.Set(\"X-PinchTab-Source\", \"scheduler\") }\n"},
	}
	sets, tokenPkgs := pinchtabRequestHeaderSets(t, planted)
	if len(strippedHeaderViolations(sets, tokenPkgs)) != 0 {
		t.Fatal("a package that sends the internal token was flagged; its headers survive ingress")
	}
}

// A package that routes its request through the orchestrator's hop-auth owner
// (AuthorizeTabRequest) is authorized the same as one that sends the token directly, so its
// X-PinchTab-* sets are not flagged.
func TestStrippedHeaderCensusAllowsAPackageThatRoutesThroughTheAuthorizer(t *testing.T) {
	planted := []srccensus.SourceFile{
		{Name: "internal/scheduler/x.go", Text: "package scheduler\nimport \"net/http\"\nfunc f(a interface{ AuthorizeTabRequest(string, *http.Request) error }, req *http.Request) {\n\t_ = a.AuthorizeTabRequest(\"t\", req)\n\treq.Header.Set(\"X-PinchTab-Source\", \"scheduler\")\n}\n"},
	}
	sets, tokenPkgs := pinchtabRequestHeaderSets(t, planted)
	if len(strippedHeaderViolations(sets, tokenPkgs)) != 0 {
		t.Fatal("a package that routes through AuthorizeTabRequest was flagged; its headers survive ingress via hop auth")
	}
}

// A response-side write (w.Header().Set) is not a request header the strip layer can drop.
func TestStrippedHeaderCensusIgnoresResponseWrites(t *testing.T) {
	planted := []srccensus.SourceFile{
		{Name: "internal/mcp/resp.go", Text: "package mcp\nimport \"net/http\"\nfunc f(w http.ResponseWriter) { w.Header().Set(\"X-PinchTab-Source\", \"mcp\") }\n"},
	}
	sets, _ := pinchtabRequestHeaderSets(t, planted)
	if len(sets) != 0 {
		t.Fatalf("a response-side w.Header().Set was counted as a request header: %v", sets)
	}
}
