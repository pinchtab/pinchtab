package stealth

import (
	"fmt"
	"go/ast"
	"go/token"
	"os/exec"
	"path/filepath"
	"reflect"
	goruntime "runtime"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/srccensus"
)

func hostVersionOracle(t *testing.T) (uaDataPlatform, version string) {
	t.Helper()
	switch goruntime.GOOS {
	case "darwin":
		out, err := exec.Command("sw_vers", "-productVersion").Output()
		if err != nil {
			t.Skipf("sw_vers unavailable: %v", err)
		}
		return "macOS", strings.TrimSpace(string(out))
	case "linux":
		out, err := exec.Command("uname", "-r").Output()
		if err != nil {
			t.Skipf("uname unavailable: %v", err)
		}
		return "Linux", strings.TrimSpace(string(out))
	default:
		t.Skipf("no host version oracle for %s", goruntime.GOOS)
		return "", ""
	}
}

func TestPlatformVersionForHostPlatformTracksHost(t *testing.T) {
	uaDataPlatform, raw := hostVersionOracle(t)
	want := normalizePlatformVersion(raw)
	if want == "" {
		t.Fatalf("oracle produced no version from %q", raw)
	}
	if got := PlatformVersionFor(uaDataPlatform); got != want {
		t.Fatalf("PlatformVersionFor(%q) = %q, want host version %q", uaDataPlatform, got, want)
	}
}

