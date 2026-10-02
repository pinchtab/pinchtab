package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
	"gopkg.in/yaml.v3"
)

func TestHandleSnapshot_NoTab(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/snapshot", nil)
	w := httptest.NewRecorder()
	h.HandleSnapshot(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleSnapshot_WithFilter(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/snapshot?filter=interactive&format=compact", nil)
	w := httptest.NewRecorder()
	h.HandleSnapshot(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleSnapshot_FileOutput_NoTab(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{StateDir: "/tmp"}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/snapshot?output=file", nil)
	w := httptest.NewRecorder()
	h.HandleSnapshot(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleSnapshot_PathTraversal(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{StateDir: "/tmp/pinchtab-test"}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/snapshot?output=file&path=../../etc/passwd", nil)
	w := httptest.NewRecorder()
	h.HandleSnapshot(w, req)
	// Should fail — either 404 (no tab) or 400 (path traversal)
	if w.Code == http.StatusOK {
		t.Error("expected error for path traversal")
	}
}

func TestHandleTabSnapshot_MissingTabID(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/tabs//snapshot", nil)
	w := httptest.NewRecorder()
	h.HandleTabSnapshot(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleTabSnapshot_NoTab(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/tabs/tab_123/snapshot", nil)
	req.SetPathValue("id", "tab_123")
	w := httptest.NewRecorder()
	h.HandleTabSnapshot(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

type wireSnapshot struct {
	FrameID string           `json:"frameId"`
	Nodes   []map[string]any `json:"nodes"`
}

func wireSnapshotFor(t *testing.T, h *Handlers, tabID string) wireSnapshot {
	t.Helper()
	var snap wireSnapshot
	if err := json.Unmarshal([]byte(snapshotBody(t, h, tabID, "json")), &snap); err != nil {
		t.Fatal(err)
	}
	if snap.FrameID == "" {
		t.Fatal("no top-level frameId, so the root frame the nodes omit is named nowhere")
	}
	if len(snap.Nodes) == 0 {
		t.Fatal("snapshot has no nodes")
	}
	return snap
}

func snapshotQueryBody(t *testing.T, h *Handlers, query string) string {
	t.Helper()
	w := httptest.NewRecorder()
	h.HandleSnapshot(w, httptest.NewRequest("GET", "/snapshot?"+query, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("snapshot?%s: status %d body=%s", query, w.Code, w.Body.String())
	}
	return w.Body.String()
}

func TestASingleFrameSnapshotCarriesNoPerNodeFrameFields(t *testing.T) {
	h, tabID := newEnrichmentFixture(t)

	for _, node := range wireSnapshotFor(t, h, tabID).Nodes {
		for _, key := range []string{"frameId", "frameUrl", "frameName"} {
			if _, ok := node[key]; ok {
				t.Errorf("node %v carries %s on a page with one frame", node["ref"], key)
			}
		}
	}
}

func TestAnIframeSnapshotKeepsFrameFieldsOnInFrameNodesOnly(t *testing.T) {
	h, tabID, _ := newFrameDisclosureFixture(t)
	snap := wireSnapshotFor(t, h, tabID)

	frameOf := func(name string) (string, string) {
		t.Helper()
		for _, node := range snap.Nodes {
			if node["name"] == name {
				id, _ := node["frameId"].(string)
				url, _ := node["frameUrl"].(string)
				return id, url
			}
		}
		t.Fatalf("no node named %q", name)
		return "", ""
	}

	if id, url := frameOf("OuterHeading"); id != "" || url != "" {
		t.Errorf("main-frame node carries frameId=%q frameUrl=%q", id, url)
	}
	id, url := frameOf("InnerHeading")
	if id == "" || id == snap.FrameID {
		t.Errorf("in-frame node frameId = %q, want the child frame (root is %q)", id, snap.FrameID)
	}
	if !strings.HasSuffix(url, "/inner.html") {
		t.Errorf("in-frame node frameUrl = %q, want the child document", url)
	}
}

func TestEverySnapshotEmitterNamesTheRootFrameOnce(t *testing.T) {
	h, tabID, _ := newFrameDisclosureFixture(t)
	rootFrameID := wireSnapshotFor(t, h, tabID).FrameID

	var file struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(snapshotQueryBody(t, h, "tabId="+tabID+"&output=file")), &file); err != nil {
		t.Fatal(err)
	}
	fileContent, err := os.ReadFile(file.Path)
	if err != nil {
		t.Fatal(err)
	}

	snapshotQueryBody(t, h, "tabId="+tabID+"&filter=interactive")
	diff := snapshotQueryBody(t, h, "tabId="+tabID+"&diff=true")
	if !strings.Contains(diff, "OuterHeading") {
		t.Fatalf("diff reports no main-frame node, so it cannot show one being stripped: %s", diff)
	}

	for emitter, body := range map[string]string{
		"yaml": snapshotBody(t, h, tabID, "yaml"),
		"file": string(fileContent),
		"diff": diff,
	} {
		if got := strings.Count(body, rootFrameID); got != 1 {
			t.Errorf("%s names the root frame %d times, want once at the top level", emitter, got)
		}
	}
}

func TestSnapshotDiffHonoursEveryFormat(t *testing.T) {
	h, tabID, _ := newFrameDisclosureFixture(t)
	diffIn := func(format string) (string, string) {
		t.Helper()
		snapshotQueryBody(t, h, "tabId="+tabID+"&filter=interactive")
		w := httptest.NewRecorder()
		h.HandleSnapshot(w, httptest.NewRequest("GET", "/snapshot?tabId="+tabID+"&diff=true&format="+format, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("diff (%s): status %d body=%s", format, w.Code, w.Body.String())
		}
		return w.Header().Get("Content-Type"), w.Body.String()
	}

	const unchangedLine, addedLine = `button "OuterBtn"`, `StaticText "OuterBtn" [+]`

	contentType, text := diffIn("text")
	if !strings.HasPrefix(contentType, "text/plain") {
		t.Errorf("text diff Content-Type = %q, want text/plain", contentType)
	}
	if !strings.Contains(text, " nodes | +") || !strings.Contains(text, addedLine) {
		t.Errorf("text diff lacks the counts header or the added node:\n%s", text)
	}
	if strings.Contains(text, unchangedLine) {
		t.Errorf("text diff repeats an unchanged node:\n%s", text)
	}

	contentType, compact := diffIn("compact")
	if !strings.HasPrefix(contentType, "text/plain") || !strings.Contains(compact, unchangedLine) {
		t.Errorf("compact diff (%s) no longer lists the whole tree:\n%s", contentType, compact)
	}

	contentType, jsonBody := diffIn("json")
	var jsonDiff map[string]any
	if err := json.Unmarshal([]byte(jsonBody), &jsonDiff); err != nil {
		t.Fatalf("json diff (%s) does not parse: %v", contentType, err)
	}

	contentType, yamlBody := diffIn("yaml")
	if !strings.HasPrefix(contentType, "text/yaml") {
		t.Errorf("yaml diff Content-Type = %q, want text/yaml", contentType)
	}
	var yamlDiff map[string]any
	if err := yaml.Unmarshal([]byte(yamlBody), &yamlDiff); err != nil {
		t.Fatalf("yaml diff does not parse: %v\n%s", err, yamlBody)
	}
	for key := range jsonDiff {
		if _, ok := yamlDiff[key]; !ok && key != "route" {
			t.Errorf("yaml diff lacks the json diff key %q", key)
		}
	}
}
