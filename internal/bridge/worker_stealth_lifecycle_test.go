package bridge

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/testbrowser"
)

// Worker stealth parity attaches to every worker target to patch its navigator.
// That attach must leave the worker running: a service worker whose install
// step waits on a timer was terminated mid-install and went "redundant", so
// PWAs could not install under PinchTab while plain Chrome installed them.
// The worker must still come out patched. WorkerNavigator has no webdriver
// property, so it reads undefined until the parity script defines it as false;
// the patch lands asynchronously after attach, so the page asks until it shows.
const serviceWorkerFixtureHTML = `<!doctype html><html><body><script>
window.swState = 'pending';
navigator.serviceWorker.register('/sw.js', {scope: '/'}).then(reg => {
  const w = reg.installing || reg.waiting || reg.active;
  const done = () => {
    if (w.state === 'redundant') { window.swState = 'redundant'; return; }
    const ask = tries => {
      const ch = new MessageChannel();
      ch.port1.onmessage = e => {
        if (e.data === 'false' || tries === 0) window.swState = w.state + ' webdriver=' + e.data;
        else setTimeout(() => ask(tries - 1), 100);
      };
      w.postMessage('webdriver?', [ch.port2]);
    };
    ask(50);
  };
  if (w.state !== 'installing') return done();
  w.addEventListener('statechange', () => { if (w.state !== 'installing') done(); });
}, err => { window.swState = 'register failed: ' + err; });
</script></body></html>`

func TestWorkerStealthParityKeepsServiceWorkerAlive(t *testing.T) {
	chromePath := testbrowser.Path(t)

	for _, tc := range []struct {
		name    string
		install string
	}{
		{name: "timer-backed install", install: `e.waitUntil(new Promise(r => setTimeout(r, 300)))`},
		{name: "immediate install", install: `e.waitUntil(Promise.resolve())`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				_, _ = fmt.Fprint(w, serviceWorkerFixtureHTML)
			})
			mux.HandleFunc("/sw.js", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/javascript")
				_, _ = fmt.Fprintf(w, "self.addEventListener('install', e => %s);\n"+
					"self.addEventListener('message', e => e.ports[0].postMessage(String(self.navigator.webdriver)));", tc.install)
			})
			srv := httptest.NewServer(mux)
			defer srv.Close()

			alloc, cancelAlloc := chromedp.NewExecAllocator(context.Background(), append(
				chromedp.DefaultExecAllocatorOptions[:],
				chromedp.ExecPath(chromePath),
				chromedp.Flag("headless", true),
				chromedp.Flag("no-sandbox", true),
			)...)
			defer cancelAlloc()
			tabCtx, cancelTab := chromedp.NewContext(alloc)
			defer cancelTab()
			if err := chromedp.Run(tabCtx); err != nil {
				t.Fatalf("start browser: %v", err)
			}

			b := &Bridge{Config: &config.RuntimeConfig{Headless: true}}
			b.installWorkerStealthParity(tabCtx)

			ctx, cancel := context.WithTimeout(tabCtx, 15*time.Second)
			defer cancel()
			if err := chromedp.Run(ctx, chromedp.Navigate(srv.URL)); err != nil {
				t.Fatalf("navigate: %v", err)
			}
			var state string
			if err := chromedp.Run(ctx, chromedp.Poll(`window.swState !== 'pending' && window.swState`, &state)); err != nil {
				t.Fatalf("wait for service worker: %v", err)
			}
			if !strings.HasSuffix(state, " webdriver=false") || strings.HasPrefix(state, "redundant") {
				t.Fatalf("service worker ended %q after worker stealth attached, want installed with webdriver=false", state)
			}
		})
	}
}