// Windows is the one platform whose platformVersion must NOT come from the host,
// and the reason lives here rather than in a comment because a comment cannot
// fail. Chrome's Sec-CH-UA-Platform-Version on Windows is derived from the
// UniversalApiContract version, not the OS version: Windows 11 22H2 reports
// 15.0.0 while its OS version is 10.0.22621. A Windows arm reading host data would
// emit something like 10.0.26100 — a value real Chrome never sends — making the
// fingerprint worse than the constant it replaced.
//
// hostPlatformVersion has no Windows branch, so this exercises the same code path
// on every GOOS: the guard is real on the macOS and Linux runners that exist, and
// its value does not wait for a Windows runner. No build tag, no skip, no oracle —
// a guard that skips is the failure mode this test exists to close.
func TestWindowsPlatformVersionIsNotDerivedFromTheHostBecauseChromeSendsAnAPIContractVersion(t *testing.T) {
	if got := hostPlatformVersion("Windows"); got != "" {
		t.Errorf("hostPlatformVersion(\"Windows\") = %q, want empty.\n"+
			"Chrome derives Sec-CH-UA-Platform-Version on Windows from the UniversalApiContract version, not the OS version "+
			"(Windows 11 22H2 sends 15.0.0 while its OS version is 10.0.22621), so a host-derived value is one real Chrome never sends. "+
			"Completing the switch with a Windows arm makes the persona easier to detect, not harder — the constant is the correct answer here.", got)
	}

	// The rule is about where the value comes from, not about emptiness: the
	// advertised Windows version stays the frozen default, so this test cannot be
	// satisfied by making PlatformVersionFor return nothing.
	if got, want := PlatformVersionFor("Windows"), "15.0.0"; got != want {
		t.Errorf("PlatformVersionFor(%q) = %q, want the frozen %q that matches what Chrome sends", "Windows", got, want)
	}

	// The assertions above cannot see the edit this test exists to stop. A Windows
	// arm gated on GOOS == "windows" — the shape a contributor "completing the
	// switch" would write — never executes on a macOS or Linux runner, so calling
	// hostPlatformVersion here returns empty either way and the guard would pass on
	// every machine this project actually tests on. The arms themselves are what
	// must be checked, and reading them is GOOS-independent.
	//
	// Read as a syntax tree rather than as text. A text scan for "Windows" is
	// defeated by the tidier spelling the file itself argues for — the exported
	// PlatformWindows constant — and by any third spelling (a local alias, a helper
	// call). Resolving comparisons through the package's own constants makes the
	// check spelling-independent, and it is what lets the anti-degradation half
	// below tell a genuine loss of the host read from a contributor rewriting the
	// existing arms with those same constants, which a literal scan reports as a
	// missing arm — a failure message asserting something untrue.
	// Package-scoped, not file-scoped: the rule is that nothing in this package
	// reads a host fact for Windows, and a guard that reads only ua.go states a
	// rule wider than it checks — the same scope gap that let the text scan miss a
	// helper one call away.
	sources, fileSet := parsePackageSources(t)
	platformNames := packageStringConstants(t, sources)
	readers := hostReadingDeclarations(t, sources)
	arms, defaultReturnsEmpty := hostPlatformVersionArms(t, sources, fileSet, platformNames, readers)

	// Check 1 — which platforms the switch answers for, by resolved value. Any
	// third arm is the regression however its platform is named.
	got := make([]string, 0, len(arms))
	for _, arm := range arms {
		if arm.platform == "" {
			t.Errorf("the arm at %s compares uaDataPlatform against an expression this guard cannot resolve to a platform name.\n"+
				"Spell it as a string literal or one of the Platform* constants: an arm whose platform cannot be read is an arm this guard cannot police.", arm.where)
		}
		got = append(got, fmt.Sprintf("%s/%s", arm.platform, arm.goos))
	}
	sort.Strings(got)
	want := []string{PlatformLinux + "/linux", PlatformMacOS + "/darwin"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("hostPlatformVersion answers for %v, want exactly %v.\n"+
			"Chrome derives Sec-CH-UA-Platform-Version on Windows from the UniversalApiContract version, not the OS version, "+
			"so any host-derived value there is one real Chrome never sends. Leave Windows on the frozen default. "+
			"Each arm must also keep its GOOS pairing: without it the arm reads this host's version while claiming another platform.", got, want)
	}
	if !defaultReturnsEmpty {
		t.Errorf("hostPlatformVersion's default arm no longer returns the empty string.\n" +
			"A default that delegates is how a Windows host read hides from a check on the arms: the switch keeps two arms and " +
			"the read moves one call away. Every platform without an arm must fall through to the frozen default in PlatformVersionFor.")
	}

	// Check 2 — the host readers are exactly the two known ones. A new
	// host-reading helper is the other way a Windows read arrives, and it is
	// invisible to any check that only looks at hostPlatformVersion.
	gotReaders := make([]string, 0, len(readers))
	for name := range readers {
		gotReaders = append(gotReaders, name)
	}
	sort.Strings(gotReaders)
	wantReaders := []string{"hostKernelVersion", "hostProductVersion"}
	if !reflect.DeepEqual(gotReaders, wantReaders) {
		t.Errorf("the package's host-fact readers are %v, want exactly %v.\n"+
			"A third reader means some platform learned to read this host; on Windows that advertises the OS version "+
			"(something like 10.0.26100) as Sec-CH-UA-Platform-Version, which real Chrome never sends.", gotReaders, wantReaders)
	}

	// Check 3 — each reader is called exactly once, from the arm for its own
	// platform. This is what survives indirection: moving a read into a helper,
	// into an early return above the switch, or up into PlatformVersionFor all show
	// up as a call site that is not the one arm allowed to have it. It also keeps
	// the parent card's anti-degradation rule, which is about the two arms reading
	// the RIGHT value — the behavioural oracle cannot check that, because it
	// exercises the macOS arm only on a macOS machine and the suite runs on Linux.
	for _, expected := range []struct{ platform, reader, why string }{
		{PlatformMacOS, "hostProductVersion", "Chrome reports the macOS product version (sw_vers -productVersion) through UA-CH, not the Darwin kernel release"},
		{PlatformLinux, "hostKernelVersion", "Chrome derives the Linux platformVersion from the kernel release (uname -r), not from a distro product version"},
	} {
		sites := callSitesOf(sources, fileSet, expected.reader)
		if len(sites) != 1 {
			t.Errorf("%s is called from %d places (%s), want exactly 1 — the %s arm of hostPlatformVersion.\n"+
				"A second call site is a host read for some other platform, wherever it is written.",
				expected.reader, len(sites), strings.Join(sites, ", "), expected.platform)
			continue
		}
		arm, ok := armFor(arms, expected.platform)
		if !ok {
			t.Errorf("hostPlatformVersion has no %s arm, so this test would be pinning an empty switch rather than the Windows exception", expected.platform)
			continue
		}
		if !slices.Contains(arm.reads, expected.reader) {
			t.Errorf("the %s arm reads %v, want %s.\n%s", expected.platform, arm.reads, expected.reader, expected.why)
		}
	}
}

