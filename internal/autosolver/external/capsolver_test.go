package external

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// --- fakes ---

type fakePage struct {
	url, title, html string
}

func (p *fakePage) URL() string   { return p.url }
func (p *fakePage) Title() string { return p.title }
func (p *fakePage) HTML() (string, error) {
	return p.html, nil
}
func (p *fakePage) HTMLWithin(time.Duration) (string, error) { return p.html, nil }
func (p *fakePage) Screenshot() ([]byte, error)              { return nil, nil }

type fakeExecutor struct {
	lastInject string // last non-read Evaluate expr (the token injection)
	arkoseJSON string // value returned for the window.__ptArkose read
	userAgent  string // value returned for the navigator.userAgent read
}

func (e *fakeExecutor) Click(context.Context, float64, float64) error        { return nil }
func (e *fakeExecutor) Type(context.Context, string) error                   { return nil }
func (e *fakeExecutor) WaitFor(context.Context, string, time.Duration) error { return nil }
func (e *fakeExecutor) Navigate(context.Context, string) error               { return nil }
func (e *fakeExecutor) Evaluate(_ context.Context, expr string, result interface{}) error {
	if strings.Contains(expr, "window.__ptArkose?") { // the capture read, not the token injection
		if sp, ok := result.(*string); ok {
			*sp = e.arkoseJSON
		}
		return nil
	}
	if strings.Contains(expr, "navigator.userAgent") { // the UA read
		if sp, ok := result.(*string); ok {
			*sp = e.userAgent
		}
		return nil
	}
	e.lastInject = expr
	return nil
}

// --- helper tests ---

func TestDetectCaptchaType(t *testing.T) {
	cases := map[string]string{
		`<div class="g-recaptcha" data-sitekey="x"></div>`:                  "recaptcha",
		`<div class="h-captcha" data-sitekey="x"></div>`:                    "hcaptcha",
		`<div class="cf-turnstile" data-sitekey="x"></div>`:                 "turnstile",
		`<div id="arkose" data-pkey="ABC"></div>`:                           "funcaptcha",
		`<script src="https://lnkd-api.arkoselabs.com/v2/api.js"></script>`: "funcaptcha",
		// v3: render=<key> with no data-sitekey widget.
		`<script src="https://www.google.com/recaptcha/api.js?render=6Lc_V3_KEY"></script>`: "recaptcha-v3",
		// v2 wins when a data-sitekey widget is present, even alongside render=explicit.
		`<div class="g-recaptcha" data-sitekey="x"></div><script src="https://www.google.com/recaptcha/api.js?render=explicit"></script>`: "recaptcha",
		`<p>nothing here</p>`: "",
	}
	for html, want := range cases {
		if got := detectCaptchaType(html); got != want {
			t.Errorf("detectCaptchaType(%q) = %q, want %q", html, got, want)
		}
	}
}

func TestExtractSitekey(t *testing.T) {
	if got := extractSitekey(`<div data-sitekey="6LcXYZ"></div>`, "recaptcha"); got != "6LcXYZ" {
		t.Errorf("recaptcha sitekey = %q", got)
	}
	// Detection lowercases, so extraction must be case-insensitive + whitespace-tolerant.
	if got := extractSitekey(`<div Data-SiteKey = '6LcMixed'></div>`, "recaptcha"); got != "6LcMixed" {
		t.Errorf("case-insensitive data-sitekey = %q", got)
	}
	if got := extractSitekey(`<div data-pkey="0152B4EB-D2DC-460A-89A1-629838B529C9"></div>`, "funcaptcha"); got != "0152B4EB-D2DC-460A-89A1-629838B529C9" {
		t.Errorf("funcaptcha data-pkey = %q", got)
	}
	if got := extractSitekey(`<iframe src="https://x.arkoselabs.com/fc/gt2/?pk=0152B4EB-D2DC-460A-89A1-629838B529C9"></iframe>`, "funcaptcha"); got != "0152B4EB-D2DC-460A-89A1-629838B529C9" {
		t.Errorf("funcaptcha pk= url = %q", got)
	}
	// v3 sitekey lives in the api.js ?render= param, not data-sitekey.
	if got := extractSitekey(`<script src="https://www.google.com/recaptcha/api.js?onload=cb&render=6Lc_V3_KEY"></script>`, "recaptcha-v3"); got != "6Lc_V3_KEY" {
		t.Errorf("recaptcha-v3 render sitekey = %q", got)
	}
}

