package contentguard

import (
	"net/http/httptest"
	"testing"

	"github.com/pinchtab/pinchtab/internal/idpi"
)

// mockGuard implements idpi.Guard for testing.
type mockGuard struct {
	scanResult idpi.CheckResult
	wrapResult string
	enabled    bool
}

func (m *mockGuard) ScanContent(_ string) idpi.CheckResult { return m.scanResult }
func (m *mockGuard) CheckDomain(_ string) idpi.CheckResult { return idpi.CheckResult{} }
func (m *mockGuard) DomainAllowed(_ string) bool           { return true }
func (m *mockGuard) WrapContent(text, _ string) string {
	if m.wrapResult != "" {
		return m.wrapResult
	}
	return "<wrapped>" + text + "</wrapped>"
}
func (m *mockGuard) Enabled() bool { return m.enabled }

func TestScan_NilScanner(t *testing.T) {
	var s *Scanner
	r := s.Scan("hello", "http://example.com")
	if r.Text != "hello" {
		t.Fatalf("expected text unchanged, got %q", r.Text)
	}
	if r.Blocked || r.Warning != "" {
		t.Fatal("expected no block/warning on nil scanner")
	}
}

func TestScan_DisabledGuard(t *testing.T) {
	s := &Scanner{Guard: &mockGuard{enabled: false}, WrapEnabled: true}
	r := s.Scan("hello", "http://example.com")
	if r.Text != "hello" {
		t.Fatalf("expected text unchanged, got %q", r.Text)
	}
}

func TestScan_NilGuard(t *testing.T) {
	s := &Scanner{Guard: nil, WrapEnabled: true}
	r := s.Scan("hello", "http://example.com")
	if r.Text != "hello" {
		t.Fatalf("expected text unchanged, got %q", r.Text)
	}
}

func TestScan_Blocked(t *testing.T) {
	s := &Scanner{
		Guard: &mockGuard{
			enabled:    true,
			scanResult: idpi.CheckResult{Threat: true, Blocked: true, Reason: "injection detected"},
		},
		WrapEnabled: true,
	}
	r := s.Scan("malicious", "http://evil.com")
	if !r.Blocked {
		t.Fatal("expected blocked")
	}
	if r.BlockReason != "injection detected" {
		t.Fatalf("expected reason, got %q", r.BlockReason)
	}
	// Text should not be wrapped when blocked
	if r.Text != "malicious" {
		t.Fatalf("expected original text on block, got %q", r.Text)
	}
}

func TestScan_ThreatNotBlocked(t *testing.T) {
	s := &Scanner{
		Guard: &mockGuard{
			enabled:    true,
			scanResult: idpi.CheckResult{Threat: true, Blocked: false, Reason: "suspicious", Pattern: "eval("},
		},
		WrapEnabled: false,
	}
	r := s.Scan("code", "http://example.com")
	if r.Blocked {
		t.Fatal("should not be blocked")
	}
	if r.Warning != "suspicious" {
		t.Fatalf("expected warning, got %q", r.Warning)
	}
	if r.Pattern != "eval(" {
		t.Fatalf("expected pattern, got %q", r.Pattern)
	}
	if r.Text != "code" {
		t.Fatalf("expected unwrapped text, got %q", r.Text)
	}
}

func TestScan_WrapEnabled_WithThreat(t *testing.T) {
	s := &Scanner{
		Guard: &mockGuard{
			enabled:    true,
			scanResult: idpi.CheckResult{Threat: true, Blocked: false, Reason: "warn"},
		},
		WrapEnabled: true,
	}
	r := s.Scan("hello", "http://example.com")
	if r.Text != "<wrapped>hello</wrapped>" {
		t.Fatalf("expected wrapped text, got %q", r.Text)
	}
	if r.Warning != "warn" {
		t.Fatalf("expected warning, got %q", r.Warning)
	}
}

func TestScan_WrapEnabled_NoThreat(t *testing.T) {
	s := &Scanner{
		Guard: &mockGuard{
			enabled:    true,
			scanResult: idpi.CheckResult{}, // no threat
		},
		WrapEnabled: true,
	}
	r := s.Scan("safe content", "http://example.com")
	// Wrapping is unconditional when enabled
	if r.Text != "<wrapped>safe content</wrapped>" {
		t.Fatalf("expected wrapped text even without threat, got %q", r.Text)
	}
	if r.Blocked || r.Warning != "" {
		t.Fatal("expected no block/warning")
	}
}

func TestScan_WrapEnabled_AlwaysWraps(t *testing.T) {
	// Security invariant: WrapEnabled=true should wrap content EVEN when no threat
	// is detected. This is the trust-boundary wrapping behavior that ensures all
	// content passing through the pipeline is wrapped regardless of scan results.
	s := &Scanner{
		Guard: &mockGuard{
			enabled:    true,
			scanResult: idpi.CheckResult{Threat: false, Blocked: false}, // explicitly no threat
		},
		WrapEnabled: true,
	}
	r := s.Scan("completely safe content", "http://safe.example.com")
	if r.Blocked {
		t.Fatal("should not be blocked")
	}
	if r.Warning != "" {
		t.Fatalf("expected no warning, got %q", r.Warning)
	}
	// The key invariant: wrapping happens unconditionally when WrapEnabled=true
	if r.Text != "<wrapped>completely safe content</wrapped>" {
		t.Fatalf("expected wrapping even without threat, got %q", r.Text)
	}
}

func TestScanOnly_NeverWraps(t *testing.T) {
	s := &Scanner{
		Guard: &mockGuard{
			enabled:    true,
			scanResult: idpi.CheckResult{Threat: true, Reason: "warn"},
		},
		WrapEnabled: true, // even with wrap enabled, ScanOnly should not wrap
	}
	r := s.ScanOnly("text")
	if r.Text != "text" {
		t.Fatalf("ScanOnly should not wrap, got %q", r.Text)
	}
	if r.Warning != "warn" {
		t.Fatalf("expected warning, got %q", r.Warning)
	}
}

func TestScanOnly_Blocked(t *testing.T) {
	s := &Scanner{
		Guard: &mockGuard{
			enabled:    true,
			scanResult: idpi.CheckResult{Threat: true, Blocked: true, Reason: "blocked"},
		},
	}
	r := s.ScanOnly("x")
	if !r.Blocked {
		t.Fatal("expected blocked")
	}
	if r.BlockReason != "blocked" {
		t.Fatalf("expected reason, got %q", r.BlockReason)
	}
}

func TestSetHeaders_Warning(t *testing.T) {
	w := httptest.NewRecorder()
	r := &Result{Warning: "injection hint", Pattern: "eval("}
	r.SetHeaders(w)
	if got := w.Header().Get("X-IDPI-Warning"); got != "injection hint" {
		t.Fatalf("expected warning header, got %q", got)
	}
	if got := w.Header().Get("X-IDPI-Pattern"); got != "eval(" {
		t.Fatalf("expected pattern header, got %q", got)
	}
}

func TestSetHeaders_NoWarning(t *testing.T) {
	w := httptest.NewRecorder()
	r := &Result{Text: "clean"}
	r.SetHeaders(w)
	if got := w.Header().Get("X-IDPI-Warning"); got != "" {
		t.Fatalf("expected no warning header, got %q", got)
	}
}

func TestSetHeaders_NilResult(t *testing.T) {
	w := httptest.NewRecorder()
	var r *Result
	r.SetHeaders(w) // should not panic
	if got := w.Header().Get("X-IDPI-Warning"); got != "" {
		t.Fatalf("expected no header on nil result, got %q", got)
	}
}
