package httpx

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/srccensus"
)

func TestCopyProxiedResponseHeadersLeavesTheOuterChainsHeadersSingleValued(t *testing.T) {
	owned := OuterChainResponseHeaders()
	if len(owned) < 4 {
		t.Fatalf("owned header set = %v, want at least the request id and the three security headers", owned)
	}

	dst := http.Header{}
	for _, name := range owned {
		dst.Set(name, "outer-"+name)
	}
	src := http.Header{"Content-Type": {"application/json"}}
	for _, name := range owned {
		src.Set(name, "upstream-"+name)
	}

	CopyProxiedResponseHeaders(dst, src)

	for _, name := range owned {
		if got := dst.Values(name); len(got) != 1 {
			t.Errorf("%s = %v, want exactly one value: a caller cannot tell which of two request ids the outer process logged", name, got)
		} else if got[0] != "outer-"+name {
			t.Errorf("%s = %q, want the outer chain's own value", name, got[0])
		}
	}
	if got := dst.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want the upstream header copied through", got)
	}
}

func TestCopyProxiedResponseHeadersMatchesOwnedHeadersCaseInsensitively(t *testing.T) {
	dst := http.Header{}
	dst.Set(RequestIDHeader, "outer")

	src := http.Header{}
	src["x-request-id"] = []string{"upstream"}

	CopyProxiedResponseHeaders(dst, src)

	if got := dst.Values(RequestIDHeader); len(got) != 1 || got[0] != "outer" {
		t.Errorf("%s = %v, want the outer value alone — an upstream spelling the canonicaliser did not touch must still be filtered", RequestIDHeader, got)
	}
}

func TestCopyProxiedResponseHeadersDropsHopByHopHeaders(t *testing.T) {
	dst := http.Header{}
	src := http.Header{
		"Transfer-Encoding": {"chunked"},
		"Connection":        {"keep-alive"},
		"X-Custom":          {"kept"},
	}

	CopyProxiedResponseHeaders(dst, src)

	for _, name := range []string{"Transfer-Encoding", "Connection"} {
		if got := dst.Get(name); got != "" {
			t.Errorf("%s = %q, want it dropped: an upstream framing header is not the outer response's to republish", name, got)
		}
	}
	if got := dst.Get("X-Custom"); got != "kept" {
		t.Errorf("X-Custom = %q, want the upstream header copied through", got)
	}
}

func TestOwnedResponseHeadersAreDistinctAndCanonical(t *testing.T) {
	seen := map[string]bool{}
	for _, name := range OuterChainResponseHeaders() {
		lower := strings.ToLower(name)
		if seen[lower] {
			t.Errorf("%q is listed twice in the owned header set", name)
		}
		seen[lower] = true
		if name != http.CanonicalHeaderKey(name) {
			t.Errorf("%q is not the canonical spelling %q, so a census comparing it to a source literal misses", name, http.CanonicalHeaderKey(name))
		}
	}
}

func TestOuterChainResponseHeadersCannotBeMutatedByCallers(t *testing.T) {
	got := OuterChainResponseHeaders()
	got[0] = "X-Not-Owned"

	if !OuterChainOwnsResponseHeader(RequestIDHeader) {
		t.Fatalf("a caller editing the returned slice changed the owned set, so one proxy hop could stop filtering %s", RequestIDHeader)
	}
}

// This reached three copy sites at once because each grew its own copy loop. The rule is
// therefore about the SHAPE: a response header added under a key the code does not name is
// by construction republishing somebody else's header set, which is what makes the outer
// chain's headers multi-valued. A literal key — Vary: Origin — is a deliberate append and
// stays legal.
func TestNoProxyHopAddsResponseHeadersUnderAKeyItDoesNotName(t *testing.T) {
	files := srccensus.Tree(t, filepath.Join("..", ".."), 100)

	helperCalls := 0
	for _, file := range files {
		parsed, err := parser.ParseFile(token.NewFileSet(), file.Name, file.Text, 0)
		if err != nil {
			t.Errorf("%s: cannot parse, so this census can neither clear nor flag it: %v", file.Name, err)
			continue
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if sel.Sel.Name == "CopyProxiedResponseHeaders" {
				helperCalls++
				return true
			}
			if sel.Sel.Name != "Add" || len(call.Args) == 0 || !isHeaderAccessorCall(sel.X) {
				return true
			}
			if _, literal := call.Args[0].(*ast.BasicLit); literal {
				return true
			}
			t.Errorf("%s adds a response header under a variable key instead of calling httpx.CopyProxiedResponseHeaders; the outer chain has already set its request id and security headers on this response, so appending an upstream copy makes them multi-valued and the second request id appears in no log", file.Name)
			return true
		})
	}

	if helperCalls < 3 {
		t.Fatalf("found %d call(s) to CopyProxiedResponseHeaders in the whole module, want at least the known proxy hops; the helper has been inlined again, or this census has stopped seeing the module", helperCalls)
	}
}

// isHeaderAccessorCall reports whether expr is a Header() call — w.Header().Add(...) — as
// opposed to a plain Header field, which is how the REQUEST direction is built and is not
// this rule's subject.
func isHeaderAccessorCall(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "Header"
}