func TestExtractRecaptchaAction(t *testing.T) {
	cases := map[string]string{
		`<script>grecaptcha.execute('6Lc', {action: 'login'})</script>`: "login",
		`grecaptcha.enterprise.execute("6Lc",{action:"submit_form"})`:   "submit_form",
		`<button data-action="checkout"></button>`:                      "checkout", // attribute fallback
		`<p>no action here</p>`:                                         "",         // optional → empty
	}
	for html, want := range cases {
		if got := extractRecaptchaAction(html); got != want {
			t.Errorf("extractRecaptchaAction(%q) = %q, want %q", html, got, want)
		}
	}
}

func TestExtractArkoseSubdomain(t *testing.T) {
	html := `<script src="https://lnkd-api.arkoselabs.com/v2/0152/api.js"></script>`
	if got := extractArkoseSubdomain(html); got != "https://lnkd-api.arkoselabs.com" {
		t.Errorf("arkose subdomain = %q", got)
	}
}

func TestCapsolverTaskType(t *testing.T) {
	cases := map[string]string{
		"recaptcha":    "ReCaptchaV2TaskProxyLess",
		"recaptcha-v3": "ReCaptchaV3TaskProxyLess",
		"hcaptcha":     "HCaptchaTaskProxyLess",
		"turnstile":    "AntiTurnstileTaskProxyLess",
		"funcaptcha":   "FunCaptchaTaskProxyLess",
	}
	for in, want := range cases {
		got, ok := capsolverTaskType(in)
		if !ok || got != want {
			t.Errorf("capsolverTaskType(%q) = %q,%v want %q", in, got, ok, want)
		}
	}
	if _, ok := capsolverTaskType("bogus"); ok {
		t.Error("expected bogus type to be unsupported")
	}
}

// --- end-to-end Solve against a mock Capsolver API ---

// mockCapsolver returns a server that answers createTask with an inline
// ready solution, and records the last task it received.
func mockCapsolver(t *testing.T, token string, gotTask *capsolverTask) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/createTask") {
			var req capsolverCreateRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			*gotTask = req.Task
			resp := capsolverResponse{Status: "ready"}
			if strings.Contains(req.Task.Type, "FunCaptcha") {
				resp.Solution.Token = token
			} else {
				resp.Solution.GRecaptchaResponse = token
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.Error(w, "not found", 404)
	}))
}

// mockCapsolverPolling answers createTask with a taskId only (no inline
// solution), then getTaskResult returns "processing" until the Nth poll, when
// it returns "ready" with the token — exercising the real poll loop.
func mockCapsolverPolling(t *testing.T, token string, readyOnPoll int) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	polls := 0
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/createTask") {
			_ = json.NewEncoder(w).Encode(capsolverResponse{TaskID: "task-123"})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/getTaskResult") {
			mu.Lock()
			polls++
			n := polls
			mu.Unlock()
			resp := capsolverResponse{TaskID: "task-123", Status: "processing"}
			if n >= readyOnPoll {
				resp.Status = "ready"
				resp.Solution.GRecaptchaResponse = token
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.Error(w, "not found", 404)
	}))
}

func TestSolveRecaptchaPolling(t *testing.T) {
	srv := mockCapsolverPolling(t, "POLLED-TOKEN", 3) // ready on the 3rd getTaskResult
	defer srv.Close()

	c := NewCapsolver(CapsolverConfig{APIKey: "k", BaseURL: srv.URL, PollInterval: 2 * time.Millisecond})
	page := &fakePage{url: "https://ex.com", html: `<div class="g-recaptcha" data-sitekey="6LcABC"></div>`}
	exec := &fakeExecutor{}

	res, err := c.Solve(context.Background(), page, exec)
	if err != nil {
		t.Fatalf("Solve: %v", err)
	}
	if !res.Solved {
		t.Fatalf("expected solved via polling, got error=%q", res.Error)
	}
	if !strings.Contains(exec.lastInject, "POLLED-TOKEN") {
		t.Errorf("polled token not injected: %q", exec.lastInject)
	}
}

