package bridge

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

const uploadFixtureHTML = `<body>
<input type="file" id="target" multiple>
</body>`

func newUploadFixture(t *testing.T) context.Context {
	t.Helper()

	ctx := newActionTab(t, 30*time.Second)

	dataURL := "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(uploadFixtureHTML))
	if err := chromedp.Run(ctx,
		chromedp.Navigate(dataURL),
		chromedp.WaitVisible("#target", chromedp.ByID),
	); err != nil {
		t.Fatal(err)
	}
	return ctx
}

// stageFile writes content under dir/name and returns the path, so a test can
// choose the on-disk basename the browser will see.
func stageFile(t *testing.T, dir, name, content string) string {
	t.Helper()

	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func pageFileNames(t *testing.T, ctx context.Context) []string {
	t.Helper()

	var names []string
	if err := chromedp.Run(ctx, chromedp.Evaluate(
		`Array.from(document.getElementById('target').files).map(f => f.name)`, &names)); err != nil {
		t.Fatal(err)
	}
	return names
}

// The premise the whole filename fix rests on: the page-visible file.name is the
// BASENAME of the staged path, so preserving the caller's filename on disk is
// what makes forms gating on `accept=".csv"` or `file.name.endsWith(...)` pass.
// Nothing else can set it — CDP hands the input a path, not a name — so this is
// asserted against a real browser rather than assumed.
func TestSetFileInputFilesShowsTheStagedBasenameToThePage(t *testing.T) {
	ctx := newUploadFixture(t)

	staged := stageFile(t, t.TempDir(), "quarterly-report.csv", "a,b\n1,2\n")
	nodeID := uploadTargetNodeID(t, ctx)

	b := &Bridge{}
	if err := b.SetFileInputFiles(ctx, nodeID, []string{staged}); err != nil {
		t.Fatal(err)
	}

	names := pageFileNames(t, ctx)
	if len(names) != 1 || names[0] != "quarterly-report.csv" {
		t.Fatalf("page sees %v, want [quarterly-report.csv]", names)
	}
}

// Two files with the same basename is the case the per-index staging directory
// exists for: they must reach the page as two files under their real shared
// name, not one file or a de-duplicated rename.
func TestSetFileInputFilesKeepsDuplicateBasenamesFromSeparateDirs(t *testing.T) {
	ctx := newUploadFixture(t)

	base := t.TempDir()
	first := stageFile(t, filepath.Join(base, "0"), "data.csv", "first")
	second := stageFile(t, filepath.Join(base, "1"), "data.csv", "second")
	nodeID := uploadTargetNodeID(t, ctx)

	b := &Bridge{}
	if err := b.SetFileInputFiles(ctx, nodeID, []string{first, second}); err != nil {
		t.Fatal(err)
	}

	names := pageFileNames(t, ctx)
	if len(names) != 2 || names[0] != "data.csv" || names[1] != "data.csv" {
		t.Fatalf("page sees %v, want two files both named data.csv", names)
	}
}

func uploadTargetNodeID(t *testing.T, ctx context.Context) int64 {
	t.Helper()

	b := &Bridge{}
	nodeID, err := b.ResolveSelectorToNodeID(ctx, "#target", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	return nodeID
}