// platformArm is one case of hostPlatformVersion's switch, with the platform and
// GOOS it compares against resolved to their values, so the constant and the
// literal spelling of the same arm are indistinguishable here.
type platformArm struct {
	platform string
	goos     string
	reads    []string
	where    string
}

func armFor(arms []platformArm, platform string) (platformArm, bool) {
	for _, arm := range arms {
		if arm.platform == platform {
			return arm, true
		}
	}
	return platformArm{}, false
}

// hostFactImport is the package every host read here goes through today. It is
// named rather than derived so that swapping the dependency reds this test — the
// census below would otherwise quietly become empty.
const hostFactImport = "host"

// parsePackageSources parses every non-test file of this package, so a host read
// added in a sibling file is inside the census rather than outside it.
func parsePackageSources(t *testing.T) (map[string]*ast.File, *token.FileSet) {
	t.Helper()
	fileSet, sources := srccensus.Load(t, ".", 5).Syntax()
	importsHostFacts := false
	for _, file := range sources {
		for _, spec := range file.Imports {
			if strings.HasSuffix(strings.Trim(spec.Path.Value, `"`), "/"+hostFactImport) {
				importsHostFacts = true
			}
		}
	}
	if !importsHostFacts {
		t.Fatalf("no file in this package imports a %q package; the host-reader census has nothing to key on and would pass vacuously", hostFactImport)
	}
	return sources, fileSet
}

func packageStringConstants(t *testing.T, sources map[string]*ast.File) map[string]string {
	t.Helper()
	values := map[string]string{}
	for _, decl := range allDeclarations(sources) {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range value.Names {
				if i >= len(value.Values) {
					continue
				}
				if resolved, ok := stringValueOf(value.Values[i], values); ok {
					values[name.Name] = resolved
				}
			}
		}
	}
	if len(values) == 0 {
		t.Fatal("no string constants found in ua.go; the arms below could not be resolved through the platform vocabulary")
	}
	return values
}

// stringValueOf resolves a string literal, a reference to an already-known
// constant, or a + chain of either. The + chain matters: a contributor can spell a
// platform name as "Win" + "dows" and defeat any check that only reads literals.
func stringValueOf(expr ast.Expr, known map[string]string) (string, bool) {
	switch typed := expr.(type) {
	case *ast.BasicLit:
		if typed.Kind == token.STRING {
			return strings.Trim(typed.Value, `"`), true
		}
	case *ast.Ident:
		if value, ok := known[typed.Name]; ok {
			return value, true
		}
	case *ast.BinaryExpr:
		if typed.Op != token.ADD {
			return "", false
		}
		left, okLeft := stringValueOf(typed.X, known)
		right, okRight := stringValueOf(typed.Y, known)
		if okLeft && okRight {
			return left + right, true
		}
	}
	return "", false
}