// TestSolveCapsolverFailedStatus also guards against the poll loop hanging on a
// terminal failure: with the bug, status=="failed" (errorId 0) loops forever and
// this test (ctx.Background, never cancelled) would time out instead of failing.
func TestSolveCapsolverFailedStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/createTask") {
			_ = json.NewEncoder(w).Encode(capsolverResponse{TaskID: "t1"})
			return
		}
		_ = json.NewEncoder(w).Encode(capsolverResponse{TaskID: "t1", Status: "failed", ErrorCode: "ERROR_CAPTCHA_UNSOLVABLE"})
	}))
	defer srv.Close()

	c := NewCapsolver(CapsolverConfig{APIKey: "k", BaseURL: srv.URL, PollInterval: 2 * time.Millisecond})
	page := &fakePage{url: "https://ex.com", html: `<div class="g-recaptcha" data-sitekey="6LcABC"></div>`}

	res, err := c.Solve(context.Background(), page, &fakeExecutor{})
	if err == nil || res.Solved {
		t.Fatalf("expected failure on status=failed, got solved=%v err=%v", res.Solved, err)
	}
	if !strings.Contains(res.Error, "failed") {
		t.Errorf("expected 'failed' in error, got %q", res.Error)
	}
}

func TestSolveCapsolverCreateTaskError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(capsolverResponse{ErrorID: 1, ErrorCode: "ERROR_KEY_DENIED_ACCESS", ErrorDescription: "bad key"})
	}))
	defer srv.Close()

	c := NewCapsolver(CapsolverConfig{APIKey: "k", BaseURL: srv.URL})
	page := &fakePage{url: "https://ex.com", html: `<div class="g-recaptcha" data-sitekey="6LcABC"></div>`}

	res, err := c.Solve(context.Background(), page, &fakeExecutor{})
	if err == nil || res.Solved {
		t.Fatalf("expected createTask error, got solved=%v err=%v", res.Solved, err)
	}
}

func TestSolveRecaptcha(t *testing.T) {
	var task capsolverTask
	srv := mockCapsolver(t, "RECAP-TOKEN", &task)
	defer srv.Close()

	c := NewCapsolver(CapsolverConfig{APIKey: "k", BaseURL: srv.URL})
	page := &fakePage{url: "https://ex.com", html: `<div class="g-recaptcha" data-sitekey="6LcABC"></div>`}
	exec := &fakeExecutor{}

	res, err := c.Solve(context.Background(), page, exec)
	if err != nil {
		t.Fatalf("Solve: %v", err)
	}
	if !res.Solved {
		t.Fatalf("expected solved, got error=%q", res.Error)
	}
	if task.Type != "ReCaptchaV2TaskProxyLess" || task.WebsiteKey != "6LcABC" {
		t.Errorf("task = %+v", task)
	}
	if task.WebsitePublicKey != "" {
		t.Errorf("recaptcha task should not set websitePublicKey")
	}
	if !strings.Contains(exec.lastInject, "RECAP-TOKEN") {
		t.Errorf("token not injected: %q", exec.lastInject)
	}
}

func TestSolveRecaptchaV3(t *testing.T) {
	var task capsolverTask
	srv := mockCapsolver(t, "V3-TOKEN", &task)
	defer srv.Close()

	c := NewCapsolver(CapsolverConfig{APIKey: "k", BaseURL: srv.URL})
	page := &fakePage{
		url:  "https://ex.com/login",
		html: `<script src="https://www.google.com/recaptcha/api.js?render=6Lc_V3_KEY"></script><script>grecaptcha.execute('6Lc_V3_KEY', {action: 'login'});</script>`,
	}
	exec := &fakeExecutor{}

	res, err := c.Solve(context.Background(), page, exec)
	if err != nil {
		t.Fatalf("Solve: %v", err)
	}
	if !res.Solved {
		t.Fatalf("expected solved, got error=%q", res.Error)
	}
	if task.Type != "ReCaptchaV3TaskProxyLess" {
		t.Errorf("task type = %q", task.Type)
	}
	if task.WebsiteKey != "6Lc_V3_KEY" {
		t.Errorf("websiteKey = %q (want render sitekey)", task.WebsiteKey)
	}
	if task.PageAction != "login" {
		t.Errorf("pageAction = %q (want login)", task.PageAction)
	}
	if task.WebsitePublicKey != "" {
		t.Errorf("v3 task should not set websitePublicKey")
	}
	if !strings.Contains(exec.lastInject, "V3-TOKEN") {
		t.Errorf("token not injected: %q", exec.lastInject)
	}
}

func TestSolveFuncaptcha(t *testing.T) {
	var task capsolverTask
	srv := mockCapsolver(t, "FC-TOKEN", &task)
	defer srv.Close()

	c := NewCapsolver(CapsolverConfig{APIKey: "k", BaseURL: srv.URL})
	page := &fakePage{
		url:  "https://www.linkedin.com/checkpoint",
		html: `<div data-pkey="0152B4EB-D2DC-460A-89A1-629838B529C9"></div><script src="https://lnkd-api.arkoselabs.com/v2/api.js"></script>`,
	}
	exec := &fakeExecutor{userAgent: "Mozilla/5.0 (TestUA)"}

	res, err := c.Solve(context.Background(), page, exec)
	if err != nil {
		t.Fatalf("Solve: %v", err)
	}
	if !res.Solved {
		t.Fatalf("expected solved, got error=%q", res.Error)
	}
	if task.Type != "FunCaptchaTaskProxyLess" {
		t.Errorf("task type = %q", task.Type)
	}
	if task.UserAgent != "Mozilla/5.0 (TestUA)" {
		t.Errorf("browser UA should be forwarded; got %q", task.UserAgent)
	}
	if task.WebsitePublicKey != "0152B4EB-D2DC-460A-89A1-629838B529C9" {
		t.Errorf("websitePublicKey = %q", task.WebsitePublicKey)
	}
	if task.WebsiteKey != "" {
		t.Errorf("funcaptcha task should not set websiteKey")
	}
	if task.FuncaptchaApiJSSubdomain != "https://lnkd-api.arkoselabs.com" {
		t.Errorf("funcaptchaApiJSSubdomain = %q", task.FuncaptchaApiJSSubdomain)
	}
	if task.Data != "" {
		t.Errorf("no blob captured, expected empty data, got %q", task.Data)
	}
	if !strings.Contains(exec.lastInject, "FC-TOKEN") {
		t.Errorf("token not injected: %q", exec.lastInject)
	}
}

// TestSolveFuncaptchaWithBlob verifies the document-start hook's captured
// blob/pk/surl (window.__ptArkose) override static extraction and are forwarded
// to CapSolver as the task data.
func TestSolveFuncaptchaWithBlob(t *testing.T) {
	var task capsolverTask
	srv := mockCapsolver(t, "FC-TOKEN", &task)
	defer srv.Close()

	c := NewCapsolver(CapsolverConfig{APIKey: "k", BaseURL: srv.URL})
	page := &fakePage{
		url:  "https://www.linkedin.com/checkpoint",
		html: `<div data-pkey="STATIC-PK"></div><script src="https://lnkd-api.arkoselabs.com/v2/api.js"></script>`,
	}
	exec := &fakeExecutor{arkoseJSON: `{"blob":"BDA-BLOB-XYZ","pk":"LIVE-PK","surl":"https://lnkd-api.arkoselabs.com"}`}

	res, err := c.Solve(context.Background(), page, exec)
	if err != nil || !res.Solved {
		t.Fatalf("Solve: err=%v solved=%v error=%q", err, res.Solved, res.Error)
	}
	if task.WebsitePublicKey != "LIVE-PK" {
		t.Errorf("captured pk should override static; got %q", task.WebsitePublicKey)
	}
	// Assert the JSON shape, not just substring — a wrong wrapper key must fail.
	var dataObj map[string]string
	if err := json.Unmarshal([]byte(task.Data), &dataObj); err != nil {
		t.Fatalf("task.Data is not valid JSON: %q (%v)", task.Data, err)
	}
	if dataObj["blob"] != "BDA-BLOB-XYZ" {
		t.Errorf("expected data.blob=BDA-BLOB-XYZ, got %+v", dataObj)
	}
}
