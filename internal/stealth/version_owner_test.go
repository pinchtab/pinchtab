package stealth

import (
	"go/ast"
	"go/token"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/browserprobe"
	"github.com/pinchtab/pinchtab/internal/srccensus"
)

// versionShaped is the full four-part Chrome build ("144.0.7559.133") and the UA-reduced
// form ("144.0.0.0") that every version surface derives from it.
var versionShaped = regexp.MustCompile(`^\d+\.\d+\.\d+\.\d+$`)

// bareMajorShaped is the Chrome major on its own, the third spelling the fallback had
// before it was owned in one place — ReducedBrowserVersion carried `major = "144"`.
var bareMajorShaped = regexp.MustCompile(`^\d{3}$`)

// versionLiteralExemptions are the four-part numeric literals in these packages that are
// NOT a browser version, listed by VALUE with the reason. Exempting by value rather than
// by file keeps the exemption from widening: a new literal at an exempt site still fails.
var versionLiteralExemptions = map[string]string{
	"127.0.0.1": "the loopback address, not a version",
	"0.0.0.0":   "the wildcard bind address, not a version",
	"99.0.0.0":  "the Sec-CH-UA GREASE placeholder, which is deliberately not a real Chrome build and must not track the fallback",
}

// versionOwnerPackages are the packages that build or default a browser version. They are
// listed as PREFIXES over a derived module walk rather than as a file list, so a new file
// in any of them is scanned without an edit here.
var versionOwnerPackages = []string{
	"internal/config/",
	"internal/stealth/",
	"internal/handlers/",
}

// The fallback Chrome version had four independent copies across three packages — two
// config defaults and two fallbacks in this file's UA builder — so a Chrome release meant
// four edits in lockstep and nothing failed when they drifted. browserprobe now owns it,
// and this census is what keeps it owned: any version-shaped literal reintroduced in the
// packages that build a version fails here, by value, with the owner named.
//
// browserprobe itself is not scanned — it is the owner, and the one definition lives
// there.
func TestNoPackageKeepsItsOwnCopyOfTheFallbackBrowserVersion(t *testing.T) {
	exemptionSeen := map[string]bool{}
	scanned := 0

	for _, file := range srccensus.Tree(t, "../..", 200) {
		if !inVersionOwnerPackage(file.Name) {
			continue
		}
		scanned++

		parsed := srccensus.Parse(t, token.NewFileSet(), file.Name, file.Text, 0)
		ast.Inspect(parsed, func(node ast.Node) bool {
			lit, ok := node.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			value, uErr := strconv.Unquote(lit.Value)
			if uErr != nil {
				return true
			}
			if !versionShaped.MatchString(value) && !bareMajorShaped.MatchString(value) {
				return true
			}
			if reason, exempt := versionLiteralExemptions[value]; exempt {
				exemptionSeen[value] = true
				_ = reason
				return true
			}
			t.Errorf("%s writes the browser version %q out as a literal; read browserprobe.FallbackChromeVersion instead, or add %q to versionLiteralExemptions with the reason it is not a version", file.Name, value, value)
			return true
		})
	}

	if scanned < 20 {
		t.Fatalf("scanned %d files across %v; the walk matched almost nothing and this census would pass vacuously", scanned, versionOwnerPackages)
	}
	for value, reason := range versionLiteralExemptions {
		if !exemptionSeen[value] {
			t.Errorf("%q is exempted as %q but no longer appears in these packages; drop the entry rather than leaving a stale exemption", value, reason)
		}
	}
}

