package handlers

import (
	"context"
	"encoding/json"
	"go/ast"
	"go/token"
	"net/http"
	"net/http/httptest"
	"path"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/activity"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/srccensus"
	"github.com/pinchtab/semantic"
)

type vocabMockBridge struct {
	mockBridge
	refCache      *bridge.RefCache
	executeCalled bool
}

func (m *vocabMockBridge) GetRefCache(string) *bridge.RefCache { return m.refCache }

func (m *vocabMockBridge) ExecuteAction(ctx context.Context, kind string, req bridge.ActionRequest) (map[string]any, error) {
	m.executeCalled = true
	return map[string]any{"success": true}, nil
}

func newVocabHandlers(cacheToken string) (*Handlers, *vocabMockBridge) {
	mb := &vocabMockBridge{
		mockBridge: mockBridge{availableActions: []string{bridge.ActionClick}},
	}
	if cacheToken != "" {
		mb.refCache = &bridge.RefCache{DomEpoch: cacheToken, Refs: map[string]int64{"e1": 42}}
	}
	return New(mb, &config.RuntimeConfig{ActionTimeout: time.Second}, nil, nil, nil), mb
}

func postVocabAction(h *Handlers, body string, header string) (*httptest.ResponseRecorder, map[string]any) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/action", strings.NewReader(body))
	if header != "" {
		req.Header.Set(vocabHeader, header)
	}
	h.HandleAction(w, req)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w, out
}

// A ref carrying a token from an earlier snapshot, while the tab's current vocabulary is a
// different token, is the exact silent-wrong-click case: today it resolved e1 positionally
// against whichever snapshot ran last and reported success. It must now be refused loudly,
// before any node is touched, with its own code.
func TestARefFromASupersededVocabularyIsRefusedBeforeAnyNodeIsTouched(t *testing.T) {
	h, mb := newVocabHandlers("current")
	w, body := postVocabAction(h, `{"kind":"click","ref":"e1","tabId":"tab1","vocab":"stale"}`, "")

	if w.Code != 409 {
		t.Fatalf("status = %d, want 409 — a ref minted under a superseded vocabulary must be refused, not resolved positionally: %s", w.Code, w.Body.String())
	}
	if body["code"] != vocabSupersededCode {
		t.Errorf("code = %v, want %q", body["code"], vocabSupersededCode)
	}
	if body["retryable"] != true {
		t.Errorf("retryable = %v, want true — re-snapshotting and retrying with the fresh ref is exactly the fix", body["retryable"])
	}
	if mb.executeCalled {
		t.Error("the action reached ExecuteAction; the refusal must land before any node is touched")
	}
}

// The absence half: a ref echoed with the tab's CURRENT token is not the superseded case, so
// the guard must let it through to resolution rather than being satisfiable by refusing every
// ref. It is NOT refused with the vocab code (it fails downstream only because the mock cannot
// resolve e1, which is a different, later refusal).
func TestARefCarryingTheCurrentVocabularyIsNotRefusedByTheGuard(t *testing.T) {
	h, mb := newVocabHandlers("current")
	_, body := postVocabAction(h, `{"kind":"click","ref":"e1","tabId":"tab1","vocab":"current"}`, "")

	if body["code"] == vocabSupersededCode {
		t.Errorf("a ref used within its own vocabulary was refused as superseded: %v", body)
	}
	_ = mb
}

