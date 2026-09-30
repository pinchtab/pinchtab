package observe

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/heapsnap"
)

const leakPage = `<html><body><script>window.leak = []; function grow(){ window.leak.push(Array.from({length: 1500000}, (_, i) => i + 0.5)); }</script></body></html>`

func TestGarbageCollectionReclaimsADroppedArray(t *testing.T) {
	tab := openTab(t, launchBrowser(t), leakPage)
	baseline, err := ReadHeapUsage(tab, true)
	if err != nil {
		t.Fatalf("baseline: %v", err)
	}
	if err := chromedp.Run(tab, chromedp.Evaluate(`grow(); grow(); true`, nil)); err != nil {
		t.Fatal(err)
	}
	grown, err := ReadHeapUsage(tab, false)
	if err != nil {
		t.Fatalf("grown: %v", err)
	}
	if grown.UsedJSHeapSize < baseline.UsedJSHeapSize+16<<20 {
		t.Fatalf("used heap %d after two 12MB arrays, baseline %d: the leak did not show", grown.UsedJSHeapSize, baseline.UsedJSHeapSize)
	}
	if err := chromedp.Run(tab, chromedp.Evaluate(`window.leak = null; true`, nil)); err != nil {
		t.Fatal(err)
	}
	collected, err := ReadHeapUsage(tab, true)
	if err != nil {
		t.Fatalf("collected: %v", err)
	}
	if !collected.GC || grown.GC {
		t.Fatalf("gc flags = %v/%v, want the reading to say whether it collected first", grown.GC, collected.GC)
	}
	if collected.UsedJSHeapSize > grown.UsedJSHeapSize-16<<20 {
		t.Fatalf("used heap %d after release and gc, %d before: gc did not reclaim the dropped arrays", collected.UsedJSHeapSize, grown.UsedJSHeapSize)
	}
	if collected.TotalJSHeapSize < collected.UsedJSHeapSize || collected.JSHeapSizeLimit <= collected.TotalJSHeapSize {
		t.Errorf("heap sizes used=%d total=%d limit=%d, want used <= total < limit", collected.UsedJSHeapSize, collected.TotalJSHeapSize, collected.JSHeapSizeLimit)
	}
	if collected.Documents < 1 || collected.Frames < 1 || collected.Nodes < 1 {
		t.Errorf("counters = %+v, want a live document, frame and nodes", collected)
	}
}

func TestHeapSnapshotStreamsAValidV8Snapshot(t *testing.T) {
	tab := openTab(t, launchBrowser(t), `<html><body><script>window.keep = Array.from({length: 50}, () => [1, 2, 3]);</script></body></html>`)
	path := filepath.Join(t.TempDir(), "tab"+heapsnap.Ext)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	chunks := 0
	sink := heapsnap.NewSink(f, 0)
	err = TakeHeapSnapshot(tab, func(chunk string) error {
		chunks++
		return sink.WriteChunk(chunk)
	})
	if cerr := f.Close(); cerr != nil {
		t.Fatal(cerr)
	}
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() != sink.Bytes() || chunks < 2 {
		t.Fatalf("file %v size vs sink %d over %d chunks: %v", info, sink.Bytes(), chunks, err)
	}

	raw, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	header, err := heapsnap.ReadHeader(raw)
	_ = raw.Close()
	if err != nil {
		t.Fatalf("header: %v", err)
	}
	if len(header.Meta.NodeFields) == 0 || header.NodeCount == 0 {
		t.Fatalf("header = %+v, want snapshot.meta with node fields and a node count", header)
	}
	agg, err := heapsnap.ParseFileWith(path, heapsnap.ParseOptions{})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if agg.NodeCount != header.NodeCount {
		t.Fatalf("nodes array holds %d nodes, node_count says %d", agg.NodeCount, header.NodeCount)
	}
	found := false
	for _, c := range agg.Summary(heapsnap.MaxTop).TopByCount {
		if c.Name == "Array" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Array missing from the constructors of a page holding 51 arrays")
	}
}

func TestHeapSnapshotStopsAtTheSinkRefusalAndLeavesTheTabUsable(t *testing.T) {
	tab := openTab(t, launchBrowser(t), "<html><body>hi</body></html>")
	sink := heapsnap.NewSink(discard{}, 1024)
	err := TakeHeapSnapshot(tab, sink.WriteChunk)
	if !errors.Is(err, heapsnap.ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
	if _, err := ReadHeapUsage(tab, false); err != nil {
		t.Fatalf("tab unusable after an aborted snapshot: %v", err)
	}
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }
