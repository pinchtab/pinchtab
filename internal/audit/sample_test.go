package audit

import (
	"reflect"
	"testing"
)

var fixtureSiteURLs = []string{
	"http://fixtures/audit-site/index.html",
	"http://fixtures/audit-site/broken-assets.html",
	"http://fixtures/audit-site/console-errors.html",
	"http://fixtures/audit-site/a11y-issues.html",
	"http://fixtures/audit-site/clean.html",
	"http://fixtures/audit-site/forms.html",
	"http://fixtures/audit-site/cookie-echo.html",
	"http://fixtures/audit-site/products/p1.html",
	"http://fixtures/audit-site/products/p2.html",
	"http://fixtures/audit-site/products/p3.html",
	"http://fixtures/audit-site/products/p4.html",
	"http://fixtures/audit-site/products/p5.html",
}

func TestGroupURLsCollapsesProductTemplates(t *testing.T) {
	groups := GroupURLs(fixtureSiteURLs)

	var products *PageGroup
	multi := 0
	for i := range groups {
		if len(groups[i].URLs) >= 2 {
			multi++
			products = &groups[i]
		}
	}
	if multi != 1 {
		t.Fatalf("template groups = %d, want 1 (products only)", multi)
	}
	if len(products.URLs) != 5 {
		t.Errorf("products group size = %d, want 5", len(products.URLs))
	}
	if products.Template != "http://fixtures/audit-site/products/p#.html" {
		t.Errorf("products template = %q", products.Template)
	}
}

func TestGroupURLsKeepsDistinctPagesApart(t *testing.T) {
	groups := GroupURLs([]string{
		"http://x/index.html",
		"http://x/clean.html",
		"http://x/forms.html",
	})
	if len(groups) != 3 {
		t.Errorf("groups = %d, want 3 distinct (letters differ, not digits)", len(groups))
	}
}

func TestSamplePagesDeterministicPicks(t *testing.T) {
	first := SamplePages(fixtureSiteURLs, 2, nil)
	for range 10 {
		if again := SamplePages(fixtureSiteURLs, 2, nil); !reflect.DeepEqual(first, again) {
			t.Fatalf("sampling not deterministic:\n%v\n%v", first, again)
		}
	}

	products := 0
	for _, u := range first {
		if u == "http://fixtures/audit-site/products/p1.html" || u == "http://fixtures/audit-site/products/p2.html" {
			products++
		}
	}
	if products != 2 {
		t.Errorf("expected the 2 lexically-first product pages, got plan %v", first)
	}
	if len(first) != 9 {
		t.Errorf("plan size = %d, want 9 (7 non-group + 2 sampled)", len(first))
	}
}

func TestSamplePagesHomepageFirst(t *testing.T) {
	plan := SamplePages(fixtureSiteURLs, 2, nil)
	if plan[0] != "http://fixtures/audit-site/index.html" {
		t.Errorf("plan[0] = %q, want homepage", plan[0])
	}

	shuffled := []string{
		"http://fixtures/audit-site/products/p3.html",
		"http://fixtures/audit-site/index.html",
		"http://fixtures/audit-site/products/p1.html",
	}
	plan = SamplePages(shuffled, 1, nil)
	if plan[0] != "http://fixtures/audit-site/products/p3.html" {
		t.Errorf("entry URL must stay first even when grouped, got %v", plan)
	}
	if len(plan) != 2 {
		t.Errorf("plan = %v, want entry + index (entry fills the group quota)", plan)
	}
}

func TestSamplePagesTemplatePagesLast(t *testing.T) {
	plan := SamplePages(fixtureSiteURLs, 2, nil)
	sawProduct := false
	for _, u := range plan[1:] {
		isProduct := templateKey(u) == "http://fixtures/audit-site/products/p#.html"
		if sawProduct && !isProduct {
			t.Errorf("ungrouped page %q after template pages: %v", u, plan)
		}
		if isProduct {
			sawProduct = true
		}
	}
}

func TestSamplePagesNoCapKeepsAll(t *testing.T) {
	plan := SamplePages(fixtureSiteURLs, 0, nil)
	if len(plan) != len(fixtureSiteURLs) {
		t.Errorf("plan size = %d, want %d (no cap)", len(plan), len(fixtureSiteURLs))
	}
}

func TestSamplePagesExternalGroups(t *testing.T) {
	urls := []string{"http://x/home", "http://x/shoes", "http://x/hats"}
	external := []PageGroup{{Template: "catalog", URLs: []string{"http://x/shoes", "http://x/hats"}}}
	plan := SamplePages(urls, 1, external)
	want := []string{"http://x/home", "http://x/hats"}
	if !reflect.DeepEqual(plan, want) {
		t.Errorf("plan = %v, want %v (external group sampled)", plan, want)
	}
}

func TestSamplePagesEmpty(t *testing.T) {
	if plan := SamplePages(nil, 2, nil); plan != nil {
		t.Errorf("SamplePages(nil) = %v", plan)
	}
}
