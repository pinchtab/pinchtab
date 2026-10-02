// An external test package on purpose: this compares the static-fetch name against
// the CDP one, and internal/testbrowser reaches staticfetch through
// internal/browsers/ghostchrome, so an in-package test importing it would cycle.
package staticfetch_test

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/bridge/observe"
	"github.com/pinchtab/pinchtab/internal/browsers/ghostchrome/staticfetch"
	"github.com/pinchtab/pinchtab/internal/testbrowser"
)

// This is the criterion the product decision exists for: the accessible name is a
// matching key, so the SAME element on the SAME page must give an agent the same
// string whichever provider produced the snapshot. A test comparing ghost-chrome
// against itself cannot see the divergence being closed — the CDP side has to be
// the oracle.
func TestAccessibleNameMatchesTheCDPPathForTheSameElement(t *testing.T) {
	const marker = "PARITY"
	label := marker + strings.Repeat("x", 400) + "-end"
	body := `<!doctype html><html><head><title>parity</title></head><body>` +
		`<button id="target">` + label + `</button></body></html>`

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(body))
	}))
	defer ts.Close()

	lite := staticfetch.NewBrowser()
	defer func() { _ = lite.Close() }()
	if _, err := lite.Navigate(context.Background(), ts.URL); err != nil {
		t.Fatalf("Navigate: %v", err)
	}
	result, err := lite.Snapshot(context.Background(), "", "all")
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	staticName := ""
	for _, node := range result.Nodes {
		if strings.HasPrefix(node.Name, marker) {
			staticName = node.Name
			break
		}
	}
	if staticName == "" {
		t.Fatal("the static snapshot carries no node named from the fixture button")
	}

	cdpName := cdpAccessibleName(t, body, marker)

	if staticName != cdpName {
		t.Errorf("provider divergence on a matching key:\n static-fetch (%d bytes) = %q\n CDP          (%d bytes) = %q\nAn agent matching on `name` must get one string per element regardless of provider",
			len(staticName), staticName, len(cdpName), cdpName)
	}
}

func cdpAccessibleName(t *testing.T, body, marker string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(testbrowser.Tab(t), 30*time.Second)
	t.Cleanup(cancel)

	dataURL := "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(body))
	if err := chromedp.Run(ctx,
		chromedp.Navigate(dataURL),
		chromedp.WaitVisible("#target", chromedp.ByID),
	); err != nil {
		t.Fatal(err)
	}

	raw, err := observe.FetchAXTree(ctx)
	if err != nil {
		t.Fatalf("FetchAXTree: %v", err)
	}
	flat, _ := observe.BuildSnapshot(raw, "all", -1)
	for _, node := range flat {
		if strings.HasPrefix(node.Name, marker) {
			return node.Name
		}
	}
	t.Fatal("the CDP snapshot carries no node named from the fixture button; the oracle read nothing")
	return ""
}
