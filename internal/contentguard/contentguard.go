// Package contentguard consolidates the repeated IDPI scan/block/warn/wrap
// pattern from handler files into a single reusable service.
package contentguard

import (
	"net/http"

	"github.com/pinchtab/pinchtab/internal/idpi"
)

// Scanner wraps an idpi.Guard and provides convenience methods that
// encapsulate the scan→block/warn→wrap logic previously duplicated
// across multiple HTTP handlers.
type Scanner struct {
	Guard       idpi.Guard
	WrapEnabled bool
}

// Result holds the outcome of a content scan.
type Result struct {
	Text        string // processed text (wrapped if WrapEnabled + guard active)
	Blocked     bool
	BlockReason string
	Warning     string // non-empty when Threat && !Blocked
	Pattern     string // matched pattern (if any)
}

// Scan runs IDPI content scanning on text. If WrapEnabled and content is not
// blocked, wraps the text with trust-boundary markers.
func (s *Scanner) Scan(text, pageURL string) *Result {
	if s == nil || s.Guard == nil || !s.Guard.Enabled() {
		return &Result{Text: text}
	}
	cr := s.Guard.ScanContent(text)
	r := &Result{Text: text}
	if cr.Blocked {
		r.Blocked = true
		r.BlockReason = cr.Reason
		return r
	}
	if cr.Threat {
		r.Warning = cr.Reason
		r.Pattern = cr.Pattern
	}
	if s.WrapEnabled {
		r.Text = s.Guard.WrapContent(text, pageURL)
	}
	return r
}

// ScanOnly runs scanning without wrapping (useful when wrapping is handled
// separately by the caller for format-specific output).
func (s *Scanner) ScanOnly(text string) *Result {
	if s == nil || s.Guard == nil || !s.Guard.Enabled() {
		return &Result{Text: text}
	}
	cr := s.Guard.ScanContent(text)
	r := &Result{Text: text}
	if cr.Blocked {
		r.Blocked = true
		r.BlockReason = cr.Reason
		return r
	}
	if cr.Threat {
		r.Warning = cr.Reason
		r.Pattern = cr.Pattern
	}
	return r
}

// SetHeaders writes IDPI warning headers to the response when a threat was
// detected but not blocked.
func (r *Result) SetHeaders(w http.ResponseWriter) {
	if r == nil {
		return
	}
	if r.Warning != "" {
		w.Header().Set("X-IDPI-Warning", r.Warning)
	}
	if r.Pattern != "" {
		w.Header().Set("X-IDPI-Pattern", r.Pattern)
	}
}
