package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

func TestCreateTabRequestsRenderedTargetWithoutWindowFocus(t *testing.T) {
	created := make(chan json.RawMessage, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _, _, err := ws.UpgradeHTTP(r, w)
		if err != nil {
			return
		}
		defer conn.Close()

		for {
			payload, op, err := wsutil.ReadClientData(conn)
			if err != nil {
				return
			}
			if op != ws.OpText {
				continue
			}

			var command struct {
				ID        int64           `json:"id"`
				Method    string          `json:"method"`
				Params    json.RawMessage `json:"params"`
				SessionID string          `json:"sessionId,omitempty"`
			}
			if json.Unmarshal(payload, &command) != nil {
				return
			}

			result := `{}`
			switch command.Method {
			case "Target.attachToTarget":
				result = `{"sessionId":"session-existing"}`
			case "Runtime.evaluate":
				result = `{"result":{"type":"object","className":"Window"}}`
			case "Page.getFrameTree":
				result = `{"frameTree":{"frame":{"id":"frame-existing","url":"about:blank"}}}`
			case "DOM.getDocument":
				result = `{"root":{"nodeId":1,"backendNodeId":1,"nodeType":9,"nodeName":"#document","localName":"","nodeValue":""}}`
			case "Target.createTarget":
				created <- append(json.RawMessage(nil), command.Params...)
				result = `{"targetId":"created-tab"}`
			}

			response := fmt.Sprintf(`{"id":%d,"result":%s`, command.ID, result)
			if command.SessionID != "" {
				response += fmt.Sprintf(`,"sessionId":%q`, command.SessionID)
			}
			if wsutil.WriteServerMessage(conn, ws.OpText, []byte(response+`}`)) != nil {
				return
			}
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/devtools/browser/test"
	allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, wsURL, chromedp.NoModifyURL)
	defer allocCancel()
	browserCtx, browserCancel := chromedp.NewContext(allocCtx, chromedp.WithTargetID(target.ID("existing-tab")))
	defer browserCancel()
	if err := chromedp.Run(browserCtx); err != nil {
		t.Fatalf("initialize browser context: %v", err)
	}

	setupCalled := false
	tm := NewTabManager(browserCtx, nil, nil, nil, func(ctx context.Context, _ string) error {
		if cdp.ExecutorFromContext(ctx) == nil {
			return fmt.Errorf("tab setup received no target executor")
		}
		setupCalled = true
		return nil
	})
	_, _, tabCancel, err := tm.CreateTabInBrowserContext("about:blank", "context-profile")
	if err != nil {
		t.Fatalf("CreateTab: %v", err)
	}
	if !setupCalled {
		t.Fatal("CreateTab did not run target setup")
	}
	defer tabCancel()

	var params struct {
		Background       *bool  `json:"background"`
		NewWindow        *bool  `json:"newWindow"`
		Focus            *bool  `json:"focus"`
		BrowserContextID string `json:"browserContextId"`
	}
	select {
	case raw := <-created:
		if err := json.Unmarshal(raw, &params); err != nil {
			t.Fatalf("decode Target.createTarget params: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("Target.createTarget request not observed: %v", ctx.Err())
	}

	if params.Background != nil && *params.Background {
		t.Fatal("Target.createTarget background = true, want absent or false for a rendered tab")
	}
	if params.NewWindow != nil && *params.NewWindow {
		t.Fatal("Target.createTarget newWindow = true, want absent or false")
	}
	if params.Focus != nil && *params.Focus {
		t.Fatal("Target.createTarget focus = true, want absent or false")
	}
	if params.BrowserContextID != "context-profile" {
		t.Fatalf("Target.createTarget browserContextId = %q, want context-profile", params.BrowserContextID)
	}

	adoptSetupCalled := false
	adoptTM := NewTabManager(browserCtx, nil, nil, nil, func(ctx context.Context, _ string) error {
		if cdp.ExecutorFromContext(ctx) == nil {
			return fmt.Errorf("adopted tab setup received no target executor")
		}
		adoptSetupCalled = true
		return nil
	})
	if _, err := adoptTM.adoptExistingTarget(target.ID("created-tab"), false); err != nil {
		t.Fatalf("adoptExistingTarget: %v", err)
	}
	if !adoptSetupCalled {
		t.Fatal("adoptExistingTarget did not run target setup")
	}
}
