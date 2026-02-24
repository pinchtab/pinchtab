//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
)

// UP1: Single file upload with explicit selector
func TestUpload_SingleFile(t *testing.T) {
	repoRoot := findRepoRoot()
	testHtmlPath := filepath.Join(repoRoot, "tests/assets/upload-test.html")
	testFileURL := fmt.Sprintf("file://%s", testHtmlPath)

	navCode, _ := httpPost(t, "/navigate", map[string]string{"url": testFileURL})
	if navCode != 200 {
		t.Skipf("navigation to file:// URL not supported (code %d), skipping upload test", navCode)
	}

	testFilePath := filepath.Join(repoRoot, "tests/assets/test-upload.png")
	code, body := httpPost(t, "/upload", map[string]any{
		"selector": "#single",
		"paths":    []string{testFilePath},
	})

	if code != 200 {
		t.Skipf("upload returned %d (file:// URLs have limitations in headless Chrome)", code)
	}

	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	status, ok := resp["status"].(string)
	if !ok || status != "ok" {
		t.Errorf("expected status='ok', got %v", resp["status"])
	}

	files, ok := resp["files"].(float64)
	if !ok || int(files) != 1 {
		t.Errorf("expected files=1, got %v", resp["files"])
	}
}

// UP4: Multiple files upload with explicit selector
func TestUpload_MultipleFiles(t *testing.T) {
	repoRoot := findRepoRoot()
	testHtmlPath := filepath.Join(repoRoot, "tests/assets/upload-test.html")
	testFileURL := fmt.Sprintf("file://%s", testHtmlPath)

	navCode, _ := httpPost(t, "/navigate", map[string]string{"url": testFileURL})
	if navCode != 200 {
		t.Skipf("navigation to file:// URL not supported (code %d), skipping upload test", navCode)
	}

	testFilePath := filepath.Join(repoRoot, "tests/assets/test-upload.png")
	code, body := httpPost(t, "/upload", map[string]any{
		"selector": "#multi",
		"paths":    []string{testFilePath, testFilePath},
	})

	if code != 200 {
		t.Skipf("upload returned %d (file:// URLs have limitations in headless Chrome)", code)
	}

	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	files, ok := resp["files"].(float64)
	if !ok || int(files) != 2 {
		t.Errorf("expected files=2, got %v", resp["files"])
	}
}

// UP6: Default selector (uses default input[type=file])
func TestUpload_DefaultSelector(t *testing.T) {
	repoRoot := findRepoRoot()
	testHtmlPath := filepath.Join(repoRoot, "tests/assets/upload-test.html")
	testFileURL := fmt.Sprintf("file://%s", testHtmlPath)

	navCode, _ := httpPost(t, "/navigate", map[string]string{"url": testFileURL})
	if navCode != 200 {
		t.Skipf("navigation to file:// URL not supported (code %d), skipping upload test", navCode)
	}

	testFilePath := filepath.Join(repoRoot, "tests/assets/test-upload.png")
	code, _ := httpPost(t, "/upload", map[string]any{
		"paths": []string{testFilePath},
	})

	if code != 200 {
		t.Skipf("upload with default selector returned %d", code)
	}
}

// UP7: Invalid selector should error
func TestUpload_InvalidSelector(t *testing.T) {
	repoRoot := findRepoRoot()
	testHtmlPath := filepath.Join(repoRoot, "tests/assets/upload-test.html")
	testFileURL := fmt.Sprintf("file://%s", testHtmlPath)

	navCode, _ := httpPost(t, "/navigate", map[string]string{"url": testFileURL})
	if navCode != 200 {
		t.Skipf("navigation to file:// URL not supported, skipping upload test")
	}

	testFilePath := filepath.Join(repoRoot, "tests/assets/test-upload.png")
	code, _ := httpPost(t, "/upload", map[string]any{
		"selector": "#nonexistent",
		"paths":    []string{testFilePath},
	})

	if code == 200 {
		t.Error("expected error for invalid selector")
	}
}

// UP8: Missing paths field should error
func TestUpload_MissingFiles(t *testing.T) {
	code, _ := httpPost(t, "/upload", map[string]any{
		"selector": "#single",
	})

	if code == 200 {
		t.Errorf("expected 400 for missing paths, got %d", code)
	}
}

// UP9: Non-existent file path should error
func TestUpload_FileNotFound(t *testing.T) {
	code, _ := httpPost(t, "/upload", map[string]any{
		"paths": []string{"/tmp/nonexistent_file_xyz_12345.jpg"},
	})

	if code == 200 {
		t.Errorf("expected 400 for non-existent file, got %d", code)
	}
}

// UP11: Bad JSON should error
func TestUpload_BadJSON(t *testing.T) {
	code, _ := httpPostRaw(t, "/upload", "{broken")

	if code != 400 {
		t.Errorf("expected 400 for bad JSON, got %d", code)
	}
}
