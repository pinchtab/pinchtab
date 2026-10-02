package observe

import (
	"context"
	"testing"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
)

func requestWillBeSent(requestID, loaderID string) *network.EventRequestWillBeSent {
	return &network.EventRequestWillBeSent{
		RequestID: network.RequestID(requestID),
		LoaderID:  cdp.LoaderID(loaderID),
		Request:   &network.Request{URL: "https://example.test/" + requestID, Method: "GET"},
		Type:      network.ResourceTypeFetch,
	}
}

func frameNavigated(parentID, loaderID string) *page.EventFrameNavigated {
	return &page.EventFrameNavigated{Frame: &cdp.Frame{
		ID:       "frame",
		ParentID: cdp.FrameID(parentID),
		LoaderID: cdp.LoaderID(loaderID),
	}}
}

func TestTopFrameCommitAbandonsPreviousDocumentRequests(t *testing.T) {
	nm := NewNetworkMonitor(10)
	buf := nm.GetOrCreateBufferForTest("tab")
	_, completions := buf.SubscribeCompletions()

	nm.recordEvent(context.Background(), buf, requestWillBeSent("cut-fetch", "old-doc"))
	nm.recordEvent(context.Background(), buf, frameNavigated("", "new-doc"))

	if count, _ := buf.InflightStatus(); count != 0 {
		t.Fatalf("inflight after top-frame commit = %d, want 0", count)
	}
	entry, ok := buf.Get("cut-fetch")
	if !ok {
		t.Fatal("abandoned request missing from the network log")
	}
	if !entry.Finished || !entry.Failed || entry.Error != abandonedByNavigationError || entry.EndTime.IsZero() {
		t.Fatalf("abandoned entry = finished:%v failed:%v error:%q endTime:%v, want a failed, ended entry",
			entry.Finished, entry.Failed, entry.Error, entry.EndTime)
	}
	select {
	case id := <-completions:
		if id != "cut-fetch" {
			t.Fatalf("completion for %q, want cut-fetch", id)
		}
	default:
		t.Fatal("no completion published for the abandoned request")
	}
}

func TestTopFrameCommitKeepsCurrentDocumentAndWorkerRequests(t *testing.T) {
	nm := NewNetworkMonitor(10)
	buf := nm.GetOrCreateBufferForTest("tab")

	nm.recordEvent(context.Background(), buf, requestWillBeSent("document", "new-doc"))
	nm.recordEvent(context.Background(), buf, requestWillBeSent("early-fetch", "new-doc"))
	nm.recordEvent(context.Background(), buf, requestWillBeSent("worker-fetch", ""))
	nm.recordEvent(context.Background(), buf, frameNavigated("", "new-doc"))

	if count, _ := buf.InflightStatus(); count != 3 {
		t.Fatalf("inflight after commit = %d, want 3 (current document and worker requests still pending)", count)
	}
	if entry, _ := buf.Get("early-fetch"); entry.Finished || entry.Failed {
		t.Fatalf("current-document request was ended: %+v", entry)
	}
}

func TestSubframeCommitLeavesTopDocumentRequestsInFlight(t *testing.T) {
	nm := NewNetworkMonitor(10)
	buf := nm.GetOrCreateBufferForTest("tab")

	nm.recordEvent(context.Background(), buf, requestWillBeSent("top-fetch", "top-doc"))
	nm.recordEvent(context.Background(), buf, frameNavigated("top-frame", "iframe-doc"))

	if count, _ := buf.InflightStatus(); count != 1 {
		t.Fatalf("inflight after subframe commit = %d, want 1", count)
	}
}
