package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/testbrowser"
)

// startUserOwnedBrowser launches a browser the way the reporter's i3 session
// does: externally, with a debugging port, owning its own tabs. PinchTab never
// launches or owns this process.
func startUserOwnedBrowser(t *testing.T) (wsURL string, devtoolsPort int) {
	t.Helper()
	chromePath := testbrowser.Path(t)
	profile := testbrowser.ProfileDir(t)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	devtoolsPort = listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(chromePath,
		fmt.Sprintf("--remote-debugging-port=%d", devtoolsPort),
		"--user-data-dir="+profile,
		"--headless=new",
		"--no-sandbox",
		"--no-first-run",
		"--no-default-browser-check",
		"about:blank",
	)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		_ = os.RemoveAll(profile)
	})

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/json/version", devtoolsPort))
		if err == nil {
			var info struct {
				WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
			}
			decodeErr := json.NewDecoder(resp.Body).Decode(&info)
			_ = resp.Body.Close()
			if decodeErr == nil && info.WebSocketDebuggerURL != "" {
				return info.WebSocketDebuggerURL, devtoolsPort
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("user-owned browser never exposed a devtools endpoint")
	return "", 0
}

// closeTabOutOfBand closes a tab the way the user does: in their own browser,
// with no involvement from PinchTab's CDP session.
func closeTabOutOfBand(t *testing.T, devtoolsPort int, targetID string) {
	t.Helper()
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/json/close/%s", devtoolsPort, targetID))
	if err != nil {
		t.Fatalf("close tab out of band: %v", err)
	}
	_ = resp.Body.Close()
}

// Attaching to a browser that is already running binds PinchTab's browser-level
// context to a tab that the user owns, because chromedp's first context adopts
// an existing target instead of creating one. Closing that particular tab must
// not take the instance down with it: the tab was never PinchTab's to depend on.
//
// Before the fix, CreateTab issued Target.createTarget over that dead tab's CDP
// session, so every later call blocked for the full tabCreateTimeout and the
// instance stayed wedged until it was stopped and re-attached (issue #690).
func TestCreateTabSurvivesUserClosingTheAdoptedBrowserContextTab(t *testing.T) {
	wsURL, devtoolsPort := startUserOwnedBrowser(t)

	allocCtx, allocCancel := chromedp.NewRemoteAllocator(context.Background(), wsURL)
	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	t.Cleanup(func() {
		browserCancel()
		allocCancel()
	})
	if err := chromedp.Run(browserCtx, chromedp.ActionFunc(func(context.Context) error { return nil })); err != nil {
		t.Fatalf("attach to user-owned browser: %v", err)
	}

	chromeCtx := chromedp.FromContext(browserCtx)
	if chromeCtx == nil || chromeCtx.Target == nil {
		t.Fatal("attached browser context has no target")
	}
	adoptedUserTab := chromeCtx.Target.TargetID.String()

	tm := NewTabManager(browserCtx, &config.RuntimeConfig{}, nil, nil)

	if _, _, _, err := tm.CreateTab("about:blank"); err != nil {
		t.Fatalf("baseline CreateTab against the attached browser failed: %v", err)
	}

	closeTabOutOfBand(t, devtoolsPort, adoptedUserTab)
	time.Sleep(1500 * time.Millisecond)

	done := make(chan error, 1)
	start := time.Now()
	go func() {
		_, _, _, err := tm.CreateTab("about:blank")
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("CreateTab failed after the user closed the adopted tab %s: %v", adoptedUserTab, err)
		}
		t.Logf("CreateTab recovered in %v", time.Since(start).Round(time.Millisecond))
	case <-time.After(tabCreateTimeout - time.Second):
		t.Fatalf("CreateTab wedged for %v after the user closed the adopted tab %s; "+
			"the browser-level call is still routed through that tab's dead CDP session",
			time.Since(start).Round(time.Millisecond), adoptedUserTab)
	}
}

// The reported failure was on the shorthand read routes: every one of them
// returned "resolve topmost dialog: resolve top frame: context deadline
// exceeded" until the instance was stopped and re-attached.
//
// Target.targetDestroyed arrives over the CDP session of the tab chromedp
// adopted for the browser-level context. Once the user closes that tab the
// event stops arriving for every other tab too, so a tab that dies afterwards
// is never dropped, and tab selection keeps handing out its dead context.
func TestTabSelectionDropsATabThatDiedAfterTheDestroyedEventStopped(t *testing.T) {
	wsURL, devtoolsPort := startUserOwnedBrowser(t)

	allocCtx, allocCancel := chromedp.NewRemoteAllocator(context.Background(), wsURL)
	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	t.Cleanup(func() {
		browserCancel()
		allocCancel()
	})
	if err := chromedp.Run(browserCtx, chromedp.ActionFunc(func(context.Context) error { return nil })); err != nil {
		t.Fatalf("attach to user-owned browser: %v", err)
	}

	chromeCtx := chromedp.FromContext(browserCtx)
	if chromeCtx == nil || chromeCtx.Target == nil {
		t.Fatal("attached browser context has no target")
	}
	adoptedUserTab := chromeCtx.Target.TargetID.String()

	tm := NewTabManager(browserCtx, &config.RuntimeConfig{}, nil, nil)
	workTabID, _, _, err := tm.CreateTab("about:blank")
	if err != nil {
		t.Fatalf("create the tab under test: %v", err)
	}
	tm.mu.RLock()
	workTabCDPID := tm.tabs[workTabID].CDPID
	tm.mu.RUnlock()

	baselineCtx, _, err := tm.TabContext("")
	if err != nil {
		t.Fatalf("baseline tab selection: %v", err)
	}
	if _, _, err := TopmostModalNodeID(baselineCtx, ""); err != nil {
		t.Fatalf("baseline dialog resolution: %v", err)
	}

	// The user closes the adopted tab, which silences targetDestroyed, and then
	// the tab PinchTab was working in. Nothing tells PinchTab either is gone.
	closeTabOutOfBand(t, devtoolsPort, adoptedUserTab)
	time.Sleep(500 * time.Millisecond)
	closeTabOutOfBand(t, devtoolsPort, workTabCDPID)
	time.Sleep(staleSweepInterval + time.Second)

	type result struct {
		err error
	}
	done := make(chan result, 1)
	start := time.Now()
	go func() {
		selected, _, selectErr := tm.TabContext("")
		if selectErr != nil {
			// A named error is a correct outcome: the caller can act on it.
			done <- result{err: nil}
			return
		}
		_, _, resolveErr := TopmostModalNodeID(selected, "")
		done <- result{err: resolveErr}
	}()

	select {
	case res := <-done:
		if res.err != nil {
			t.Fatalf("tab selection returned a context that could not be read: %v", res.err)
		}
		t.Logf("tab selection recovered in %v", time.Since(start).Round(time.Millisecond))
	case <-time.After(15 * time.Second):
		t.Fatalf("tab selection handed out the dead tab %s and the read wedged for %v",
			workTabCDPID, time.Since(start).Round(time.Millisecond))
	}
}
