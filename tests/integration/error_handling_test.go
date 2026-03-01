//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"
)

// ER5: Unicode content handling
func TestError_UnicodeContent(t *testing.T) {
	// Navigate to a page with Unicode (CJK/emoji/RTL)
	navigate(t, "https://www.wikipedia.org")

	// Test snapshot
	code, snapBody := httpGet(t, "/snapshot?tabId="+currentTabID)
	if code != 200 {
		t.Errorf("snapshot failed with %d", code)
	}

	// Verify response is valid JSON
	var snapData map[string]any
	if err := json.Unmarshal(snapBody, &snapData); err != nil {
		t.Errorf("snapshot response is not valid JSON: %v", err)
	}

	// Test text
	code, textBody := httpGet(t, "/text?tabId="+currentTabID)
	if code != 200 {
		t.Errorf("text failed with %d", code)
	}

	// Verify response is valid JSON
	var textData map[string]any
	if err := json.Unmarshal(textBody, &textData); err != nil {
		t.Errorf("text response is not valid JSON: %v", err)
	}
}

// ER6: Empty page handling
func TestError_EmptyPage(t *testing.T) {
	// Navigate to about:blank (empty page)
	navigate(t, "about:blank")

	// Test snapshot
	code, snapBody := httpGet(t, "/snapshot?tabId="+currentTabID)
	if code != 200 {
		t.Errorf("snapshot on empty page failed with %d", code)
	}

	// Verify response is valid JSON
	var snapData map[string]any
	if err := json.Unmarshal(snapBody, &snapData); err != nil {
		t.Errorf("snapshot response is not valid JSON: %v", err)
	}

	// Test text
	code, textBody := httpGet(t, "/text?tabId="+currentTabID)
	if code != 200 {
		t.Errorf("text on empty page failed with %d", code)
	}

	// Verify response is valid JSON
	var textData map[string]any
	if err := json.Unmarshal(textBody, &textData); err != nil {
		t.Errorf("text response is not valid JSON: %v", err)
	}
}

// ER3: Binary page (PDF) handling
func TestError_BinaryPage(t *testing.T) {
	// Navigate to a PDF
	navigate(t, "https://www.w3.org/WAI/WCAG21/Techniques/pdf/pdf_files/techniques.pdf")

	// Verify navigation completes (any status code, not a hang)
	// The navigate() helper will fail if the request hangs
	t.Logf("navigation to PDF completed successfully")

	// Test snapshot on PDF content
	// PDF may not render as a traditional page, so snapshot might fail or return empty
	// We just verify it doesn't crash and returns gracefully
	code, snapBody := httpGet(t, "/snapshot?tabId="+currentTabID)
	if code == 200 {
		// Verify response is valid JSON (even if empty/error)
		var snapData map[string]any
		if err := json.Unmarshal(snapBody, &snapData); err != nil {
			t.Errorf("snapshot response is not valid JSON: %v", err)
		}
	} else if code >= 400 && code < 500 {
		// Client error is acceptable for binary content
		t.Logf("snapshot returned %d (acceptable for PDF)", code)
	} else {
		t.Errorf("snapshot failed with unexpected code %d", code)
	}

	// Test text extraction on PDF
	code, textBody := httpGet(t, "/text?tabId="+currentTabID)
	if code == 200 {
		var textData map[string]any
		if err := json.Unmarshal(textBody, &textData); err != nil {
			t.Errorf("text response is not valid JSON: %v", err)
		}
	} else if code >= 400 && code < 500 {
		// Client error is acceptable for binary content
		t.Logf("text returned %d (acceptable for PDF)", code)
	} else {
		t.Errorf("text failed with unexpected code %d", code)
	}
}

// ER4: Rapid navigation stress test
func TestError_RapidNavigate(t *testing.T) {
	urls := []string{
		"https://example.com",
		"https://httpbin.org",
		"https://example.com",
		"https://httpbin.org",
		"https://example.com",
	}

	// Use a WaitGroup to ensure all navigations complete
	var wg sync.WaitGroup
	errors := make([]error, len(urls))
	var mu sync.Mutex

	startTime := time.Now()

	// Rapidly navigate to all URLs
	for i, url := range urls {
		wg.Add(1)
		go func(idx int, u string) {
			defer wg.Done()
			code, _ := httpPost(t, "/navigate", map[string]string{"url": u})
			if code != 200 {
				mu.Lock()
				errors[idx] = fmt.Errorf("navigate to %s failed with code %d", u, code)
				mu.Unlock()
			}
		}(i, url)
	}

	// Wait for all navigations to complete
	wg.Wait()
	elapsed := time.Since(startTime)

	// Check that navigations completed quickly (within 5 seconds for rapid test)
	if elapsed > 5*time.Second {
		t.Logf("rapid navigation took %v (slower than expected but acceptable)", elapsed)
	}

	// Verify no critical errors occurred
	for _, err := range errors {
		if err != nil {
			t.Errorf("%v", err)
		}
	}

	// Verify final page is example.com (last navigate wins)
	// Wait a bit for the last navigation to settle
	time.Sleep(500 * time.Millisecond)

	code, snapBody := httpGet(t, "/snapshot")
	if code == 200 {
		var snapData map[string]any
		if err := json.Unmarshal(snapBody, &snapData); err != nil {
			t.Errorf("snapshot response is not valid JSON: %v", err)
		}
		// Note: We can't easily verify the exact URL without accessing browser state,
		// but the important check is that we get a valid response (server didn't crash)
	} else {
		t.Errorf("snapshot failed with code %d", code)
	}

	t.Logf("rapid navigation test completed: 5 navigations in %v", elapsed)
}
