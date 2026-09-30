package handlers

import (
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/browsers/ghostchrome/bridgekit"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/srccensus"
)

type optionalCapabilitySite struct {
	pos     string
	iface   string
	methods []string
	raw     bool
}

func parseHandlerSources(t *testing.T, extra map[string]string) (*token.FileSet, []*ast.File) {
	t.Helper()
	fset, parsed := srccensus.Load(t, ".", 50).Syntax()
	files := make([]*ast.File, 0, len(parsed)+len(extra))
	for _, name := range slices.Sorted(maps.Keys(parsed)) {
		files = append(files, parsed[name])
	}
	for name, src := range extra {
		file, err := parser.ParseFile(fset, name, src, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		files = append(files, file)
	}
	return fset, files
}

func interfaceDecls(files []*ast.File) map[string]*ast.InterfaceType {
	decls := map[string]*ast.InterfaceType{}
	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			if spec, ok := n.(*ast.TypeSpec); ok {
				if iface, ok := spec.Type.(*ast.InterfaceType); ok {
					decls[spec.Name.Name] = iface
				}
			}
			return true
		})
	}
	return decls
}

func interfaceMethods(t *testing.T, expr ast.Expr, decls map[string]*ast.InterfaceType) (string, []string) {
	t.Helper()
	var iface *ast.InterfaceType
	name := "interface{...}"
	switch e := expr.(type) {
	case *ast.Ident:
		name = e.Name
		iface = decls[e.Name]
	case *ast.InterfaceType:
		iface = e
	}
	if iface == nil {
		t.Fatalf("optional capability %s is not an interface declared in package handlers; the census cannot read its methods", name)
	}
	var methods []string
	for _, field := range iface.Methods.List {
		if len(field.Names) == 0 {
			_, embedded := interfaceMethods(t, field.Type, decls)
			methods = append(methods, embedded...)
			continue
		}
		for _, n := range field.Names {
			methods = append(methods, n.Name)
		}
	}
	sort.Strings(methods)
	return name, methods
}

func isBridgeSelector(e ast.Expr) bool {
	sel, ok := e.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "Bridge"
}

func bridgeAliases(file *ast.File) map[string]bool {
	aliases := map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		switch s := n.(type) {
		case *ast.AssignStmt:
			if len(s.Lhs) != len(s.Rhs) {
				break
			}
			for i, rhs := range s.Rhs {
				if ident, ok := s.Lhs[i].(*ast.Ident); ok && isBridgeSelector(rhs) {
					aliases[ident.Name] = true
				}
			}
		case *ast.ValueSpec:
			for i, v := range s.Values {
				if i < len(s.Names) && isBridgeSelector(v) {
					aliases[s.Names[i].Name] = true
				}
			}
		}
		return true
	})
	return aliases
}

func isBridgeValue(e ast.Expr, aliases map[string]bool) bool {
	if ident, ok := e.(*ast.Ident); ok {
		return aliases[ident.Name]
	}
	return isBridgeSelector(e)
}

func typeSwitchSubject(s *ast.TypeSwitchStmt) ast.Expr {
	switch a := s.Assign.(type) {
	case *ast.ExprStmt:
		if ta, ok := a.X.(*ast.TypeAssertExpr); ok {
			return ta.X
		}
	case *ast.AssignStmt:
		if len(a.Rhs) == 1 {
			if ta, ok := a.Rhs[0].(*ast.TypeAssertExpr); ok {
				return ta.X
			}
		}
	}
	return nil
}

