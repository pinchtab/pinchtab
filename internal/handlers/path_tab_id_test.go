package handlers

import (
	"bytes"
	"go/ast"
	"go/token"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/srccensus"
)

func pathTabHandlers(t *testing.T) *Handlers {
	t.Helper()
	return New(&mockBridge{}, &config.RuntimeConfig{ActionTimeout: time.Second, AllowNetworkIntercept: true, AllowDownload: true, AllowCookies: true}, nil, nil, nil)
}

var pathTabRoutes = []struct {
	name   string
	method string
	call   func(h *Handlers, w http.ResponseWriter, r *http.Request)
}{
	{"network route list", http.MethodGet, (*Handlers).HandleTabNetworkRouteList},
	{"network route add", http.MethodPost, (*Handlers).HandleTabNetworkRoute},
	{"network unroute", http.MethodDelete, (*Handlers).HandleTabNetworkUnroute},
	{"network by id", http.MethodGet, (*Handlers).HandleTabNetworkByID},
	{"handoff", http.MethodPost, (*Handlers).HandleTabHandoff},
	{"resume", http.MethodPost, (*Handlers).HandleTabResume},
	{"handoff status", http.MethodGet, (*Handlers).HandleTabHandoffStatus},
	{"title", http.MethodGet, (*Handlers).HandleTabTitle},
	{"url", http.MethodGet, (*Handlers).HandleTabURL},
	{"html", http.MethodGet, (*Handlers).HandleTabHTML},
	{"styles", http.MethodGet, (*Handlers).HandleTabStyles},
	{"metrics", http.MethodGet, (*Handlers).HandleTabMetrics},
	{"download", http.MethodGet, (*Handlers).HandleTabDownload},
	{"pdf", http.MethodGet, (*Handlers).HandleTabPDF},
	{"clear cookies", http.MethodDelete, (*Handlers).HandleTabClearCookies},
	{"close", http.MethodPost, (*Handlers).HandleTabClose},
}

func TestEveryTabPathRouteRefusesAnEmptyOrBlankIDTheSameWay(t *testing.T) {
	for _, route := range pathTabRoutes {
		for _, id := range []string{"", "   ", "\t"} {
			t.Run(route.name+"/"+strings.TrimSpace(id)+"blank", func(t *testing.T) {
				h := pathTabHandlers(t)
				req := httptest.NewRequest(route.method, "/tabs/x/anything", nil)
				req.SetPathValue("id", id)
				req.SetPathValue("requestId", "req1")
				w := httptest.NewRecorder()
				route.call(h, w, req)
				if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "tab id required") {
					t.Fatalf("id %q: %d %s, want 400 tab id required", id, w.Code, w.Body.String())
				}
			})
		}
	}
}

func TestTabPDFRefusesABodyTabIDThatDisagreesWithThePath(t *testing.T) {
	h := pathTabHandlers(t)
	for _, tc := range []struct{ body, want string }{
		{`{"tabId":"B"}`, "does not match path id"},
		{`{"tabId":""}`, "invalid tabId"},
		{`{"tabId":7}`, "invalid tabId"},
	} {
		req := httptest.NewRequest(http.MethodPost, "/tabs/A/pdf", bytes.NewReader([]byte(tc.body)))
		req.SetPathValue("id", "A")
		w := httptest.NewRecorder()
		h.HandleTabPDF(w, req)
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), tc.want) {
			t.Fatalf("body %s: %d %s, want 400 %s", tc.body, w.Code, w.Body.String(), tc.want)
		}
	}
}

func TestThePathTabIDPreludeHasOneOwner(t *testing.T) {
	_, files := srccensus.Load(t, ".", handlersSourceFileFloor).Syntax()
	literalSites := map[string]int{}
	pathValueSites := map[string]int{}
	missingWording := 0
	for name, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.BasicLit:
				if x.Kind == token.STRING && x.Value == `"tab id required"` {
					literalSites[name]++
				}
				if x.Kind == token.STRING && strings.Contains(x.Value, "missing tab id") && name == "health_tabs.go" {
					missingWording++
				}
			case *ast.CallExpr:
				if sel, ok := x.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "PathValue" && len(x.Args) == 1 {
					if lit, ok := x.Args[0].(*ast.BasicLit); ok && lit.Value == `"id"` {
						pathValueSites[name]++
					}
				}
			}
			return true
		})
	}
	total := 0
	for _, n := range literalSites {
		total += n
	}
	if total != 1 || literalSites["tab_body_adapter.go"] != 1 {
		t.Errorf("\"tab id required\" is spelled at %v, want once in tab_body_adapter.go", literalSites)
	}
	allowed := map[string]bool{"tab_body_adapter.go": true, "find.go": true, "state_tab.go": true}
	for file := range pathValueSites {
		if !allowed[file] {
			t.Errorf("%s reads r.PathValue(\"id\") itself; route it through requirePathTabID", file)
		}
	}
	for _, file := range []string{"tab_body_adapter.go", "find.go", "state_tab.go"} {
		if pathValueSites[file] == 0 {
			t.Errorf("%s no longer reads the path id; update the census exclusions", file)
		}
	}
	if missingWording != 0 {
		t.Errorf("health_tabs.go still spells the drifted 'missing tab id' wording")
	}
}
