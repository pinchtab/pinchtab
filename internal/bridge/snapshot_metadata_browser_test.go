package bridge

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/config"
)

func newMetadataBridge(t *testing.T) (*Bridge, context.Context) {
	t.Helper()
	ctx := newActionTab(t, 30*time.Second)

	html := `<form><label for="email">Email</label>
		<input id="email" type="text" placeholder="you@example.com" data-testid="signup-email">
		<button type="submit">Continue</button></form>`
	if err := chromedp.Run(ctx,
		chromedp.Navigate("data:text/html;base64,"+base64.StdEncoding.EncodeToString([]byte(html))),
		chromedp.WaitVisible("#email", chromedp.ByID),
	); err != nil {
		t.Fatal(err)
	}

	b := New(context.Background(), ctx, &config.RuntimeConfig{})
	b.RegisterTab("tab-metadata", ctx)
	return b, ctx
}

func TestSnapshotSkipsTheDOMPassOnlyWhenAskedTo(t *testing.T) {
	b, ctx := newMetadataBridge(t)

	for _, tc := range []struct {
		name         string
		skip         bool
		wantEnriched bool
	}{
		{name: "metadata wanted", skip: false, wantEnriched: true},
		{name: "metadata skipped", skip: true, wantEnriched: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := b.Snapshot(ctx, "tab-metadata", "", ContentParams{SkipMetadata: tc.skip})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Nodes) == 0 {
				t.Fatal("the snapshot is empty, so neither answer here means anything")
			}
			enriched := false
			for _, node := range result.Nodes {
				if node.Tag != "" || node.TestID != "" || node.Placeholder != "" {
					enriched = true
					break
				}
			}
			if enriched != tc.wantEnriched {
				t.Errorf("SkipMetadata=%v produced enriched=%v, want %v; the flag is what keeps a per-node DOM round trip off the format that cannot render its result",
					tc.skip, enriched, tc.wantEnriched)
			}
		})
	}
}
