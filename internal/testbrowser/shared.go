package testbrowser

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// TabLoadTimeout replaces the production tab-create load budget in test Bridges:
// with several go test ./... runs sharing the machine a first load can exceed 20s.
const TabLoadTimeout = 60 * time.Second

// launchTimeout replaces chromedp's 20s DevTools-URL wait, which runs out under
// parallel test runs.
const launchTimeout = 60 * time.Second

// shared is launched lazily so a package whose browser tests all skip never starts it.
var shared struct {
	once          sync.Once
	ctx           context.Context
	cancelBrowser context.CancelFunc
	cancelAlloc   context.CancelFunc
	profile       string
	err           error
}

// Main stops the shared browser after the package's tests; without it the browser
// outlives the test binary on macOS:
//
//	func TestMain(m *testing.M) { os.Exit(testbrowser.Main(m)) }
func Main(m *testing.M) int {
	code := m.Run()
	stopShared()
	return code
}

// Tab opens a tab in the package's shared browser and closes it, with any tab
// opened from it, when the test ends. Tabs share the default browser context
// (Chrome refuses new tabs in a fresh one when headless), so cookies and storage
// carry over between tests.
func Tab(t testing.TB) context.Context {
	t.Helper()
	browserCtx := sharedBrowser(t)
	tabCtx, cancel := chromedp.NewContext(browserCtx)
	t.Cleanup(cancel)
	if err := chromedp.Run(tabCtx); err != nil {
		t.Fatalf("open tab in shared test browser: %v", err)
	}
	return tabCtx
}

// Launch starts a browser of the test's own, for a test that needs a browser-wide
// launch flag; prefer Tab, since every Launch pays a full browser start.
func Launch(t testing.TB, opts ...chromedp.ExecAllocatorOption) context.Context {
	t.Helper()
	alloc, cancelAlloc := chromedp.NewExecAllocator(context.Background(), allocatorOptions(Path(t), ProfileDir(t), opts...)...)
	ctx, cancelBrowser := chromedp.NewContext(alloc)
	t.Cleanup(func() {
		cancelBrowser()
		cancelAlloc()
	})
	if err := chromedp.Run(ctx); err != nil {
		t.Fatalf("start test browser: %v", err)
	}
	return ctx
}

func sharedBrowser(t testing.TB) context.Context {
	t.Helper()
	binary := Path(t)
	shared.once.Do(func() { shared.err = startShared(binary) })
	if shared.err != nil {
		t.Fatalf("start shared test browser: %v", shared.err)
	}
	return shared.ctx
}

func startShared(binary string) error {
	profile, err := os.MkdirTemp("", "pinchtab-test-profile-")
	if err != nil {
		return fmt.Errorf("create browser profile dir: %w", err)
	}
	shared.profile = profile
	alloc, cancelAlloc := chromedp.NewExecAllocator(context.Background(), allocatorOptions(binary, profile)...)
	ctx, cancelBrowser := chromedp.NewContext(alloc)
	shared.ctx, shared.cancelBrowser, shared.cancelAlloc = ctx, cancelBrowser, cancelAlloc
	return chromedp.Run(ctx)
}

func stopShared() {
	if shared.cancelAlloc == nil {
		return
	}
	shared.cancelBrowser()
	shared.cancelAlloc()
	removeProfileDir(stderrLogger{}, shared.profile)
}

type stderrLogger struct{}

func (stderrLogger) Logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}

func allocatorOptions(binary, profile string, extra ...chromedp.ExecAllocatorOption) []chromedp.ExecAllocatorOption {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(binary),
		chromedp.UserDataDir(profile),
		chromedp.Flag("headless", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.WSURLReadTimeout(launchTimeout),
	)
	return append(opts, extra...)
}
