package handlers

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var namesRoute = regexp.MustCompile(`\b(GET|POST|PUT|PATCH|DELETE) /[A-Za-z{]`)

var bareRoutePattern = regexp.MustCompile(`^(GET|POST|PUT|PATCH|DELETE) /\S*$`)

var routeNamingProseRecorded = map[string]string{
	"actions.go|send this as POST /action with a JSON body": "names the same verb's body form for an HTTP caller who already chose the raw query route; the CLI cannot produce that request, so there is no pinchtab verb to name",
}

func routeNamingProseLiterals(t *testing.T) map[string]string {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("no handler sources found: %v", err)
	}
	found := map[string]string{}
	fset := token.NewFileSet()
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			lit, ok := node.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			text, err := strconv.Unquote(lit.Value)
			if err != nil || !namesRoute.MatchString(text) || bareRoutePattern.MatchString(text) {
				return true
			}
			found[file+":"+strconv.Itoa(fset.Position(lit.Pos()).Line)] = text
			return true
		})
	}
	if len(found) == 0 {
		t.Fatal("no prose literal in the package names a METHOD /path; the walk stopped matching and this census would pass over nothing")
	}
	return found
}

func recordedReason(file, text string) (string, string) {
	for key, reason := range routeNamingProseRecorded {
		recordedFile, needle, _ := strings.Cut(key, "|")
		if recordedFile == filepath.Base(file) && strings.Contains(text, needle) {
			return key, reason
		}
	}
	return "", ""
}

func TestNoHintNamesABareRouteAsTheThingToCall(t *testing.T) {
	used := map[string]bool{}
	for site, text := range routeNamingProseLiterals(t) {
		file, _, _ := strings.Cut(site, ":")
		if key, _ := recordedReason(file, text); key != "" {
			used[key] = true
			continue
		}
		if strings.Contains(text, "{id}") {
			t.Errorf("%s names a route with an unsubstituted {id} placeholder: %q; render the real tab id, or record the literal with a reason", site, text)
			continue
		}
		if !strings.Contains(text, "pinchtab ") {
			t.Errorf("%s tells the caller to call a bare route: %q; name the pinchtab verb beside it, or record the literal with the reason no verb exists", site, text)
		}
	}
	for key, reason := range routeNamingProseRecorded {
		if reason == "" {
			t.Errorf("%q is recorded with no reason", key)
		}
		if !used[key] {
			t.Errorf("%q is recorded but no literal matches it any more; the record outlived its code and now permits the next bare-route hint", key)
		}
	}
}