// Fail-safe permissiveness: an action carrying no token, or one hitting a tab whose cache
// carries no token, keeps today's behaviour so no existing client breaks. Each is asserted
// against the same stale/current setup so a guard that refused unconditionally would red here.
func TestTheGuardIsPermissiveWhenEitherSideHasNoToken(t *testing.T) {
	cases := []struct {
		name       string
		cacheToken string
		body       string
	}{
		{"no token on the request", "current", `{"kind":"click","ref":"e1","tabId":"tab1"}`},
		{"no cache at all", "", `{"kind":"click","ref":"e1","tabId":"tab1","vocab":"stale"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, _ := newVocabHandlers(tc.cacheToken)
			_, body := postVocabAction(h, tc.body, "")
			if body["code"] == vocabSupersededCode {
				t.Errorf("%s was refused as superseded; the guard must stay permissive when a token is absent", tc.name)
			}
		})
	}
}

// A stale token on an action that does not resolve through the ref cache — a coordinate click
// carries its own target — must not be blocked: scoping the guard to ref-targeted actions is
// what keeps ordinary non-ref flows working after a renumbering publish.
func TestAStaleTokenDoesNotBlockANonRefAction(t *testing.T) {
	h, mb := newVocabHandlers("current")
	w, body := postVocabAction(h, `{"kind":"click","x":10,"y":20,"tabId":"tab1","vocab":"stale"}`, "")

	if body["code"] == vocabSupersededCode {
		t.Fatalf("a coordinate click was refused as superseded though it never resolves a ref: %v", body)
	}
	if !mb.executeCalled {
		t.Errorf("the non-ref action did not reach ExecuteAction (status %d): %s", w.Code, w.Body.String())
	}
}

// The tag travels in the body, not a header: the front door strips every inbound
// X-PinchTab-* header from public clients, so a header tag would never reach the guard.
// Driven through TrustedInternalProxyStripMiddleware (the middleware that strips) to prove
// the body field survives where a header would not.
//
//	Row A — the tag equals the resolved tab: a superseded token is refused, so the implicit
//	  snap-then-click flow keeps its supersession protection.
//	Row B — the tag names a different tab than the one resolved (the current pointer moved
//	  off the snapshotted tab): the token is ignored, not refused, killing the mirror-case
//	  false 409. Pre-fix (header tag, or no tag) this answered 409.
func TestVocabTabTravelsInTheBodyThroughTheStripMiddleware(t *testing.T) {
	serve := func(h *Handlers, body string) (int, map[string]any) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/action", strings.NewReader(body))
		TrustedInternalProxyStripMiddleware("")(http.HandlerFunc(h.HandleAction)).ServeHTTP(w, req)
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}

	hA, _ := newVocabHandlers("current")
	code, body := serve(hA, `{"kind":"click","ref":"e1","tabId":"tab1","vocab":"stale","vocabTab":"tab1"}`)
	if code != 409 || body["code"] != vocabSupersededCode {
		t.Fatalf("row A: a superseded token tagged for the resolved tab was not refused through the strip chain: status %d body %v", code, body)
	}

	hB, _ := newVocabHandlers("current")
	_, body = serve(hB, `{"kind":"click","ref":"e1","tabId":"tab1","vocab":"stale","vocabTab":"other-tab"}`)
	if body["code"] == vocabSupersededCode {
		t.Fatalf("row B: a token tagged for a moved-off tab produced a false supersession through the strip chain: %v", body)
	}
}

// The token rides back in a response header, so a client that read it from the header — not
// the JSON body — echoes it the same way. The guard reads the header when the request carried
// no explicit field.
func TestTheGuardReadsTheTokenFromTheRequestHeader(t *testing.T) {
	h, _ := newVocabHandlers("current")
	w, body := postVocabAction(h, `{"kind":"click","ref":"e1","tabId":"tab1"}`, "stale")
	if w.Code != 409 || body["code"] != vocabSupersededCode {
		t.Errorf("a superseded token sent as a header was not refused: status %d body %s", w.Code, w.Body.String())
	}
}

// The GET form: the token is a query parameter, and the unknown-parameter guard must ALLOW
// it (it is derived from ActionRequest's fields, so `vocab` is accepted with no edit) while
// still enforcing the mismatch.
func TestTheGETFormCarriesAndEnforcesTheToken(t *testing.T) {
	h, _ := newVocabHandlers("current")
	w := httptest.NewRecorder()
	h.HandleAction(w, httptest.NewRequest("GET", "/action?kind=click&ref=e1&tabId=tab1&vocab=stale", nil))
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if w.Code != 409 || body["code"] != vocabSupersededCode {
		t.Errorf("the GET form did not enforce a superseded token (status %d): %s", w.Code, w.Body.String())
	}
}

// A new epoch mints a fresh token, so a publish that renumbers a tab's refs (a new document)
// supersedes the previous one. A same-document republish does NOT mint a fresh token — EpochRefs
// carries the prior epoch's token — which is what stops the annotate and stale-ref-retry paths
// from superseding a ref the agent still holds. This test only pins that the mint, when it does
// happen, yields a distinct non-empty token.
func TestEachMintedTokenIsDistinctAndNonEmpty(t *testing.T) {
	seen := map[string]bool{}
	for range 64 {
		tok := bridge.MintVocabToken()
		if tok == "" {
			t.Fatal("MintVocabToken returned an empty token; an empty cache token disables the guard")
		}
		if seen[tok] {
			t.Fatalf("MintVocabToken repeated %q; two publishes would share a vocabulary and the guard would miss the supersession", tok)
		}
		seen[tok] = true
	}
}

// Wiring census: the epoch ref book is what makes a ref denote a node across reads, and it also
// owns the vocabulary token — carrying it across a same-document republish and minting a fresh one
// only for a new document. Every ref-publishing site must route through bridge.EpochRefs, or that
// site reverts to a bare cache: a hand-rolled cache would either mint a fresh token on every
// republish (superseding refs the agent still holds — the exact regression criteria three and five
// forbid) or renumber refs positionally. This is the browserless proof for those criteria, since
// the handlers reach live CDP and cannot be driven by the package mock. Dropping the EpochRefs call
// at any site reddens here while leaving the behavioural gate tests green, so this census is
// load-bearing.
func TestEveryRefPublishSiteRoutesThroughTheEpochRefBook(t *testing.T) {
	pkg := srccensus.Load(t, ".", handlersSourceFileFloor)
	for _, tc := range []struct {
		fn  string
		why string
	}{
		{"HandleSnapshot", "the /snapshot response is the primary vocabulary a client reads and echoes"},
		{"HandleCapture", "the /capture response publishes the same tab vocabulary as /snapshot"},
		{"collectScreenshotAnnotations", "the annotate path republishes with a hardcoded interactive filter and must not supersede an existing ref"},
		{"refreshRefCache", "the stale-ref retry refreshes the cache and must not supersede a ref minted before it"},
	} {
		fn, file := handlerFuncDecl(t, pkg, tc.fn)
		if !callsSelector(fn, "EpochRefs") {
			t.Errorf("%s (%s) does not call bridge.EpochRefs, so this publish site does not route through the epoch ref book — %s", tc.fn, file, tc.why)
		}
	}

	snapshot, _ := handlerFuncDecl(t, pkg, "HandleSnapshot")
	if !callsAnyIdent(snapshot, "publishVocab") {
		t.Error("HandleSnapshot does not emit the token in a response header, so a client reading a compact/text snapshot cannot obtain it")
	}
	if snapshotJSON, _ := handlerFuncDecl(t, pkg, "writeSnapshot"); !mentionsString(snapshotJSON, "vocabularyToken") {
		t.Error("HandleSnapshot does not carry the token in its JSON body, so a JSON snapshot consumer cannot echo it")
	}
	capture, _ := handlerFuncDecl(t, pkg, "HandleCapture")
	if !callsAnyIdent(capture, "publishVocab") {
		t.Error("HandleCapture does not emit the token in a response header, so a /capture client cannot obtain it uniformly with /snapshot")
	}
}

func TestEveryReEpochSitePublishesTheVocabularyOnItsResponse(t *testing.T) {
	publishers := map[string][]string{
		"snapshot.go":            {"snapshot.go"},
		"capture.go":             {"capture.go"},
		"extract.go":             {"extract.go"},
		"a11y_axe.go":            {"a11y_axe.go"},
		"find.go":                {"find.go", "actions.go"},
		"screenshot_annotate.go": {"screenshot.go", "annotate.go"},
		"action_execution.go":    {"actions.go"},
		"handlers.go":            {"actions.go"},
		"actions.go":             {"actions.go"},
	}
	publishHelpers := []string{"publishVocab", "publishTabVocab", "publishVocabIfReepoched"}

	pkg := srccensus.Load(t, ".", handlersSourceFileFloor)
	_, files := pkg.Syntax()
	for file, parsed := range files {
		if !callsSelector(parsed, "EpochRefs") && !callsSelector(parsed, "refreshRefCache") {
			continue
		}
		answering, ok := publishers[file]
		if !ok {
			t.Errorf("%s re-epochs a tab's refs but no response is recorded as publishing the vocabulary for it; publish through publishVocab and add a row", file)
			continue
		}
		for _, pub := range answering {
			pubFile, ok := files[pub]
			if !ok {
				t.Errorf("%s is recorded as answering for the re-epoch in %s but is not a source file in %s; re-point the row at wherever that response moved", pub, file, pkg.Dir())
				continue
			}
			if !callsAnyIdent(pubFile, publishHelpers...) {
				t.Errorf("%s answers for the re-epoch in %s but never calls a vocab publish helper, so its response leaves the client echoing a superseded token", pub, file)
			}
		}
	}

	writer, writerFile := handlerFuncDecl(t, pkg, "publishVocab")
	for file, parsed := range files {
		if file == writerFile {
			continue
		}
		if setsHeader(parsed, "vocabHeader") {
			t.Errorf("%s sets the vocab header directly; publication goes through publishVocab so every publisher also names the tab the token belongs to", file)
		}
	}
	if !setsHeader(writer, "vocabHeader") {
		t.Errorf("publishVocab (%s) no longer sets vocabHeader, so exempting its file from the direct-set ban excuses nothing; re-point this guard at whatever writes the header now", writerFile)
	}

	graph := handlerCallGraph(files)
	reEpochs := []string{"bridge.EpochRefs", "refreshRefCache"}
	reachingHandlers := 0
	for name := range graph.handlers {
		if !graph.reaches(name, reEpochs...) {
			continue
		}
		reachingHandlers++
		if !graph.reaches(name, publishHelpers...) {
			t.Errorf("%s can re-epoch a tab's refs (%s) but its response never reaches a vocab publish helper, so the client's next ref action is refused vocab_superseded", name, strings.Join(graph.pathTo(name, reEpochs...), " -> "))
		}
	}
	if reachingHandlers < 20 {
		t.Errorf("only %d handlers reach a re-epoch site; the call graph has lost the semantic-selector paths (count, attr, actions, macro …)", reachingHandlers)
	}
}

type packageCallGraph struct {
	refs     map[string]map[string]bool
	handlers map[string]bool
}

func handlerCallGraph(files map[string]*ast.File) packageCallGraph {
	g := packageCallGraph{refs: map[string]map[string]bool{}, handlers: map[string]bool{}}
	for _, f := range files {
		imported := map[string]bool{}
		for _, spec := range f.Imports {
			p, _ := strconv.Unquote(spec.Path.Value)
			name := path.Base(p)
			if spec.Name != nil {
				name = spec.Name.Name
			}
			imported[name] = true
		}
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			name := fd.Name.Name
			if fd.Recv != nil && strings.HasPrefix(name, "Handle") {
				g.handlers[name] = true
			}
			if g.refs[name] == nil {
				g.refs[name] = map[string]bool{}
			}
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.SelectorExpr:
					if id, ok := x.X.(*ast.Ident); ok && imported[id.Name] {
						g.refs[name][id.Name+"."+x.Sel.Name] = true
					} else {
						g.refs[name][x.Sel.Name] = true
					}
				case *ast.CallExpr:
					fun := x.Fun
					if ix, ok := fun.(*ast.IndexExpr); ok {
						fun = ix.X
					}
					if id, ok := fun.(*ast.Ident); ok {
						g.refs[name][id.Name] = true
					}
				}
				return true
			})
		}
	}
	return g
}

func (g packageCallGraph) reaches(from string, targets ...string) bool {
	return g.pathTo(from, targets...) != nil
}

func (g packageCallGraph) pathTo(from string, targets ...string) []string {
	seen := map[string]bool{from: true}
	var walk func(string) []string
	walk = func(name string) []string {
		if slices.Contains(targets, name) {
			return []string{name}
		}
		for next := range g.refs[name] {
			if seen[next] {
				continue
			}
			seen[next] = true
			if rest := walk(next); rest != nil {
				return append([]string{name}, rest...)
			}
		}
		return nil
	}
	return walk(from)
}

func handlerFuncDecl(t *testing.T, pkg *srccensus.Package, name string) (*ast.FuncDecl, string) {
	t.Helper()
	fn, ok := pkg.Func(name)
	if !ok {
		t.Fatalf("%s is no longer declared anywhere in %s; re-point this guard at whatever replaced it rather than deleting it", name, pkg.Dir())
	}
	_, files := pkg.Syntax()
	for _, decl := range files[fn.File].Decls {
		if fd, ok := decl.(*ast.FuncDecl); ok && fd.Name.Name == name {
			return fd, fn.File
		}
	}
	t.Fatalf("srccensus located %s in %s but its declaration is not among that file's decls", name, fn.File)
	return nil, ""
}

func callsAnyIdent(scope ast.Node, idents ...string) bool {
	found := false
	ast.Inspect(scope, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		var got string
		switch fn := call.Fun.(type) {
		case *ast.Ident:
			got = fn.Name
		case *ast.SelectorExpr:
			got = fn.Sel.Name
		}
		for _, id := range idents {
			if got == id {
				found = true
			}
		}
		return true
	})
	return found
}

func setsHeader(scope ast.Node, headerIdent string) bool {
	found := false
	ast.Inspect(scope, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		if se, ok := call.Fun.(*ast.SelectorExpr); !ok || se.Sel.Name != "Set" {
			return true
		}
		if id, ok := call.Args[0].(*ast.Ident); ok && id.Name == headerIdent {
			found = true
		}
		return true
	})
	return found
}

func callsSelector(scope ast.Node, sel string) bool {
	found := false
	ast.Inspect(scope, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if se, ok := call.Fun.(*ast.SelectorExpr); ok && se.Sel.Name == sel {
				found = true
			}
		}
		return true
	})
	return found
}

func mentionsString(scope ast.Node, lit string) bool {
	found := false
	ast.Inspect(scope, func(n ast.Node) bool {
		if bl, ok := n.(*ast.BasicLit); ok && bl.Kind == token.STRING && strings.Trim(bl.Value, "`\"") == lit {
			found = true
		}
		return true
	})
	return found
}

