package ghostchrome

import (
	"fmt"
	"strings"
)

// QualityThreshold is the minimum quality score for accepting a Ghost result.
const QualityThreshold = 60

// GhostResult holds the outcome of a Ghost static fetch attempt.
type GhostResult struct {
	OK           bool
	Content      string
	Title        string
	URL          string
	Quality      int
	NeedsBrowser bool
	IsBlocked    bool
	IsThin       bool
	PageClass    string // "static", "unknown"
	SkipReason   string // non-empty when Ghost decided not to try
}

// ShouldAccept returns true when the ghost result is good enough to use
// without escalating to a full browser.
func (r *GhostResult) ShouldAccept() bool {
	if !r.OK {
		return false
	}
	if r.NeedsBrowser {
		return false
	}
	if r.IsBlocked {
		return false
	}
	if r.IsThin {
		return false
	}
	switch r.PageClass {
	case "spa", "dynamic", "blocked":
		return false
	}
	return r.Quality >= QualityThreshold
}

// FormatReason returns a human-readable explanation of why the result
// was skipped or what signals were observed.
func (r *GhostResult) FormatReason() string {
	if r.SkipReason != "" {
		return r.SkipReason
	}
	return fmt.Sprintf("quality=%d needsBrowser=%t pageClass=%s", r.Quality, r.NeedsBrowser, r.PageClass)
}

// SnapshotNode holds the minimal fields needed to assess a snapshot node's quality.
type SnapshotNode struct {
	Role string
	Name string
}

// AssessContent evaluates pre-fetched text content against ghost quality criteria.
// It returns a GhostResult indicating whether the content is rich enough to serve.
func AssessContent(content string) *GhostResult {
	result := &GhostResult{OK: true, Content: content, PageClass: "static"}
	result.Quality = EstimateQuality(content)
	result.IsThin = result.Quality < 20
	result.NeedsBrowser = LooksLikeSPA(content)
	if result.NeedsBrowser {
		result.PageClass = "spa"
	}
	return result
}

// AssessSnapshot returns true when the snapshot nodes are rich enough to serve.
// Thin snapshots (fewer than 3 nodes) or those with only generic containers
// are rejected so that the request escalates to Chrome.
func AssessSnapshot(nodes []SnapshotNode) bool {
	if len(nodes) < 3 {
		return false
	}
	for _, n := range nodes {
		switch n.Role {
		case "generic", "none", "":
			continue
		default:
			return true
		}
	}
	return false
}

// LooksLikeSPA returns true when the content appears to be an SPA shell
// that needs a real browser to render meaningful content.
func LooksLikeSPA(content string) bool {
	words := len(strings.Fields(content))
	if words > 100 {
		return false
	}
	lower := strings.ToLower(content)
	markers := []string{
		"id=\"__next\"",
		"id=\"root\"",
		"id=\"app\"",
		"<noscript>",
		"window.__initial",
	}
	for _, m := range markers {
		if strings.Contains(lower, m) {
			return true
		}
	}
	return false
}

// EstimateQuality returns a 0-100 score based on content richness.
func EstimateQuality(content string) int {
	words := len(strings.Fields(content))
	if words == 0 {
		return 0
	}
	if words < 50 {
		return 20
	}
	if words < 200 {
		return 50
	}
	return 80
}
