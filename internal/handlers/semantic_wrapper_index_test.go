package handlers

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/selector"
	"github.com/pinchtab/pinchtab/internal/srccensus"
	"github.com/pinchtab/pinchtab/internal/testbrowser"
	"github.com/pinchtab/semantic"
)

func newWrapperIndexHandler(t *testing.T) *Handlers {
	t.Helper()

	cache := &bridge.RefCache{
		Nodes: []bridge.A11yNode{
			{Ref: "e0", Role: "button", Name: "Save"},
			{Ref: "e1", Role: "button", Name: "Save"},
			{Ref: "e2", Role: "button", Name: "Save"},
		},
		Refs: map[string]int64{"e0": 1, "e1": 2, "e2": 3},
	}
	h := New(&findMockBridge{refCache: cache}, &config.RuntimeConfig{ActionTimeout: 10 * time.Second}, nil, nil, nil)
	h.Matcher = semantic.NewCombinedMatcher(semantic.NewHashingEmbedder(128))
	return h
}

func resolveWrapperSelector(t *testing.T, h *Handlers, raw string) (string, error) {
	t.Helper()

	req := bridge.ActionRequest{}
	handled, err := h.applySemanticActionSelector(context.Background(), "tab1", selector.Parse(raw), &req)
	if !handled {
		t.Fatalf("%s did not route to the semantic matcher, so this test is not exercising the wrapper path", raw)
	}
	return req.Ref, err
}

// TestAPositionalWrapperOverASemanticFormIsZeroBased is the defect: the matcher
// publishes nth as one-based, this project publishes it as zero-based, and without the
// translation at the boundary the documented nth:0 matched nothing at all while the
// elements were plainly on the page.
func TestAPositionalWrapperOverASemanticFormIsZeroBased(t *testing.T) {
	h := newWrapperIndexHandler(t)

	for _, tc := range []struct{ raw, wantRef string }{
		{"role:button Save", "e0"},
		{"first:role:button Save", "e0"},
		{"nth:0:role:button Save", "e0"},
		{"nth:1:role:button Save", "e1"},
		{"nth:2:role:button Save", "e2"},
		{"last:role:button Save", "e2"},
	} {
		ref, err := resolveWrapperSelector(t, h, tc.raw)
		if err != nil {
			t.Errorf("%s: %v", tc.raw, err)
			continue
		}
		if ref != tc.wantRef {
			t.Errorf("%s resolved %s, want %s", tc.raw, ref, tc.wantRef)
		}
	}
}

// TestAnOutOfRangeWrapperIndexSaysSoInsteadOfNoMatch: an agent cannot act on "no
// matching element found" when the elements exist and only the index is wrong. The
// message must name the index the caller wrote — zero-based, this project's spelling —
// and must not leak the translated one-based index the matcher received.
func TestAnOutOfRangeWrapperIndexSaysSoInsteadOfNoMatch(t *testing.T) {
	h := newWrapperIndexHandler(t)

	_, err := resolveWrapperSelector(t, h, "nth:7:role:button Save")
	if err == nil {
		t.Fatal("nth:7 over three matches must refuse")
	}
	msg := err.Error()
	for _, needle := range []string{"index 7", "out of range", "3 element(s)"} {
		if !strings.Contains(msg, needle) {
			t.Errorf("refusal %q does not carry %q", msg, needle)
		}
	}
	if strings.Contains(msg, "no matching element found") {
		t.Errorf("refusal %q reports an empty page for what is only a bad index", msg)
	}
	if strings.Contains(msg, "nth:8") {
		t.Errorf("refusal %q names the translated one-based index; it must name what the caller sent", msg)
	}
	// The status itself is asserted at the HTTP boundary in semantic_status_test.go —
	// a mapper can be correct while nobody calls it, which is exactly what happened
	// here. What this layer owns is that the refusal CARRIES the sentinel every caller
	// maps from.
	if !errors.Is(err, ErrElementNotFound) {
		t.Errorf("out-of-range refusal does not wrap ErrElementNotFound, so every caller has to recognise it by wording")
	}
}