func optionalCapabilitySites(t *testing.T, fset *token.FileSet, files []*ast.File) []optionalCapabilitySite {
	t.Helper()
	decls := interfaceDecls(files)
	var sites []optionalCapabilitySite
	add := func(n ast.Node, typ ast.Expr, raw bool) {
		name, methods := interfaceMethods(t, typ, decls)
		sites = append(sites, optionalCapabilitySite{pos: fset.Position(n.Pos()).String(), iface: name, methods: methods, raw: raw})
	}
	for _, file := range files {
		aliases := bridgeAliases(file)
		ast.Inspect(file, func(n ast.Node) bool {
			switch e := n.(type) {
			case *ast.TypeAssertExpr:
				if e.Type != nil && isBridgeValue(e.X, aliases) {
					add(e, e.Type, true)
				}
			case *ast.TypeSwitchStmt:
				if subject := typeSwitchSubject(e); subject != nil && isBridgeValue(subject, aliases) {
					for _, stmt := range e.Body.List {
						for _, typ := range stmt.(*ast.CaseClause).List {
							if ident, ok := typ.(*ast.Ident); ok && ident.Name == "nil" {
								continue
							}
							add(typ, typ, true)
						}
					}
				}
			case *ast.CallExpr:
				if idx, ok := e.Fun.(*ast.IndexExpr); ok {
					if fn, ok := idx.X.(*ast.Ident); ok && fn.Name == "bridgeAs" {
						add(e, idx.Index, false)
					}
				}
			}
			return true
		})
	}
	return sites
}

func ghostChromeAdapterOverRealBridge(t *testing.T) bridge.BridgeAPI {
	t.Helper()
	ctx, cancel := chromedp.NewContext(context.Background())
	t.Cleanup(cancel)
	cfg := &config.RuntimeConfig{StateDir: t.TempDir()}
	return bridgekit.NewBridgeAdapter(bridge.New(context.Background(), ctx, cfg), cfg)
}

func reachableThroughUnwrap(b bridge.BridgeAPI, methods []string) bool {
	for b != nil {
		v := reflect.ValueOf(b)
		all := true
		for _, m := range methods {
			if !v.MethodByName(m).IsValid() {
				all = false
				break
			}
		}
		if all {
			return true
		}
		wrapper, ok := b.(bridgeUnwrapper)
		if !ok {
			return false
		}
		b = wrapper.Unwrap()
	}
	return false
}

func unreachableCapabilities(sites []optionalCapabilitySite, b bridge.BridgeAPI) []string {
	var missing []string
	for _, site := range sites {
		if !reachableThroughUnwrap(b, site.methods) {
			missing = append(missing, site.pos+" "+site.iface+" "+strings.Join(site.methods, ","))
		}
	}
	return missing
}

func TestEveryOptionalBridgeCapabilityIsReachableUnderGhostChrome(t *testing.T) {
	fset, files := parseHandlerSources(t, nil)
	sites := optionalCapabilitySites(t, fset, files)
	if len(sites) < 17 {
		t.Fatalf("census found %d optional-capability lookups on h.Bridge, want at least the 17 known; it would pass vacuously", len(sites))
	}

	for _, missing := range unreachableCapabilities(sites, ghostChromeAdapterOverRealBridge(t)) {
		t.Errorf("under the ghost-chrome adapter the handlers cannot reach %s", missing)
	}
}

func TestOptionalBridgeCapabilitiesAreLookedUpThroughBridgeAs(t *testing.T) {
	fset, files := parseHandlerSources(t, nil)
	for _, site := range optionalCapabilitySites(t, fset, files) {
		if site.raw {
			t.Errorf("%s asserts h.Bridge.(%s) directly, which never looks past a decorator; use bridgeAs[%s](h.Bridge)", site.pos, site.iface, site.iface)
		}
	}
}

