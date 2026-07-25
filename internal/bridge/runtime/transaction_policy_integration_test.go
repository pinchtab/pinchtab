//go:build integration

package runtime

import (
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/config"
)

// Run manually with an unpacked-extension-capable Chrome for Testing or Chromium:
//
//	PINCHTAB_TEST_TRANSACTION_POLICY_CHROME=/path/to/chrome \
//	  go test -tags=integration ./internal/bridge/runtime -run TestTransactionPolicyDNRIntegration -count=1
//
// This is environment-gated and is not asserted to run in CI. The fixture uses
// server-side counters only; a blocked request has no JavaScript "attempt" flag
// that could turn a browser failure into a passing policy proof.
func TestTransactionPolicyDNRIntegration(t *testing.T) {
	binary := os.Getenv("PINCHTAB_TEST_TRANSACTION_POLICY_CHROME")
	if binary == "" {
		t.Skip("set PINCHTAB_TEST_TRANSACTION_POLICY_CHROME to a Chrome for Testing or Chromium binary")
	}
	var mu sync.Mutex
	counts := map[string]int{}
	methodCounts := map[string]int{}
	count := func(path string) int { mu.Lock(); defer mu.Unlock(); return counts[path] }
	methodCount := func(method, path string) int { mu.Lock(); defer mu.Unlock(); return methodCounts[method+" "+path] }
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		counts[r.URL.Path]++
		methodCounts[r.Method+" "+r.URL.Path]++
		mu.Unlock()
		switch r.URL.Path {
		case "/worker.js":
			w.Header().Set("Content-Type", "application/javascript")
			_, _ = w.Write([]byte(`fetch('/worker-forbidden',{method:'POST'}).catch(()=>{}).then(()=>fetch('/worker-ran'));`))
		case "/shared-worker.js":
			w.Header().Set("Content-Type", "application/javascript")
			_, _ = w.Write([]byte(`fetch('/shared-worker-forbidden',{method:'POST'}).catch(()=>{}).then(()=>fetch('/shared-worker-ran'));`))
		case "/service-worker.js":
			w.Header().Set("Content-Type", "application/javascript")
			_, _ = w.Write([]byte(`self.addEventListener('install', e => e.waitUntil(fetch('/service-worker-forbidden',{method:'POST'}).catch(()=>{}).then(()=>fetch('/service-worker-ran'))));`))
		case "/redirect":
			http.Redirect(w, r, "/redirect-forbidden", http.StatusTemporaryRedirect)
		case "/ws-allowed":
			// A real 101 response makes the counter evidence an actual WebSocket
			// handshake rather than merely a JavaScript construction attempt.
			h, ok := w.(http.Hijacker)
			if !ok {
				http.Error(w, "hijacking unavailable", http.StatusInternalServerError)
				return
			}
			conn, rw, err := h.Hijack()
			if err != nil {
				return
			}
			defer conn.Close()
			sum := sha1.Sum([]byte(r.Header.Get("Sec-WebSocket-Key") + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
			_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: " + base64.StdEncoding.EncodeToString(sum[:]) + "\r\n\r\n")
			_ = rw.Flush()
		default:
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<!doctype html><iframe name=normal hidden></iframe>
<form action=/form-forbidden method=POST target=normal><input name=x value=1></form>
<form action=/popup-initial-forbidden method=POST target=policyPopup><input name=x value=1></form>
<script>
fetch('/page-forbidden',{method:'POST'}).catch(()=>{}).then(()=>fetch('/page-ran'));
document.forms[0].submit(); fetch('/form-ran'); document.forms[1].submit();
new Worker('/worker.js'); new SharedWorker('/shared-worker.js'); navigator.serviceWorker && navigator.serviceWorker.register('/service-worker.js');
fetch('/redirect',{method:'POST'}).catch(()=>{});
new WebSocket(location.origin.replace(/^http/,'ws')+'/ws-forbidden'); new WebSocket(location.origin.replace(/^http/,'ws')+'/ws-allowed');
fetch('/cart/add',{method:'POST'}); fetch('/cart/update',{method:'POST'}); fetch('/cart/remove',{method:'POST'});
fetch('/orders'); fetch('/orders',{method:'POST'}); fetch('/orders/%6F%72der',{method:'POST'});
fetch('/cart/preparation',{method:'POST'}); fetch('/cart/123/order',{method:'POST'}); fetch('/cart/123/%6Frder',{method:'POST'}); fetch('/cart/123/%6F%72der',{method:'POST'}); fetch('/cart/123/%2Forder',{method:'POST'}); fetch('/cart/preorder',{method:'POST'});
fetch('/checkout',{method:'POST'}); fetch('/chec%6bout',{method:'POST'});
const trailing = location.origin.replace('//127.0.0.1:', '//127.0.0.1.:'); fetch(trailing+'/trailing-read'); fetch(trailing+'/trailing-forbidden',{method:'POST'}); fetch(trailing+'/ord%65r',{method:'POST'}); fetch(trailing+'/%6F%72der',{method:'POST'});
fetch('/?wc-ajax=add_to_cart',{method:'POST'}); fetch('/?wc-ajax=add_to_cart&action=checkout',{method:'POST'});
fetch('/?action=checkout'); fetch('/?%61ction=checkout'); fetch('/?action=%63heckout'); fetch('/?%61%63tion=checkout',{method:'POST'}); fetch('/?action=cart');
fetch('/read'); fetch('/read',{method:'HEAD'});
</script>`))
		}
	}))
	defer server.Close()
	cfg := &config.RuntimeConfig{StateDir: t.TempDir(), ProfileDir: t.TempDir(), ChromeBinary: binary, Headless: true, TransactionPolicy: config.TransactionPolicyConfig{Enabled: true, Hosts: []string{"127.0.0.1"}, DenyRules: []config.TransactionPolicyRule{{Method: "*", PathPrefix: "/page-forbidden"}, {Method: "*", PathPrefix: "/form-forbidden"}, {Method: "*", PathPrefix: "/popup-initial-forbidden"}, {Method: "*", PathPrefix: "/worker-forbidden"}, {Method: "*", PathPrefix: "/shared-worker-forbidden"}, {Method: "*", PathPrefix: "/service-worker-forbidden"}, {Method: "*", PathPrefix: "/redirect-forbidden"}, {Method: "*", PathPrefix: "/ws-forbidden"}, {Method: "*", PathPrefix: "/checkout"}, {Method: "*", PathPrefix: "/trailing-forbidden"}, {Method: "POST", PathPrefix: "/orders"}, {Method: "POST", PathPrefix: "/", PathSegment: "order"}, {Method: "*", PathPrefix: "/", QueryParam: "action", QueryValue: "checkout"}}, AllowRules: []config.TransactionPolicyRule{{Method: "*", PathPrefix: "/cart"}, {Method: "POST", PathPrefix: "/redirect"}, {Method: "*", PathPrefix: "/ws-allowed"}, {Method: "POST", PathPrefix: "/checkout"}, {Method: "POST", PathPrefix: "/", QueryParam: "wc-ajax", QueryValue: "add_to_cart"}}}}
	launch, err := PrepareTransactionPolicyExtension(cfg)
	if err != nil {
		t.Fatal(err)
	}
	_, allocCancel, browserCtx, browserCancel, _, err := InitChrome(launch, nil, Hooks{})
	if err != nil {
		t.Fatal(err)
	}
	// Register Chrome teardown via t.Cleanup so it runs in LIFO order relative
	// to t.TempDir() cleanup: because ProfileDir/StateDir TempDirs were
	// registered first, this cleanup runs before them, ensuring the browser
	// process (and its renderer/GPU children) have fully released the profile
	// directory before the Go test runner tries to RemoveAll it.
	t.Cleanup(func() {
		browserCancel()
		allocCancel()
		// Chrome's SingletonLock is the last file-system artifact removed by the
		// browser process on exit.  Poll until it is gone, then allow a short
		// grace period for any in-flight renderer-process writes to drain.
		const (
			shutdownTimeout     = 10 * time.Second
			pollInterval        = 50 * time.Millisecond
			rendererGracePeriod = 200 * time.Millisecond
		)
		lock := filepath.Join(cfg.ProfileDir, "SingletonLock")
		deadline := time.Now().Add(shutdownTimeout)
		for time.Now().Before(deadline) {
			if _, err := os.Lstat(lock); os.IsNotExist(err) {
				time.Sleep(rendererGracePeriod)
				return
			}
			time.Sleep(pollInterval)
		}
		t.Logf("chrome profile SingletonLock still present after %v; TempDir cleanup may log a spurious RemoveAll warning", shutdownTimeout)
	})
	if err := chromedp.Run(browserCtx, chromedp.Navigate(server.URL)); err != nil {
		t.Fatal(err)
	}
	need := []string{"/", "/trailing-read", "/orders", "/cart/add", "/cart/update", "/cart/remove", "/cart/preparation", "/cart/preorder", "/redirect", "/worker.js", "/worker-ran", "/shared-worker.js", "/shared-worker-ran", "/service-worker.js", "/service-worker-ran", "/page-ran", "/form-ran", "/ws-allowed"}
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		ready := true
		for _, path := range need {
			if count(path) == 0 {
				ready = false
				break
			}
		}
		if ready && count("/") >= 3 && count("/read") >= 2 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	for _, path := range []string{"/page-forbidden", "/form-forbidden", "/popup-initial-forbidden", "/worker-forbidden", "/shared-worker-forbidden", "/service-worker-forbidden", "/redirect-forbidden", "/ws-forbidden", "/checkout", "/trailing-forbidden", "/cart/123/order"} {
		if got := count(path); got != 0 {
			t.Errorf("forbidden request %s reached fixture %d times", path, got)
		}
	}
	for _, path := range need {
		if got := count(path); got == 0 {
			t.Errorf("allowed request %s did not reach fixture", path)
		}
	}
	if got := count("/"); got != 3 {
		t.Errorf("query deny bypass/broadening or missing exact query allow: root count = %d", got)
	}
	if got := count("/read"); got < 2 {
		t.Errorf("allowed GET/HEAD reads did not reach fixture twice: %d", got)
	}
	if got := methodCount(http.MethodGet, "/orders"); got != 1 {
		t.Errorf("GET /orders did not reach fixture exactly once: %d", got)
	}
	if got := methodCount(http.MethodPost, "/orders"); got != 0 {
		t.Errorf("POST /orders reached fixture despite deny: %d", got)
	}
	for _, path := range []string{"/cart/123/order", "/cart/123//order", "/order"} {
		if got := methodCount(http.MethodPost, path); got != 0 {
			t.Errorf("POST %s reached fixture despite raw/encoded order deny: %d", path, got)
		}
	}
	for _, path := range []string{"/cart/preparation", "/cart/preorder"} {
		if got := methodCount(http.MethodPost, path); got != 1 {
			t.Errorf("allowed POST %s did not reach fixture exactly once: %d", path, got)
		}
	}
	if t.Failed() {
		mu.Lock()
		snapshot := fmt.Sprint(counts)
		mu.Unlock()
		t.Logf("fixture counters: %s", snapshot)
	}
}