func TestAWrapperOverASemanticFormWithNoMatchesStillReportsNoMatch(t *testing.T) {
	h := newWrapperIndexHandler(t)

	_, err := resolveWrapperSelector(t, h, "nth:0:role:slider Volume")
	if err == nil {
		t.Fatal("a wrapper over a selector nothing matches must refuse")
	}
	if !strings.Contains(err.Error(), "no matching element found") {
		t.Errorf("refusal %q should report an empty match set, not an index problem", err.Error())
	}
}

// TestTheSameNthSelectsTheSameOrdinalOnBothPaths is the agreement the card asks for:
// one page, one index, both resolution paths. A one-sided test cannot catch the base
// drifting again, and the base is what drifted — the semantic path goes through the
// matcher's one-based nth while css goes through the browser-side zero-based resolver.
func TestTheSameNthSelectsTheSameOrdinalOnBothPaths(t *testing.T) {
	ctx, cancelTimeout := context.WithTimeout(testbrowser.Tab(t), 60*time.Second)
	t.Cleanup(cancelTimeout)

	html := `<button id="first">Save</button><button id="second">Save</button><button id="third">Save</button>`
	if err := chromedp.Run(ctx,
		chromedp.Navigate("data:text/html;base64,"+base64.StdEncoding.EncodeToString([]byte(html))),
		chromedp.WaitVisible("#third", chromedp.ByID),
	); err != nil {
		t.Fatal(err)
	}

	cfg := &config.RuntimeConfig{ActionTimeout: 10 * time.Second, DefaultBrowser: config.BrowserChrome, StateDir: t.TempDir()}
	b := browserBridge(ctx, cfg)
	b.RegisterTab("tab-nth", ctx)
	h := New(b, cfg, nil, nil, nil)
	h.Matcher = semantic.NewCombinedMatcher(semantic.NewHashingEmbedder(128))

	elementID := func(t *testing.T, raw string) string {
		t.Helper()
		req := bridge.ActionRequest{Selector: raw}
		if _, err := h.resolveActionRequestSelector(ctx, "tab-nth", &req); err != nil {
			t.Fatalf("resolve %s: %v", raw, err)
		}
		if req.NodeID == 0 {
			t.Fatalf("resolve %s produced no node", raw)
		}
		var id string
		if err := b.CallFunctionOnNode(ctx, req.NodeID, `function() { return this.id; }`, nil, &id); err != nil {
			t.Fatalf("describe %s: %v", raw, err)
		}
		return id
	}

	for index, wantID := range []string{"first", "second", "third"} {
		css := fmt.Sprintf("nth:%d:css:button", index)
		role := fmt.Sprintf("nth:%d:role:button Save", index)

		gotCSS := elementID(t, css)
		gotRole := elementID(t, role)
		if gotCSS != wantID {
			t.Errorf("%s selected #%s, want #%s", css, gotCSS, wantID)
		}
		if gotRole != gotCSS {
			t.Errorf("%s selected #%s but %s selected #%s; the same index must mean the same ordinal on both paths", role, gotRole, css, gotCSS)
		}
	}
}

// TestOnlyTheSelectorPackageTouchesTheWrapperIndex keeps the base translation in one
// owner. Both SemanticQuery consumers in this package — the action path and count.go —
// get the converted index for free; a second adjustment here is how the two bases
// drifted apart in the first place.
func TestOnlyTheSelectorPackageTouchesTheWrapperIndex(t *testing.T) {
	consumers := 0
	for name, body := range srccensus.Load(t, ".", 80).Sources() {
		if strings.Contains(body, `"nth:`) {
			t.Errorf("%s builds an nth: query itself; the index base is translated once, in internal/selector", name)
		}
		if strings.Contains(body, ".SemanticQuery()") {
			consumers++
		}
	}
	if consumers < 2 {
		t.Errorf("found %d SemanticQuery consumers, want at least 2 (the action path and count.go); a consumer that stopped routing through it would keep the old base", consumers)
	}
}
