package actions

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/pinchtab/pinchtab/internal/heapsnap"
	"github.com/spf13/cobra"
)

func memoryCommand(t *testing.T, args ...string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "memory"}
	cmd.Flags().Bool("gc", false, "")
	cmd.Flags().Bool("json", false, "")
	cmd.Flags().String("tab", "", "")
	cmd.Flags().String("out", "", "")
	cmd.Flags().Int("top", 0, "")
	if err := cmd.Flags().Parse(args); err != nil {
		t.Fatalf("parse flags: %v", err)
	}
	return cmd
}

func TestMemoryUsageSendsGCAndTabAndPrintsTheCounters(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Method + " " + r.URL.RequestURI()
		_, _ = io.WriteString(w, `{"tabId":"t1","usedJSHeapSize":2097152,"totalJSHeapSize":4194304,"jsHeapSizeLimit":4294967296,"documents":1,"nodes":42,"listeners":3,"frames":1,"gc":true}`)
	}))
	defer srv.Close()

	out := captureStdout(t, func() {
		Memory(apiclient.Conn{Client: srv.Client(), Base: srv.URL}, memoryCommand(t, "--gc", "--tab", "t1"))
	})
	if got != "GET /memory?gc=true&tabId=t1" {
		t.Fatalf("request = %q", got)
	}
	for _, want := range []string{"heap used 2.0 MB of 4.0 MB", "(after gc)", "nodes=42", "listeners=3"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestMemorySnapshotCopiesToOutAndReportsTheLocalPath(t *testing.T) {
	serverFile := filepath.Join(t.TempDir(), "heap_1.heapsnapshot")
	if err := os.WriteFile(serverFile, []byte(`{"snapshot":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/memory/snapshot" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		encoded, _ := json.Marshal(map[string]any{"id": "heap_1", "path": serverFile, "bytes": 15, "nodeCount": 9, "durationMs": 12, "tabId": "t1"})
		_, _ = w.Write(encoded)
	}))
	defer srv.Close()

	local := filepath.Join(t.TempDir(), "copy.heapsnapshot")
	out := captureStdout(t, func() {
		MemorySnapshot(apiclient.Conn{Client: srv.Client(), Base: srv.URL}, memoryCommand(t, "--tab", "t1", "--out", local))
	})
	if body["tabId"] != "t1" {
		t.Fatalf("body = %v, want the tab forwarded", body)
	}
	copied, err := os.ReadFile(local)
	if err != nil || string(copied) != `{"snapshot":{}}` {
		t.Fatalf("copy = %q, %v", copied, err)
	}
	if !strings.Contains(out, "heap_1 → "+local) || !strings.Contains(out, "pinchtab memory summary heap_1") {
		t.Fatalf("output = %s", out)
	}
}

func TestMemorySummaryPrintsTheConstructorTable(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.RequestURI()
		_, _ = io.WriteString(w, `{"id":"heap_1","nodeCount":20,"edgeCount":6,"totalSelfSize":3536,
			"topBySize":[{"name":"Array","count":3,"selfSize":1664}],
			"topByCount":[{"name":"(string)","count":8,"selfSize":228}],
			"duplicateStrings":[{"value":"secret","length":6,"count":3,"selfSize":120}]}`)
	}))
	defer srv.Close()

	out := captureStdout(t, func() {
		MemorySummary(apiclient.Conn{Client: srv.Client(), Base: srv.URL}, memoryCommand(t, "--top", "5"), "heap_1")
	})
	if got != "/memory/snapshot/heap_1/summary?top=5" {
		t.Fatalf("request = %q", got)
	}
	for _, want := range []string{"heap_1: 20 nodes, 6 edges", "CONSTRUCTOR", "Array", "(string)", `"secret"`} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestMemoryComparePrintsTheDeltaTable(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.RequestURI()
		_, _ = io.WriteString(w, `{"top":5,"retained":true,
			"base":{"id":"heap_a","nodeCount":20,"edgeCount":6,"totalSelfSize":3536},
			"head":{"id":"heap_b","nodeCount":15,"edgeCount":17,"totalSelfSize":10820},
			"nodeDelta":-5,"sizeDelta":7284,"changed":7,
			"constructors":[{"name":"(array)","baseCount":1,"headCount":2,"countDelta":1,"baseSelfSize":800,"headSelfSize":10000,"sizeDelta":9200,"retainedSize":10000},
				{"name":"Array","baseCount":3,"headCount":2,"countDelta":-1,"baseSelfSize":1664,"headSelfSize":64,"sizeDelta":-1600,"retainedSize":10064}],
			"newDuplicateStrings":[{"value":"new-dup-string","length":14,"count":2,"selfSize":80}]}`)
	}))
	defer srv.Close()

	cmd := memoryCommand(t, "--top", "5")
	cmd.Flags().Bool("retained", false, "")
	if err := cmd.Flags().Set("retained", "true"); err != nil {
		t.Fatal(err)
	}
	out := captureStdout(t, func() {
		MemoryCompare(apiclient.Conn{Client: srv.Client(), Base: srv.URL}, cmd, "heap_a", "heap_b")
	})
	if got != "/memory/compare?base=heap_a&head=heap_b&retained=true&top=5" {
		t.Fatalf("request = %q", got)
	}
	lines := strings.Split(out, "\n")
	firstRow := ""
	for i, line := range lines {
		if strings.Contains(line, "CONSTRUCTOR") && i+1 < len(lines) {
			firstRow = lines[i+1]
		}
	}
	if !strings.Contains(firstRow, "(array)") || !strings.Contains(firstRow, "+9.0 KB") {
		t.Fatalf("first row = %q\n%s", firstRow, out)
	}
	for _, want := range []string{"heap_a → heap_b", "+7.1 KB", "7 constructors changed", "RETAINED", "-1.6 KB", "9.8 KB", `"new-dup-string"`} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestMemoryCompareWithoutRetainedPrintsNoRetainedColumn(t *testing.T) {
	out := formatMemoryComparison(memoryComparison{Comparison: heapsnap.Comparison{
		Base:         heapsnap.Totals{ID: "heap_a"},
		Head:         heapsnap.Totals{ID: "heap_b"},
		Constructors: []heapsnap.ConstructorDelta{{Name: "Array", CountDelta: 1, SizeDelta: 32}},
	}})
	if strings.Contains(out, "RETAINED") || !strings.Contains(out, "(none)") {
		t.Fatalf("output:\n%s", out)
	}
}