func TestTheCapabilityCensusCoversAPlantedInterface(t *testing.T) {
	const planted = `package handlers
type plantedCapability interface {
	PlantedMethodNoBridgeHas()
}
func (h *Handlers) planted() {
	if c, ok := bridgeAs[plantedCapability](h.Bridge); ok {
		c.PlantedMethodNoBridgeHas()
	}
	_, _ = h.Bridge.(interface{ RecordTabScope(string, string, bool) })
}
func (hh *Handlers) plantedReceiver() {
	_, _ = hh.Bridge.(interface{ RecordTabScope(string, string, bool) })
}
func (h *Handlers) plantedAlias() {
	b := h.Bridge
	_, _ = b.(interface{ RecordTabScope(string, string, bool) })
}
func (h *Handlers) plantedSwitch() {
	switch h.Bridge.(type) {
	case nil:
	case interface{ RecordTabScope(string, string, bool) }:
	}
}
`
	fset, files := parseHandlerSources(t, map[string]string{"planted.go": planted})
	var plantedSites []optionalCapabilitySite
	for _, site := range optionalCapabilitySites(t, fset, files) {
		if strings.HasPrefix(site.pos, "planted.go") {
			plantedSites = append(plantedSites, site)
		}
	}
	if len(plantedSites) != 5 || plantedSites[0].raw {
		t.Fatalf("planted sites = %+v, want the bridgeAs lookup and four raw assertions", plantedSites)
	}
	for _, site := range plantedSites[1:] {
		if !site.raw {
			t.Fatalf("planted site %+v was not flagged as a raw assertion", site)
		}
	}

	missing := unreachableCapabilities(plantedSites, ghostChromeAdapterOverRealBridge(t))

	if len(missing) != 1 || !strings.Contains(missing[0], "plantedCapability") {
		t.Fatalf("unreachable = %v, want only the planted capability", missing)
	}
}

func TestBridgeAsPrefersTheDecoratorAndFallsBackToWhatItWraps(t *testing.T) {
	adapter := ghostChromeAdapterOverRealBridge(t)

	translating, ok := bridgeAs[interface{ FingerprintRotateActive(string) bool }](adapter)
	if !ok || any(translating) != any(adapter) {
		t.Fatal("the adapter's own translating FingerprintRotateActive must win over the wrapped bridge's")
	}
	tracker, ok := bridgeAs[tabScopeTracker](adapter)
	if !ok {
		t.Fatal("RecordTabScope is not reachable through the adapter")
	}
	if _, isBridge := tracker.(*bridge.Bridge); !isBridge {
		t.Fatalf("tabScopeTracker resolved to %T, want the wrapped *bridge.Bridge", tracker)
	}
	if _, ok := bridgeAs[tabScopeTracker](nil); ok {
		t.Fatal("a nil bridge must offer no capability")
	}
}

func TestUnderGhostChromeAnEndedSessionClosesItsTabAndTheListingLeadsWithTheCurrentTab(t *testing.T) {
	f := newSessionTabsFixture(t)
	f.b.Config.NavigateTimeout = 20 * time.Second
	f.h = New(bridgekit.NewBridgeAdapter(f.b, f.b.Config), f.b.Config, nil, nil, nil)
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<title>page</title>"))
	}))
	t.Cleanup(page.Close)
	openAt := func(sessionID string, trusted bool) string {
		rec := f.serve(f.h.HandleTab, "/tab", `{"action":"new","url":"`+page.URL+`"}`, sessionID, trusted)
		var out struct {
			TabID string `json:"tabId"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.TabID == "" {
			t.Fatalf("new tab answered %d: %s", rec.Code, rec.Body.String())
		}
		return out.TabID
	}
	own := openAt("sess_s", true)
	first := openAt("", false)
	second := openAt("", false)
	if rec := f.serve(f.h.HandleTab, "/tab", `{"action":"focus","tabId":"`+first+`"}`, "", false); rec.Code != http.StatusOK {
		t.Fatalf("focus answered %d: %s", rec.Code, rec.Body.String())
	}

	w := httptest.NewRecorder()
	f.h.HandleTabs(w, httptest.NewRequest(http.MethodGet, "/tabs", nil))
	var listing struct {
		Tabs []struct {
			ID string `json:"id"`
		} `json:"tabs"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listing); err != nil || len(listing.Tabs) == 0 {
		t.Fatalf("GET /tabs = %d %s", w.Code, w.Body.String())
	}
	if listing.Tabs[0].ID != first {
		t.Fatalf("GET /tabs through the adapter leads with %s, want the focused tab %s (second was %s)", listing.Tabs[0].ID, first, second)
	}

	closed := f.closeFor("sess_s")
	if len(closed.Closed) != 1 || closed.Closed[0] != own {
		t.Fatalf("session end through the adapter closed %v, want [%s]", closed.Closed, own)
	}
	if f.open(own) {
		t.Fatalf("the ended session's tab %s is still open", own)
	}
}