// The probe only fires when browser.version is unset, so the config defaults must NOT
// pin it — not to a literal (the census above bans those) and not to the fallback
// constant either. A default here would make every fresh config advertise the frozen
// version and the launched-binary probe would never run, which is the defect. So this
// asserts neither config default supplies a BrowserVersion field at all, and that the
// fallback survives only where it belongs: the persona layer, reading the one owner.
func TestTheConfigDefaultsLeaveBrowserVersionUnset(t *testing.T) {
	for _, tc := range []struct{ dir, file, what string }{
		{"../config", "config_file.go", "the file-config default written into a new config"},
		{"../config", "config_load.go", "the RuntimeConfig default used when no file config applies"},
	} {
		// Load fails when the package yields too few files, so a census pointed at a
		// directory that moved fatals here instead of finding nothing and passing.
		_, files := srccensus.Load(t, tc.dir, 3).Syntax()
		file, present := files[tc.file]
		if !present {
			t.Fatalf("%s is not in %s; this census is reading the wrong package", tc.file, tc.dir)
		}

		if _, found := browserVersionFieldValue(file); found {
			t.Errorf("%s sets a BrowserVersion default, so %s pins the version and the launched-binary probe never fires", tc.file, tc.what)
		}
	}

	// The fallback lives at the persona layer instead: ReducedBrowserVersion reads the
	// one owner, so the literal is applied only when the probe cannot answer.
	_, stealthFiles := srccensus.Load(t, ".", 3).Syntax()
	if !funcReferencesFallbackConstant(stealthFiles["ua.go"], "ReducedBrowserVersion") {
		t.Error("ReducedBrowserVersion no longer reads browserprobe.FallbackChromeVersion, so the UA fallback comes from somewhere else and the value can drift again")
	}
}

// The fallback the UA builder actually produces must be the owner's value, reduced. This
// is the behavioural half: the census above proves the constant is referenced, this
// proves it is the value that comes out.
func TestTheAdvertisedFallbackIsTheOwnersValue(t *testing.T) {
	major, _, _ := strings.Cut(browserprobe.FallbackChromeVersion, ".")
	if major == "" {
		t.Fatalf("browserprobe.FallbackChromeVersion = %q has no major", browserprobe.FallbackChromeVersion)
	}

	if got, want := ReducedBrowserVersion(""), major+".0.0.0"; got != want {
		t.Errorf("ReducedBrowserVersion(\"\") = %q, want %q derived from the owner", got, want)
	}
	if got, want := chromeVersionOrFallback(""), major+".0.0.0"; got != want {
		t.Errorf("chromeVersionOrFallback(\"\") = %q, want %q derived from the owner", got, want)
	}

	persona := BuildPersona(ChromeUserAgent(PlatformMacOS, ReducedBrowserVersion("")), "")
	if !strings.Contains(persona.UserAgent, "Chrome/"+major+".0.0.0") {
		t.Errorf("fallback persona UA = %q, want the owner's major %s", persona.UserAgent, major)
	}
	for _, brand := range persona.UserAgentData.Brands {
		if brand.Brand == BrandChrome && brand.Version != major {
			t.Errorf("fallback Sec-CH-UA brand version = %q, want the owner's major %s", brand.Version, major)
		}
	}
}

func inVersionOwnerPackage(name string) bool {
	for _, prefix := range versionOwnerPackages {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func isFallbackConstant(node ast.Node) bool {
	sel, ok := node.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "FallbackChromeVersion" {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "browserprobe"
}

// browserVersionFieldValue reports whether every BrowserVersion field set in file takes
// its value from the owner, and whether any such field was found at all — an absent field
// and a field set from a literal are different failures and must not collapse.
func browserVersionFieldValue(file *ast.File) (fromOwner bool, found bool) {
	fromOwner = true
	ast.Inspect(file, func(node ast.Node) bool {
		kv, ok := node.(*ast.KeyValueExpr)
		if !ok {
			return true
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok || key.Name != "BrowserVersion" {
			return true
		}
		found = true
		if !isFallbackConstant(kv.Value) {
			fromOwner = false
		}
		return true
	})
	return fromOwner, found
}

func funcReferencesFallbackConstant(file *ast.File, funcName string) bool {
	found := false
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != funcName {
			continue
		}
		ast.Inspect(fn, func(node ast.Node) bool {
			if isFallbackConstant(node) {
				found = true
			}
			return true
		})
	}
	return found
}
