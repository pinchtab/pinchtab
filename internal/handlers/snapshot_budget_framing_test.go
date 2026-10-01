package handlers

import (
	"fmt"
	"testing"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/idpi"
)

// framingProbeNodes is a page's worth of compact nodes: small individually, which
// is exactly the case where a header is not a rounding error.
func framingProbeNodes(count int) []bridge.A11yNode {
	nodes := make([]bridge.A11yNode, 0, count)
	for i := range count {
		nodes = append(nodes, bridge.A11yNode{
			Ref:  fmt.Sprintf("e%d", i),
			Role: []string{"heading", "link", "button", "textbox"}[i%4],
			Name: "Compare plans and pricing",
		})
	}
	return nodes
}

// maxTokens is documented as a ceiling on the response. It was a ceiling on the
// part of the response made of nodes: the tree was fitted to the budget and the
// header was then written on top of it.
//
// The header carries the page title and URL, so the overshoot is page-dependent
// rather than a constant. Measured through a real browser on a page with an
// ordinary marketing title and a nested path, a budget of 100 returned ~142
// tokens - 182 bytes of header against 386 bytes of nodes that were themselves
// correctly under budget.
//
// This drives the same two pieces the handler does, at the same sizes, and holds
// the whole reply to the number that was asked for.
func TestSnapshotBudgetCoversTheFramingNotJustTheNodes(t *testing.T) {
	h := &Handlers{
		Config:    &config.RuntimeConfig{},
		IDPIGuard: idpi.NewGuard(config.IDPIConfig{}, nil),
	}
	const (
		title = "Enterprise Platform Pricing, Plans and Frequently Asked Questions"
		url   = "https://example.com/a-fairly-long-marketing-path/with-nested-segments/"
	)
	nodes := framingProbeNodes(80)

	for _, format := range []string{"compact", "text"} {
		for _, budget := range []int{100, 150, 300, 600} {
			t.Run(fmt.Sprintf("%s/%d", format, budget), func(t *testing.T) {
				nodeBudget := h.snapshotNodeBudget(format, "", title, url, len(nodes), nil, "", nil, budget)
				kept, truncated := bridge.TruncateToTokens(nodes, nodeBudget, format)

				header := snapshotCompactHeader(title, url, len(kept), nil)
				if format == "text" {
					header = snapshotTextHeader(title, url, len(kept), nil)
				}
				framing := snapshotFraming(format, header, "", nil, truncated, budget)

				var body string
				if format == "text" {
					body = bridge.FormatSnapshotText(kept)
				} else {
					body = bridge.FormatSnapshotCompact(kept)
				}

				total := estimateSnapshotTokens(len(framing) + len(body))
				if total > budget {
					t.Errorf("%s budget=%d: reply is ~%d tokens (%d%% of budget) - framing ~%d + nodes ~%d; the budget is a ceiling on the reply",
						format, budget, total, total*100/budget,
						estimateSnapshotTokens(len(framing)), estimateSnapshotTokens(len(body)))
				}
				t.Logf("%-7s budget=%3d -> reply ~%3d tokens (%3d%%), framing ~%2d, %d/%d nodes",
					format, budget, total, total*100/budget, estimateSnapshotTokens(len(framing)), len(kept), len(nodes))
			})
		}
	}
}

// The framing is priced by building it, so a change to what the handler writes
// changes the charge with it. This pins that the reserve is that string's length
// and not a second description of it - the failure mode nodeCost already avoids
// by rendering through appendNode.
func TestFramingReserveIsTheFramingItself(t *testing.T) {
	h := &Handlers{
		Config:    &config.RuntimeConfig{},
		IDPIGuard: idpi.NewGuard(config.IDPIConfig{}, nil),
	}
	const title, url = "Title", "https://example.com/p"

	for _, format := range []string{"compact", "text"} {
		header := snapshotCompactHeader(title, url, 7, nil)
		if format == "text" {
			header = snapshotTextHeader(title, url, 7, nil)
		}
		want := len(snapshotFraming(format, header, "", nil, true, 500))
		got := h.snapshotFramingReserve(format, title, url, 7, nil, "", nil, 500)
		if got != want {
			t.Errorf("%s: reserved %d bytes but the framing it writes is %d", format, got, want)
		}
	}
}

// The untrusted-content wrapper is part of the reply and is decided by config, not
// by the scan, so it can be and is charged. Leaving it out would put the ceiling
// back over by an advisory paragraph - the larger of the two framings.
func TestFramingReserveChargesTheUntrustedContentWrapper(t *testing.T) {
	const title, url = "Title", "https://example.com/p"
	plain := &Handlers{
		Config:    &config.RuntimeConfig{},
		IDPIGuard: idpi.NewGuard(config.IDPIConfig{}, nil),
	}
	wrapping := &Handlers{
		Config:    &config.RuntimeConfig{IDPI: config.IDPIConfig{Enabled: true, WrapContent: true}},
		IDPIGuard: idpi.NewGuard(config.IDPIConfig{Enabled: true, WrapContent: true}, nil),
	}

	bare := plain.snapshotFramingReserve("compact", title, url, 7, nil, "", nil, 500)
	wrapped := wrapping.snapshotFramingReserve("compact", title, url, 7, nil, "", nil, 500)
	if wrapped <= bare {
		t.Fatalf("wrapping config reserved %d bytes, no more than the %d reserved without it", wrapped, bare)
	}
	if overhead := wrapped - bare; overhead < 100 {
		t.Errorf("wrapper charged only %d bytes; the advisory alone is larger than that", overhead)
	}
}

// json and yaml carry no plain-text header or advisory, and a file export writes its
// own layout, so none of them give up node budget to framing they never send. json
// is the default format: charging it the compact header and the advisory left a
// 100-token budget with no nodes at all on a long title and URL.
func TestNodeBudgetIsNotChargedFramingTheReplyDoesNotCarry(t *testing.T) {
	h := &Handlers{
		Config:    &config.RuntimeConfig{IDPI: config.IDPIConfig{Enabled: true, WrapContent: true}},
		IDPIGuard: idpi.NewGuard(config.IDPIConfig{Enabled: true, WrapContent: true}, nil),
	}
	const (
		title = "Enterprise Platform Pricing, Plans and Frequently Asked Questions"
		url   = "https://example.com/a-fairly-long-marketing-path/with-nested-segments/"
	)

	for _, tc := range []struct{ format, output string }{
		{"json", ""}, {"yaml", ""}, {"compact", "file"}, {"text", "file"}, {"json", "file"},
	} {
		if got := h.snapshotNodeBudget(tc.format, tc.output, title, url, 80, nil, "", nil, 100); got != 100 {
			t.Errorf("format=%s output=%q: node budget %d, want the full 100", tc.format, tc.output, got)
		}
	}
	if got := h.snapshotNodeBudget("compact", "", title, url, 80, nil, "", nil, 100); got >= 100 {
		t.Errorf("compact: node budget %d, want the framing reserved out of 100", got)
	}
}
