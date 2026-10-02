package observe

import (
	"context"
	"encoding/base64"
	"os"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/testbrowser"
)

func TestMain(m *testing.M) { os.Exit(testbrowser.Main(m)) }

const fixtureDeadline = 30 * time.Second

func deadlineTab(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(testbrowser.Tab(t), fixtureDeadline)
	t.Cleanup(cancel)
	return ctx
}

func htmlDataURL(html string) string {
	return "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(html))
}

func fixtureTab(t *testing.T, html, readySelector string) context.Context {
	t.Helper()
	ctx := deadlineTab(t)
	if err := chromedp.Run(ctx,
		chromedp.Navigate(htmlDataURL(html)),
		chromedp.WaitVisible(readySelector, chromedp.ByID),
	); err != nil {
		t.Fatal(err)
	}
	return ctx
}