type reepochingVocabBridge struct {
	vocabMockBridge
	next *bridge.RefCache
}

func (m *reepochingVocabBridge) ExecuteAction(ctx context.Context, kind string, req bridge.ActionRequest) (map[string]any, error) {
	if m.next != nil {
		m.refCache = m.next
	}
	return m.vocabMockBridge.ExecuteAction(ctx, kind, req)
}

func TestAnActionThatReEpochsTheTabPublishesTheNewVocabulary(t *testing.T) {
	for _, tc := range []struct {
		name string
		next *bridge.RefCache
		want string
	}{
		{"re-epoched", &bridge.RefCache{DomEpoch: "fresh"}, "fresh"},
		{"untouched", nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mb := &reepochingVocabBridge{
				vocabMockBridge: vocabMockBridge{
					mockBridge: mockBridge{availableActions: []string{bridge.ActionClick}},
					refCache:   &bridge.RefCache{DomEpoch: "current"},
				},
				next: tc.next,
			}
			h := New(mb, &config.RuntimeConfig{ActionTimeout: time.Second}, nil, nil, nil)
			w, _ := postVocabAction(h, `{"kind":"click","x":10,"y":20,"tabId":"tab1"}`, "")
			if !mb.executeCalled {
				t.Fatalf("the action did not reach ExecuteAction (status %d): %s", w.Code, w.Body.String())
			}
			if got := w.Header().Get(vocabHeader); got != tc.want {
				t.Errorf("vocab header = %q, want %q", got, tc.want)
			}
		})
	}
}