// hostReadingDeclarations is the derived set of package declarations that read a
// host fact: any top-level function or var whose body reaches the host package.
// Derived rather than listed so a new reader appears here without anyone
// remembering to add it.
func hostReadingDeclarations(t *testing.T, sources map[string]*ast.File) map[string]bool {
	t.Helper()
	readers := map[string]bool{}
	readsHost := func(node ast.Node) bool {
		reads := false
		ast.Inspect(node, func(n ast.Node) bool {
			selector, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if ident, ok := selector.X.(*ast.Ident); ok && ident.Name == hostFactImport {
				reads = true
			}
			return true
		})
		return reads
	}
	for _, decl := range allDeclarations(sources) {
		switch typed := decl.(type) {
		case *ast.FuncDecl:
			if typed.Body != nil && readsHost(typed.Body) {
				readers[typed.Name.Name] = true
			}
		case *ast.GenDecl:
			if typed.Tok != token.VAR {
				continue
			}
			for _, spec := range typed.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, name := range value.Names {
					if i < len(value.Values) && readsHost(value.Values[i]) {
						readers[name.Name] = true
					}
				}
			}
		}
	}
	if len(readers) == 0 {
		t.Fatal("no declaration in this package reads the host package; this census has nothing to constrain and would pass vacuously")
	}
	return readers
}

// hostPlatformVersionArms resolves hostPlatformVersion's switch into arms. A
// missing function is reported as such: the previous text-slicing version indexed
// on the function's name and panicked with a slice-bounds error when a careful
// rename updated production and the direct call but left the scan's literal stale.
func hostPlatformVersionArms(t *testing.T, sources map[string]*ast.File, fileSet *token.FileSet, platformNames map[string]string, readers map[string]bool) ([]platformArm, bool) {
	t.Helper()
	var decl *ast.FuncDecl
	for _, candidate := range allDeclarations(sources) {
		if fn, ok := candidate.(*ast.FuncDecl); ok && fn.Name.Name == "hostPlatformVersion" {
			decl = fn
		}
	}
	if decl == nil {
		t.Fatal("this package declares no hostPlatformVersion; if it was renamed, update this guard to the new name — it is the one place the Windows exception is enforced")
	}

	isGOOS := func(expr ast.Expr) bool {
		selector, ok := expr.(*ast.SelectorExpr)
		return ok && selector.Sel.Name == "GOOS"
	}

	var arms []platformArm
	defaultReturnsEmpty := false
	for _, statement := range decl.Body.List {
		switchStatement, ok := statement.(*ast.SwitchStmt)
		if !ok {
			continue
		}
		for _, clause := range switchStatement.Body.List {
			caseClause, ok := clause.(*ast.CaseClause)
			if !ok {
				continue
			}
			if caseClause.List == nil {
				defaultReturnsEmpty = returnsEmptyString(caseClause.Body)
				continue
			}
			position := fileSet.Position(caseClause.Pos())
			arm := platformArm{where: fmt.Sprintf("%s:%d", filepath.Base(position.Filename), position.Line)}
			for _, condition := range caseClause.List {
				ast.Inspect(condition, func(n ast.Node) bool {
					binary, ok := n.(*ast.BinaryExpr)
					if !ok || binary.Op != token.EQL {
						return true
					}
					for _, pair := range [][2]ast.Expr{{binary.X, binary.Y}, {binary.Y, binary.X}} {
						value, resolved := stringValueOf(pair[1], platformNames)
						if !resolved {
							continue
						}
						if isGOOS(pair[0]) {
							arm.goos = value
						} else if ident, ok := pair[0].(*ast.Ident); ok && ident.Name == "uaDataPlatform" {
							arm.platform = value
						}
					}
					return true
				})
			}
			ast.Inspect(&ast.BlockStmt{List: caseClause.Body}, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				if ident, ok := call.Fun.(*ast.Ident); ok && readers[ident.Name] {
					arm.reads = append(arm.reads, ident.Name)
				}
				return true
			})
			arms = append(arms, arm)
		}
	}
	if len(arms) == 0 {
		t.Fatal("hostPlatformVersion has no switch arms to read; this guard would pass vacuously")
	}
	return arms, defaultReturnsEmpty
}

