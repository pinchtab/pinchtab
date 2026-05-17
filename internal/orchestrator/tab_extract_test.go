package orchestrator

import (
	"bytes"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExtractExplicitTabID_Path(t *testing.T) {
	r := httptest.NewRequest("GET", "/x", nil)
	r.SetPathValue("id", "tab-from-path")
	got, src := ExtractExplicitTabID(r)
	if got != "tab-from-path" || src != TabIDSourcePath {
		t.Fatalf("got %q/%q; want tab-from-path/path", got, src)
	}
}

func TestExtractExplicitTabID_Query(t *testing.T) {
	r := httptest.NewRequest("GET", "/x?tabId=tab-q", nil)
	got, src := ExtractExplicitTabID(r)
	if got != "tab-q" || src != TabIDSourceQuery {
		t.Fatalf("got %q/%q; want tab-q/query", got, src)
	}
}

func TestExtractExplicitTabID_BodyJSON(t *testing.T) {
	body := []byte(`{"url":"about:blank","tabId":"tab-b"}`)
	r := httptest.NewRequest("POST", "/navigate", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.ContentLength = int64(len(body))

	got, src := ExtractExplicitTabID(r)
	if got != "tab-b" || src != TabIDSourceBody {
		t.Fatalf("got %q/%q; want tab-b/body", got, src)
	}

	// Body must be readable downstream.
	rest, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("body read after peek: %v", err)
	}
	if !bytes.Equal(rest, body) {
		t.Fatalf("body after peek = %q; want %q", rest, body)
	}
}

func TestExtractExplicitTabID_BodyWithCharset(t *testing.T) {
	body := []byte(`{"tabId":"tab-c"}`)
	r := httptest.NewRequest("POST", "/navigate", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json; charset=utf-8")
	r.ContentLength = int64(len(body))

	got, src := ExtractExplicitTabID(r)
	if got != "tab-c" || src != TabIDSourceBody {
		t.Fatalf("got %q/%q; want tab-c/body", got, src)
	}
}

func TestExtractExplicitTabID_BodySkippedForNonJSON(t *testing.T) {
	body := []byte(`{"tabId":"sneaky"}`)
	r := httptest.NewRequest("POST", "/x", bytes.NewReader(body))
	r.Header.Set("Content-Type", "text/plain")
	r.ContentLength = int64(len(body))

	got, src := ExtractExplicitTabID(r)
	if got != "" || src != TabIDSourceNone {
		t.Fatalf("non-JSON body should not be peeked, got %q/%q", got, src)
	}
}

func TestExtractExplicitTabID_BodySkippedWhenOversized(t *testing.T) {
	r := httptest.NewRequest("POST", "/x", bytes.NewReader([]byte(`{"tabId":"too-big"}`)))
	r.Header.Set("Content-Type", "application/json")
	r.ContentLength = maxBodyPeek + 1

	got, _ := ExtractExplicitTabID(r)
	if got != "" {
		t.Fatalf("oversized body should be skipped, got %q", got)
	}
}

func TestExtractExplicitTabID_BodySkippedWhenUnknownLength(t *testing.T) {
	r := httptest.NewRequest("POST", "/x", bytes.NewReader([]byte(`{"tabId":"streaming"}`)))
	r.Header.Set("Content-Type", "application/json")
	r.ContentLength = -1

	got, _ := ExtractExplicitTabID(r)
	if got != "" {
		t.Fatalf("unknown-length body should be skipped, got %q", got)
	}
}

func TestExtractExplicitTabID_QueryWinsOverBody(t *testing.T) {
	body := []byte(`{"tabId":"from-body"}`)
	r := httptest.NewRequest("POST", "/x?tabId=from-query", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.ContentLength = int64(len(body))

	got, src := ExtractExplicitTabID(r)
	if got != "from-query" || src != TabIDSourceQuery {
		t.Fatalf("query should win; got %q/%q", got, src)
	}
}

func TestExtractExplicitTabID_BadJSONLeavesBodyIntact(t *testing.T) {
	body := []byte(`{not-json`)
	r := httptest.NewRequest("POST", "/x", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.ContentLength = int64(len(body))

	got, _ := ExtractExplicitTabID(r)
	if got != "" {
		t.Fatalf("bad JSON should yield no tab id, got %q", got)
	}
	rest, _ := io.ReadAll(r.Body)
	if !bytes.Equal(rest, body) {
		t.Fatalf("body after failed peek = %q; want %q", rest, body)
	}
}

func TestExtractExplicitTabID_TrimsWhitespace(t *testing.T) {
	r := httptest.NewRequest("GET", "/x?tabId=%20%20", nil)
	got, _ := ExtractExplicitTabID(r)
	if got != "" {
		t.Fatalf("whitespace-only tab id should be empty, got %q", got)
	}
}

func TestExtractExplicitTabID_LongOKBody(t *testing.T) {
	// Pad body up close to (but under) the budget to exercise the
	// LimitReader sentinel path.
	pad := strings.Repeat(" ", maxBodyPeek-32)
	body := []byte(`{"tabId":"tail"` + pad + `}`)
	if len(body) > maxBodyPeek {
		t.Fatalf("test body %d > budget %d", len(body), maxBodyPeek)
	}
	r := httptest.NewRequest("POST", "/x", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.ContentLength = int64(len(body))

	got, _ := ExtractExplicitTabID(r)
	if got != "tail" {
		t.Fatalf("got %q, want tail", got)
	}
}

func TestExtractRequestedBrowserTarget_Query(t *testing.T) {
	r := httptest.NewRequest("GET", "/navigate?browserTarget=cloak-1", nil)
	if got := ExtractRequestedBrowserTarget(r); got != "cloak-1" {
		t.Fatalf("got %q, want cloak-1", got)
	}
}

func TestExtractRequestedBrowserTarget_BodyJSON(t *testing.T) {
	body := []byte(`{"url":"about:blank","browserTarget":"cloak-1"}`)
	r := httptest.NewRequest("POST", "/navigate", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.ContentLength = int64(len(body))

	if got := ExtractRequestedBrowserTarget(r); got != "cloak-1" {
		t.Fatalf("got %q, want cloak-1", got)
	}

	rest, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("body read after peek: %v", err)
	}
	if !bytes.Equal(rest, body) {
		t.Fatalf("body after peek = %q; want %q", rest, body)
	}
}

func TestExtractRequestedBrowserTarget_QueryWinsOverBody(t *testing.T) {
	body := []byte(`{"browserTarget":"body-target"}`)
	r := httptest.NewRequest("POST", "/navigate?browserTarget=query-target", bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.ContentLength = int64(len(body))

	if got := ExtractRequestedBrowserTarget(r); got != "query-target" {
		t.Fatalf("got %q, want query-target", got)
	}
}
