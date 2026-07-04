package audit

import (
	"testing"
)

func rulesOf(findings []SecurityFinding) []string {
	var rules []string
	for _, f := range findings {
		rules = append(rules, f.RuleID)
	}
	return rules
}

func hasRule(findings []SecurityFinding, rule string) bool {
	for _, f := range findings {
		if f.RuleID == rule {
			return true
		}
	}
	return false
}

func TestMixedContentRule(t *testing.T) {
	requests := []NetworkRequest{
		{URL: "https://site.example/page", Status: 200},
		{URL: "http://site.example/legacy.js", Status: 200},
		{URL: "https://cdn.example/app.css", Status: 200},
	}
	findings := EvaluateSecurity("https://site.example/page", "Page", requests, nil)
	if !hasRule(findings, "mixed-content") {
		t.Fatalf("findings = %v, want mixed-content", rulesOf(findings))
	}
	for _, f := range findings {
		if f.RuleID == "mixed-content" {
			if f.Severity != "high" || f.URL != "https://site.example/page" {
				t.Errorf("mixed-content finding = %+v", f)
			}
		}
	}

	httpPage := EvaluateSecurity("http://site.example/page", "Page", requests, nil)
	if hasRule(httpPage, "mixed-content") {
		t.Error("mixed-content must not fire for http pages")
	}
}

func TestInsecureFormRules(t *testing.T) {
	forms := []FormFact{
		{Action: "http://site.example/login", Method: "post", HasPassword: true},
		{Action: "http://site.example/search", Method: "get", HasPassword: false},
		{Action: "https://site.example/safe", Method: "post", HasPassword: true},
	}

	fromHTTPS := EvaluateSecurity("https://site.example/", "Page", nil, forms)
	if !hasRule(fromHTTPS, "insecure-password-form") || !hasRule(fromHTTPS, "insecure-form-action") {
		t.Errorf("https page findings = %v", rulesOf(fromHTTPS))
	}

	fromHTTP := EvaluateSecurity("http://site.example/", "Page", nil, forms)
	if !hasRule(fromHTTP, "insecure-password-form") {
		t.Errorf("password-over-http must fire regardless of page scheme, got %v", rulesOf(fromHTTP))
	}
	if hasRule(fromHTTP, "insecure-form-action") {
		t.Error("insecure-form-action only applies to https pages")
	}

	clean := EvaluateSecurity("https://site.example/", "Page", nil, []FormFact{
		{Action: "https://site.example/login", Method: "post", HasPassword: true},
	})
	if len(clean) != 0 {
		t.Errorf("https form should yield no findings, got %v", rulesOf(clean))
	}
}

func TestExposedEndpointRule(t *testing.T) {
	requests := []NetworkRequest{
		{URL: "https://site.example/.env", Status: 200},
		{URL: "https://site.example/.git/config", Status: 301},
		{URL: "https://site.example/missing/.env", Status: 404},
		{URL: "https://site.example/page?file=.env", Status: 200},
		{URL: "https://site.example/app.js", Status: 200},
	}
	findings := EvaluateSecurity("https://site.example/", "Page", requests, nil)
	count := 0
	for _, f := range findings {
		if f.RuleID == "exposed-endpoint" {
			count++
			if f.Severity != "medium" {
				t.Errorf("exposed-endpoint severity = %q", f.Severity)
			}
		}
	}
	if count != 2 {
		t.Errorf("exposed-endpoint count = %d, want 2 (.env 200 and .git 301; 404 and query-only excluded)", count)
	}
}

func TestDirectoryListingRule(t *testing.T) {
	findings := EvaluateSecurity("https://site.example/files/", "Index of /files", nil, nil)
	if !hasRule(findings, "directory-listing") {
		t.Errorf("findings = %v, want directory-listing", rulesOf(findings))
	}
	if f := EvaluateSecurity("https://site.example/", "Indexing 101", nil, nil); len(f) != 0 {
		t.Errorf("title mentioning indexing should not fire, got %v", rulesOf(f))
	}
}

func TestCleanPageYieldsNoFindings(t *testing.T) {
	requests := []NetworkRequest{{URL: "https://site.example/app.css", Status: 200}}
	forms := []FormFact{{Action: "https://site.example/search", Method: "get"}}
	if f := EvaluateSecurity("https://site.example/", "Clean Page", requests, forms); len(f) != 0 {
		t.Errorf("clean page findings = %v, want none", rulesOf(f))
	}
}