func returnsEmptyString(body []ast.Stmt) bool {
	if len(body) != 1 {
		return false
	}
	statement, ok := body[0].(*ast.ReturnStmt)
	if !ok || len(statement.Results) != 1 {
		return false
	}
	literal, ok := statement.Results[0].(*ast.BasicLit)
	return ok && literal.Kind == token.STRING && literal.Value == `""`
}

// allDeclarations flattens the package's declarations in a stable file order, so
// every census below reads the whole package rather than one file.
func allDeclarations(sources map[string]*ast.File) []ast.Decl {
	names := make([]string, 0, len(sources))
	for name := range sources {
		names = append(names, name)
	}
	sort.Strings(names)
	var decls []ast.Decl
	for _, name := range names {
		decls = append(decls, sources[name].Decls...)
	}
	return decls
}

// callSitesOf reports every call to name in the package as "file:enclosing:line",
// so a second call site is named where the reader can open it.
func callSitesOf(sources map[string]*ast.File, fileSet *token.FileSet, name string) []string {
	var sites []string
	for _, decl := range allDeclarations(sources) {
		enclosing := "package-level var"
		if fn, ok := decl.(*ast.FuncDecl); ok {
			enclosing = fn.Name.Name
		}
		ast.Inspect(decl, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == name {
				position := fileSet.Position(call.Pos())
				sites = append(sites, fmt.Sprintf("%s:%s:%d", filepath.Base(position.Filename), enclosing, position.Line))
			}
			return true
		})
	}
	return sites
}

func TestPlatformVersionForForeignPlatformUsesDefault(t *testing.T) {
	foreign := "Windows"
	if goruntime.GOOS == "windows" {
		foreign = "macOS"
	}
	if got := PlatformVersionFor(foreign); got != defaultPlatformVersions[foreign] {
		t.Fatalf("PlatformVersionFor(%q) = %q, want default %q", foreign, got, defaultPlatformVersions[foreign])
	}
	if got := PlatformVersionFor("Android"); got != "" {
		t.Fatalf("PlatformVersionFor unknown platform = %q, want empty", got)
	}
}

