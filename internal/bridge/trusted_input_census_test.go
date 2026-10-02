package bridge

import (
	"go/ast"
	"go/token"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/srccensus"
)

const (
	chromedpImportPath   = "github.com/chromedp/chromedp"
	cdprotoInputImport   = "github.com/chromedp/cdproto/input"
	trustedInputWrapper  = "TrustedInputAction"
	rawInputMethodPrefix = "Input."
)

var rawInputOwners = map[string]bool{
	"internal/bridge/cdpops/pointer.go::MouseEventAction": true,
	"internal/bridge/cdpops/pointer.go::KeyEventAction":   true,
	"internal/bridge/cdpops/pointer.go::InsertTextAction": true,
}

var chromedpInputActions = map[string]bool{
	"KeyEvent":       true,
	"KeyEventNode":   true,
	"SendKeys":       true,
	"Click":          true,
	"DoubleClick":    true,
	"MouseClickXY":   true,
	"MouseClickNode": true,
	"MouseEvent":     true,
}

type inputDispatchSites struct {
	raw       []string
	unwrapped []string
}

func trustedInputSites(t *testing.T, name, src string) inputDispatchSites {
	t.Helper()
	file := srccensus.Parse(t, token.NewFileSet(), name, src, 0)
	chromedpNames, inputNames := map[string]bool{}, map[string]bool{}
	for _, imp := range file.Imports {
		path, _ := strconv.Unquote(imp.Path.Value)
		local := path[strings.LastIndex(path, "/")+1:]
		if imp.Name != nil {
			local = imp.Name.Name
		}
		switch path {
		case chromedpImportPath:
			chromedpNames[local] = true
		case cdprotoInputImport:
			inputNames[local] = true
		}
	}

	var sites inputDispatchSites
	scan := func(fnName string, body ast.Node) {
		site := name + "::" + fnName
		wrapped := map[ast.Expr]bool{}
		ast.Inspect(body, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok && calleeName(call.Fun) == trustedInputWrapper {
				for _, arg := range call.Args {
					wrapped[arg] = true
				}
			}
			return true
		})
		ast.Inspect(body, func(n ast.Node) bool {
			switch e := n.(type) {
			case *ast.BasicLit:
				if value, err := strconv.Unquote(e.Value); err == nil && e.Kind == token.STRING && isRawInputMethod(value) {
					sites.raw = append(sites.raw, site)
				}
			case *ast.CallExpr:
				pkg, fn, ok := packageCall(e.Fun)
				switch {
				case ok && inputNames[pkg] && (strings.HasPrefix(fn, "Dispatch") || fn == "InsertText" || fn == "ImeSetComposition"):
					sites.raw = append(sites.raw, site)
				case ok && chromedpNames[pkg] && chromedpInputActions[fn] && !wrapped[e]:
					sites.unwrapped = append(sites.unwrapped, site)
				}
			}
			return true
		})
	}
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Body != nil {
				scan(d.Name.Name, d.Body)
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				if value, ok := spec.(*ast.ValueSpec); ok {
					for i, v := range value.Values {
						scan(value.Names[min(i, len(value.Names)-1)].Name, v)
					}
				}
			}
		}
	}
	return sites
}

func isRawInputMethod(value string) bool {
	method, ok := strings.CutPrefix(value, rawInputMethodPrefix)
	if !ok || strings.ContainsAny(method, " .") {
		return false
	}
	return strings.HasPrefix(method, "dispatch") || method == "insertText" || method == "imeSetComposition"
}

func calleeName(fun ast.Expr) string {
	switch f := fun.(type) {
	case *ast.Ident:
		return f.Name
	case *ast.SelectorExpr:
		return f.Sel.Name
	}
	return ""
}

func packageCall(fun ast.Expr) (string, string, bool) {
	sel, ok := fun.(*ast.SelectorExpr)
	if !ok {
		return "", "", false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return "", "", false
	}
	return pkg.Name, sel.Sel.Name, true
}

func TestEveryTrustedInputDispatchGoesThroughTheFocusEmulatingHelper(t *testing.T) {
	raw := map[string]bool{}
	var unwrapped []string
	for _, file := range srccensus.Tree(t, "../..", 200) {
		sites := trustedInputSites(t, file.Name, file.Text)
		for _, site := range sites.raw {
			raw[site] = true
		}
		unwrapped = append(unwrapped, sites.unwrapped...)
	}

	var stray []string
	for site := range raw {
		if !rawInputOwners[site] {
			stray = append(stray, site)
		}
	}
	sort.Strings(stray)
	for _, site := range stray {
		t.Errorf("%s dispatches Input.* directly; use bridgecdpops.MouseEventAction, KeyEventAction or InsertTextAction so focus emulation is enabled first", site)
	}
	for _, site := range slices.Compact(slices.Sorted(slices.Values(unwrapped))) {
		t.Errorf("%s runs a chromedp input action outside bridgecdpops.TrustedInputAction", site)
	}
	for owner := range rawInputOwners {
		if !raw[owner] {
			t.Errorf("%s no longer dispatches Input.*; update rawInputOwners so the census still guards the helper", owner)
		}
	}
}

func TestTheTrustedInputCensusSeesEveryBypassAndAllowsTheWrappedForms(t *testing.T) {
	const planted = `package p

import (
	cdpinput "github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
	bridgecdpops "github.com/pinchtab/pinchtab/internal/bridge/cdpops"
)

func rawString(ctx context.Context) error {
	return chromedp.FromContext(ctx).Target.Execute(ctx, "Input.dispatchKeyEvent", nil, nil)
}

func rawInsert(ctx context.Context) error {
	return chromedp.FromContext(ctx).Target.Execute(ctx, "Input.insertText", nil, nil)
}

func typedDispatch(ctx context.Context) error {
	return cdpinput.DispatchMouseEvent(cdpinput.MouseMoved, 1, 2).Do(ctx)
}

var seam = func(ctx context.Context) error {
	return chromedp.Run(ctx, chromedp.KeyEvent("x"))
}

func unwrappedClick(ctx context.Context) error {
	return chromedp.Run(ctx, chromedp.Click("#a"), bridgecdpops.TrustedInputAction(chromedp.SendKeys("#a", "x")))
}

func wrapped(ctx context.Context) error {
	return chromedp.Run(ctx,
		bridgecdpops.TrustedInputAction(chromedp.KeyEvent("x")),
		bridgecdpops.MouseEventAction(map[string]any{"type": "mouseMoved"}),
		chromedp.Evaluate("Input.dispatch is only text inside a script", nil),
	)
}
`
	sites := trustedInputSites(t, "planted.go", planted)
	wantRaw := []string{"planted.go::rawInsert", "planted.go::rawString", "planted.go::typedDispatch"}
	if got := slices.Sorted(slices.Values(sites.raw)); !slices.Equal(got, wantRaw) {
		t.Errorf("raw sites = %v, want %v", got, wantRaw)
	}
	wantUnwrapped := []string{"planted.go::seam", "planted.go::unwrappedClick"}
	if got := slices.Sorted(slices.Values(sites.unwrapped)); !slices.Equal(got, wantUnwrapped) {
		t.Errorf("unwrapped sites = %v, want %v", got, wantUnwrapped)
	}
}
