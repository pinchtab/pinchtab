//go:build integration

package integration

import (
	"fmt"
	"net/url"
	"testing"
	"time"
)

func TestClipboardReadWrite(t *testing.T) {
	navigate(t, "https://example.com")
	if currentTabID == "" {
		t.Fatalf("missing current tab id after navigate")
	}

	tab := url.QueryEscape(currentTabID)

	text := fmt.Sprintf("pinchtab-clipboard-%d", time.Now().UnixNano())
	code, body := httpPost(t, "/clipboard/write?tabId="+tab, map[string]any{
		"text": text,
	})
	if code != 200 {
		t.Fatalf("clipboard write failed: %d %s", code, string(body))
	}

	code, body = httpGet(t, "/clipboard/read?tabId="+tab)
	if code != 200 {
		t.Fatalf("clipboard read failed: %d %s", code, string(body))
	}

	got := jsonField(t, body, "text")
	if got != text {
		t.Fatalf("clipboard read mismatch: got %q want %q", got, text)
	}
}

func TestClipboardCopyPaste(t *testing.T) {
	navigate(t, "https://example.com")
	if currentTabID == "" {
		t.Fatalf("missing current tab id after navigate")
	}

	tab := url.QueryEscape(currentTabID)

	text := fmt.Sprintf("pinchtab-clipboard-%d", time.Now().UnixNano())
	code, body := httpPost(t, "/clipboard/copy?tabId="+tab, map[string]any{
		"text": text,
	})
	if code != 200 {
		t.Fatalf("clipboard copy failed: %d %s", code, string(body))
	}

	code, body = httpPost(t, "/clipboard/paste?tabId="+tab, map[string]any{})
	if code != 200 {
		t.Fatalf("clipboard paste failed: %d %s", code, string(body))
	}

	got := jsonField(t, body, "text")
	if got != text {
		t.Fatalf("clipboard paste mismatch: got %q want %q", got, text)
	}
}