func TestNormalizePlatformVersion(t *testing.T) {
	cases := map[string]string{
		"26.5.1":                 "26.5.1",
		"26.5":                   "26.5.0",
		"15":                     "15.0.0",
		"6.5.0-27-generic":       "6.5.0",
		"6.11.0-19-lowlatency":   "6.11.0",
		"10.0.22631 Build 22631": "10.0.22631",
		"":                       "",
		"unknown":                "",
		"14.7.1.2":               "14.7.1",
	}
	for raw, want := range cases {
		if got := normalizePlatformVersion(raw); got != want {
			t.Errorf("normalizePlatformVersion(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestBuildPersonaPlatformVersionMatchesPlatform(t *testing.T) {
	windows := BuildPersona("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/144.0.0.0 Safari/537.36", "144.0.7559.133")
	if windows.UserAgentData.Platform != "Windows" {
		t.Fatalf("platform = %q, want Windows", windows.UserAgentData.Platform)
	}
	if windows.UserAgentData.PlatformVersion != PlatformVersionFor("Windows") {
		t.Fatalf("platformVersion = %q, want %q", windows.UserAgentData.PlatformVersion, PlatformVersionFor("Windows"))
	}

	uaDataPlatform, _ := hostVersionOracle(t)
	hostUA := "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/144.0.0.0 Safari/537.36"
	if uaDataPlatform == "Linux" {
		hostUA = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/144.0.0.0 Safari/537.36"
	}
	persona := BuildPersona(hostUA, "144.0.7559.133")
	if persona.UserAgentData.PlatformVersion != PlatformVersionFor(uaDataPlatform) {
		t.Fatalf("host persona platformVersion = %q, want %q", persona.UserAgentData.PlatformVersion, PlatformVersionFor(uaDataPlatform))
	}
}

// The brand list is where the UA string and the client hints have to agree. A detector
// cross-checking the two sees a browser that cannot exist when they disagree, which is
// louder than either being wrong alone — so the vendor brand is read out of the UA rather
// than fixed at Chrome, and a UA naming no Chromium build claims no brands at all.
func TestPersonaBrandsNameTheBrowserTheUserAgentClaims(t *testing.T) {
	const version = "144.0.7559.133"
	windowsChrome := ChromeUserAgent(PlatformWindows, ReducedBrowserVersion(version))

	for _, tc := range []struct {
		name      string
		userAgent string
		want      []string
	}{
		{
			name:      "chrome",
			userAgent: windowsChrome,
			want:      []string{"Not(A:Brand", "Google Chrome", "Chromium"},
		},
		{
			name:      "edge",
			userAgent: EdgeUserAgent(windowsChrome, ReducedBrowserVersion(version)),
			want:      []string{"Not(A:Brand", "Microsoft Edge", "Chromium"},
		},
		{
			// Real Safari implements no UA-CH at all, so brands here would be the tell.
			name:      "safari",
			userAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15",
			want:      nil,
		},
	} {
		persona := BuildPersona(tc.userAgent, version)

		for _, list := range [][]BrandVersion{persona.UserAgentData.Brands, persona.UserAgentData.FullVersionList} {
			if len(list) != len(tc.want) {
				t.Errorf("%s: brands = %v, want %v", tc.name, list, tc.want)
				continue
			}
			for i, want := range tc.want {
				if list[i].Brand != want {
					t.Errorf("%s: brand %d = %q, want %q", tc.name, i, list[i].Brand, want)
				}
				if list[i].Version == "" {
					t.Errorf("%s: brand %q carries no version", tc.name, list[i].Brand)
				}
			}
		}
	}
}

// The two lists differ in one way only, and it is the way real Chrome differs: sec-ch-ua
// carries majors, the full version list carries builds.
func TestPersonaBrandVersionsAreMajorInBrandsAndFullInTheVersionList(t *testing.T) {
	persona := BuildPersona(ChromeUserAgent(PlatformMacOS, ReducedBrowserVersion("144.0.7559.133")), "144.0.7559.133")

	for _, brand := range persona.UserAgentData.Brands {
		if brand.Brand == BrandChrome && brand.Version != "144" {
			t.Errorf("sec-ch-ua %s version = %q, want the major 144", brand.Brand, brand.Version)
		}
	}
	for _, brand := range persona.UserAgentData.FullVersionList {
		if brand.Brand == BrandChrome && brand.Version != "144.0.7559.133" {
			t.Errorf("fullVersionList %s version = %q, want the full build", brand.Brand, brand.Version)
		}
	}
}

// The architecture a persona advertises must agree with the architecture its own UA
// claims. hostArchitecture-style host reads are gated exactly like
// hostPlatformVersion's: only a persona describing this host may consult it. The
// pure form takes the host facts as arguments, so both an ARM host and an x86 host
// are asserted on every runner rather than only the one the suite happens to run on.
func TestArchitectureIsDerivedFromThePersonaNotTheHost(t *testing.T) {
	version := ReducedBrowserVersion("144.0.0.0")
	hosts := []struct {
		os   string
		arch string
	}{
		{"darwin", "arm64"},
		{"darwin", "amd64"},
		{"linux", "arm64"},
		{"linux", "amd64"},
		{"windows", "amd64"},
	}

	for _, platform := range []struct {
		name           string
		uaDataPlatform string
		want           map[string]string
	}{
		{
			name:           PlatformWindows,
			uaDataPlatform: "Windows",
			want:           map[string]string{"darwin/arm64": "x86", "darwin/amd64": "x86", "linux/arm64": "x86", "linux/amd64": "x86", "windows/amd64": "x86"},
		},
		{
			name:           PlatformLinux,
			uaDataPlatform: "Linux",
			want:           map[string]string{"darwin/arm64": "x86", "darwin/amd64": "x86", "linux/arm64": "x86", "linux/amd64": "x86", "windows/amd64": "x86"},
		},
		{
			name:           PlatformMacOS,
			uaDataPlatform: "macOS",
			want:           map[string]string{"darwin/arm64": "arm", "darwin/amd64": "x86", "linux/arm64": "x86", "linux/amd64": "x86", "windows/amd64": "x86"},
		},
	} {
		ua := ChromeUserAgent(platform.name, version)
		for _, host := range hosts {
			key := host.os + "/" + host.arch
			got := architectureForHost(ua, platform.uaDataPlatform, host.os, host.arch)
			if want := platform.want[key]; got != want {
				t.Errorf("architecture for the %s persona on a %s host = %q, want %q.\n"+
					"Its UA is %q — a persona may only read the host architecture when it describes that host, "+
					"the same gate hostPlatformVersion applies to the OS version.", platform.name, key, got, want, ua)
			}
			if edge := architectureForHost(EdgeUserAgent(ua, version), platform.uaDataPlatform, host.os, host.arch); edge != got {
				t.Errorf("the Edge decoration of the %s persona reports %q where Chrome reports %q on a %s host", platform.name, edge, got, key)
			}
		}
	}
}

// The ARM branch has no live input: no shipped matrix UA carries an ARM token, so it
// is exercised with a synthetic UA. It is unreachable, not dead — a future ARM matrix
// entry is the supported way to offer an ARM persona, and it travels through here.
func TestAnArmUserAgentStillReportsArmOnAnyHost(t *testing.T) {
	version := ReducedBrowserVersion("144.0.0.0")
	for _, platform := range FingerprintPlatforms() {
		ua := ChromeUserAgent(platform, version)
		for _, token := range []string{"arm64", "aarch64", "ARM"} {
			if strings.Contains(ua, token) {
				t.Fatalf("the shipped %s UA %q now carries %q, so the synthetic UAs below no longer stand in for a real one — assert the matrix entry directly", platform, ua, token)
			}
		}
	}

	for _, ua := range []string{
		"Mozilla/5.0 (Windows NT 10.0; Win64; arm64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/144.0.0.0 Safari/537.36",
		"Mozilla/5.0 (X11; Linux aarch64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/144.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Linux; ARM) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/144.0.0.0 Safari/537.36",
	} {
		for _, hostArch := range []string{"amd64", "arm64"} {
			if got := architectureForHost(ua, "Windows", "linux", hostArch); got != "arm" {
				t.Errorf("architecture for %q on a linux/%s host = %q, want arm — a UA that names ARM claims ARM whatever the host is", ua, hostArch, got)
			}
		}
	}
}

// The struct field is not what a detector reads: getHighEntropyValues answers from the
// userAgentMetadata handed to CDP, so the assertion follows the value onto the wire.
// This runs against the real host, which is what makes it the case that would have
// caught the original defect on an Apple Silicon machine.
func TestRotatedPersonaMetadataArchitectureAgreesWithItsUserAgent(t *testing.T) {
	version := "144.0.0.0"
	hostIsAppleSilicon := goruntime.GOOS == "darwin" && goruntime.GOARCH == "arm64"

	for _, platform := range FingerprintPlatforms() {
		ua := ChromeUserAgent(platform, ReducedBrowserVersion(version))
		override := BuildUserAgentOverride(ua, version)
		if override == nil || override.UserAgentMetadata == nil {
			t.Fatalf("%s persona carries no UA-CH metadata, so no architecture reaches the browser", platform)
		}

		want := "x86"
		if platform == PlatformMacOS && hostIsAppleSilicon {
			want = "arm"
		}
		if got := override.UserAgentMetadata.Architecture; got != want {
			t.Errorf("getHighEntropyValues would report architecture %q for the %s persona on this %s/%s host, want %q.\n"+
				"Its UA is %q.", got, platform, goruntime.GOOS, goruntime.GOARCH, want, ua)
		}
	}
}