type autoRefreshFindBridge struct {
	findMockBridge
	fresh *bridge.RefCache
	reads int
}

func (m *autoRefreshFindBridge) GetRefCache(string) *bridge.RefCache {
	m.reads++
	if m.reads == 1 {
		return nil
	}
	return m.fresh
}

func TestFindPublishesTheVocabularyItsRefreshMinted(t *testing.T) {
	h, _, tabID := newDetailsDialogHandlers(t)

	w := httptest.NewRecorder()
	h.HandleFind(w, httptest.NewRequest("POST", "/find", strings.NewReader(`{"query":"Details","tabId":"`+tabID+`"}`)))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	cache := h.Bridge.GetRefCache(tabID)
	if cache == nil || cache.DomEpoch == "" {
		t.Fatalf("find on a tab with no snapshot left no epoch in the ref cache: %+v", cache)
	}
	if got := w.Header().Get(vocabHeader); got != cache.DomEpoch {
		t.Errorf("vocab header = %q, want the token the refresh minted (%q)", got, cache.DomEpoch)
	}
	if got := w.Header().Get(activity.HeaderPTTabID); got != tabID {
		t.Errorf("tab header = %q, want %s so a client keys the token to the resolved tab", got, tabID)
	}
}

func TestAMultiStepRunThatReEpochsTheTabPublishesTheNewVocabulary(t *testing.T) {
	for _, surface := range []struct {
		name  string
		path  string
		body  string
		serve func(*Handlers) http.HandlerFunc
	}{
		{"batch", "/actions", `{"tabId":"tab1","actions":[{"kind":"click","x":10,"y":20}]}`, func(h *Handlers) http.HandlerFunc { return h.HandleActions }},
		{"macro", "/macro", `{"tabId":"tab1","steps":[{"kind":"click","x":10,"y":20}]}`, func(h *Handlers) http.HandlerFunc { return h.HandleMacro }},
	} {
		for _, tc := range []struct {
			name string
			next *bridge.RefCache
			want string
		}{
			{"re-epoched", &bridge.RefCache{DomEpoch: "fresh"}, "fresh"},
			{"untouched", nil, ""},
		} {
			t.Run(surface.name+"/"+tc.name, func(t *testing.T) {
				mb := &reepochingVocabBridge{
					vocabMockBridge: vocabMockBridge{
						mockBridge: mockBridge{availableActions: []string{bridge.ActionClick}},
						refCache:   &bridge.RefCache{DomEpoch: "current"},
					},
					next: tc.next,
				}
				h := New(mb, &config.RuntimeConfig{ActionTimeout: time.Second, AllowMacro: true}, nil, nil, nil)
				w := httptest.NewRecorder()
				surface.serve(h)(w, httptest.NewRequest("POST", surface.path, strings.NewReader(surface.body)))
				if !mb.executeCalled {
					t.Fatalf("the step did not reach ExecuteAction (status %d): %s", w.Code, w.Body.String())
				}
				if got := w.Header().Get(vocabHeader); got != tc.want {
					t.Errorf("vocab header = %q, want %q", got, tc.want)
				}
				if tc.want != "" && w.Header().Get(activity.HeaderPTTabID) != "tab1" {
					t.Errorf("tab header = %q, want tab1 so a client keys the token to the tab", w.Header().Get(activity.HeaderPTTabID))
				}
			})
		}
	}
}

func TestASemanticElementReadPublishesOnlyTheVocabularyItsResolutionMinted(t *testing.T) {
	fresh := &bridge.RefCache{
		DomEpoch: "ep_fresh",
		Nodes:    []bridge.A11yNode{{Ref: "e11", Role: "link", Name: "Go page 2"}},
		Refs:     map[string]int64{"e11": 11},
	}
	for _, tc := range []struct {
		name   string
		bridge bridge.BridgeAPI
		want   string
	}{
		{"auto-refreshed", &autoRefreshFindBridge{fresh: fresh}, "ep_fresh"},
		{"cache already current", &findMockBridge{refCache: fresh}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := New(tc.bridge, &config.RuntimeConfig{ActionTimeout: 10 * time.Second}, nil, nil, nil)
			h.Matcher = semantic.NewLexicalMatcher()
			w := httptest.NewRecorder()
			h.HandleCount(w, httptest.NewRequest("GET", "/count?selector=find:Go%20page%202", nil))
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d: %s", w.Code, w.Body.String())
			}
			if got := w.Header().Get(vocabHeader); got != tc.want {
				t.Errorf("vocab header = %q, want %q", got, tc.want)
			}
		})
	}
}
