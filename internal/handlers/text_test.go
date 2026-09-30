package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"strings"
	"unicode/utf8"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/config"
)

func TestHandleText_NoTab(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/text", nil)
	w := httptest.NewRecorder()
	h.HandleText(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleText_WithTabId(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/text?tabId=nonexistent", nil)
	w := httptest.NewRecorder()
	h.HandleText(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleText_RawMode(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/text?mode=raw", nil)
	w := httptest.NewRecorder()
	h.HandleText(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleTabText_MissingTabID(t *testing.T) {
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/tabs//text", nil)
	w := httptest.NewRecorder()
	h.HandleTabText(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleTabText_NoTab(t *testing.T) {
	h := New(&mockBridge{failTab: true}, &config.RuntimeConfig{}, nil, nil, nil)
	req := httptest.NewRequest("GET", "/tabs/tab_abc/text", nil)
	req.SetPathValue("id", "tab_abc")
	w := httptest.NewRecorder()
	h.HandleTabText(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

const landingPageRawText = `PinchTab — browser control for AI agents

Install: curl -fsSL https://pinchtab.com/install.sh | sh

Endpoints: /navigate /snapshot /text /action /capture /find /evaluate

🎯 Accessibility Tree — structured tree with stable refs (e0, e1...) for click, type, and read. Deterministic with no coordinate guessing.
🧭 Navigation — drive tabs, frames, and history without a headful browser in the loop.
📄 Text extraction — readable page text with an explicit raw mode when the heuristic collapses.
🖼️ Capture — screenshots and PDFs of the live page, viewport or full document.
🔍 Find — semantic element lookup that survives markup churn.
🧪 Evaluate — run JavaScript in the page or in a specific frame.
🔐 Profiles — reuse a dedicated automation profile with explicit approval.
🧰 Macros — batch several actions into one request.
📊 Activity — every request recorded with the route that served it.`

const collapsedReadabilityText = `🎯 Accessibility Tree — structured tree with stable refs (e0, e1...) for click, type, and read.`

func textScriptBridge(t *testing.T, readability, raw string) *mockBridge {
	t.Helper()
	return &mockBridge{
		evaluateFn: func(expression string, result any) error {
			out, ok := result.(*string)
			if !ok {
				t.Fatalf("evaluate result is %T, want *string", result)
			}
			if expression == rawTextScript {
				*out = raw
				return nil
			}
			*out = readability
			return nil
		},
	}
}

func TestExtractDocumentText_ReadabilityCollapseFallsBackToRaw(t *testing.T) {
	m := textScriptBridge(t, collapsedReadabilityText, landingPageRawText)
	h := New(m, &config.RuntimeConfig{}, nil, nil, nil)

	extraction, err := h.extractDocumentText(context.Background(), "", "")
	if err != nil {
		t.Fatalf("extractDocumentText: %v", err)
	}
	if extraction.Mode != extractionReadabilityFallback {
		t.Errorf("Mode = %q, want %q", extraction.Mode, extractionReadabilityFallback)
	}
	if extraction.Text != landingPageRawText {
		t.Errorf("Text = %q, want the raw document text", extraction.Text)
	}
	if !extraction.RawKnown || extraction.RawLength != utf8.RuneCountInString(landingPageRawText) {
		t.Errorf("RawLength = %d (known=%v), want %d", extraction.RawLength, extraction.RawKnown, utf8.RuneCountInString(landingPageRawText))
	}
}

func TestExtractDocumentText_ArticleKeepsReadabilityOutput(t *testing.T) {
	landingRunes := []rune(landingPageRawText)
	article := string(landingRunes[:len(landingRunes)*9/10])
	m := textScriptBridge(t, article, landingPageRawText)
	h := New(m, &config.RuntimeConfig{}, nil, nil, nil)

	extraction, err := h.extractDocumentText(context.Background(), "", "")
	if err != nil {
		t.Fatalf("extractDocumentText: %v", err)
	}
	if extraction.Mode != extractionReadability {
		t.Errorf("Mode = %q, want %q", extraction.Mode, extractionReadability)
	}
	if extraction.Text != article {
		t.Errorf("Text = %q, want the readability output", extraction.Text)
	}
	if extraction.RawLength != utf8.RuneCountInString(landingPageRawText) {
		t.Errorf("RawLength = %d, want %d", extraction.RawLength, utf8.RuneCountInString(landingPageRawText))
	}
}

// The same word selected opposite behaviour on the two surfaces: the CLI's --full sends
// mode=raw and returns innerText, while the API's ?mode=full fell through to readability.
// full is now the alias the CLI already implies, and a mode nobody implemented is refused
// rather than silently downgraded.
func TestResolveTextMode(t *testing.T) {
	for _, tc := range []struct {
		requested string
		want      string
	}{
		{"", ""},
		{"raw", "raw"},
		{"full", "raw"},
		{"FULL", "raw"},
		{"  raw  ", "raw"},
		{"markdown", "markdown"},
		{"MARKDOWN", "markdown"},
		{"  markdown  ", "markdown"},
	} {
		got, err := resolveTextMode(tc.requested)
		if err != nil {
			t.Errorf("resolveTextMode(%q) refused: %v", tc.requested, err)
			continue
		}
		if got != tc.want {
			t.Errorf("resolveTextMode(%q) = %q, want %q", tc.requested, got, tc.want)
		}
	}
}

func TestResolveTextMode_RefusesAnUnimplementedMode(t *testing.T) {
	_, err := resolveTextMode("readable")
	if err == nil {
		t.Fatal("an unimplemented mode was accepted; the caller gets readability while believing they asked for something else")
	}
	message := err.Error()
	if !strings.Contains(message, `"readable"`) {
		t.Errorf("the refusal does not name what the caller sent: %s", message)
	}
	for _, accepted := range []string{"full", "markdown", "raw"} {
		if !strings.Contains(message, accepted) {
			t.Errorf("the refusal does not name the accepted value %q: %s", accepted, message)
		}
	}
	if !strings.Contains(message, "full, markdown, raw") {
		t.Errorf("the accepted values are not in a stable order, so the refusal differs between runs: %s", message)
	}
}

// full must be byte-identical to raw, not merely close: the two are the same extraction.
func TestExtractDocumentText_FullIsTheSameExtractionAsRaw(t *testing.T) {
	extractions := map[string]textExtraction{}
	for _, requested := range []string{"raw", "full"} {
		mode, err := resolveTextMode(requested)
		if err != nil {
			t.Fatalf("resolveTextMode(%q): %v", requested, err)
		}
		m := textScriptBridge(t, collapsedReadabilityText, landingPageRawText)
		h := New(m, &config.RuntimeConfig{}, nil, nil, nil)
		extraction, err := h.extractDocumentText(context.Background(), mode, "")
		if err != nil {
			t.Fatalf("extractDocumentText(%q): %v", requested, err)
		}
		extractions[requested] = extraction
	}

	if extractions["full"] != extractions["raw"] {
		t.Errorf("?mode=full and ?mode=raw produced different extractions:\n full %+v\n raw  %+v", extractions["full"], extractions["raw"])
	}
	if extractions["raw"].Text != landingPageRawText {
		t.Errorf("raw extraction = %q, want the raw document text unchanged", extractions["raw"].Text)
	}
}

func TestExtractDocumentText_RawModeSkipsCoverageComparison(t *testing.T) {
	m := textScriptBridge(t, collapsedReadabilityText, landingPageRawText)
	h := New(m, &config.RuntimeConfig{}, nil, nil, nil)

	extraction, err := h.extractDocumentText(context.Background(), "raw", "")
	if err != nil {
		t.Fatalf("extractDocumentText: %v", err)
	}
	if extraction.Mode != extractionRaw {
		t.Errorf("Mode = %q, want %q", extraction.Mode, extractionRaw)
	}
	if len(m.evaluateExprs) != 1 || m.evaluateExprs[0] != rawTextScript {
		t.Fatalf("mode=raw ran %d extractions (%v), want exactly the raw one", len(m.evaluateExprs), m.evaluateExprs)
	}
}

func TestReadabilityCollapsed_FloorKeepsShortPages(t *testing.T) {
	if readabilityCollapsed(1, readabilityCoverageFloorChars-1) {
		t.Error("a page below the character floor must not trigger the fallback")
	}
	if !readabilityCollapsed(1, readabilityCoverageFloorChars) {
		t.Error("a near-total collapse at the floor must trigger the fallback")
	}
	atRatio := int(float64(readabilityCoverageFloorChars) * readabilityCoverageRatio)
	if readabilityCollapsed(atRatio, readabilityCoverageFloorChars) {
		t.Error("coverage at the ratio must not trigger the fallback")
	}
}

func textResponseRecorder(t *testing.T, extraction textExtraction, maxChars int, format string) *httptest.ResponseRecorder {
	t.Helper()
	return scopedTextResponseRecorder(t, extraction, maxChars, format, nil)
}

// scopedTextResponseRecorder drives the same writer with a frame disclosure, which is the
// only difference a scoped read makes to this envelope.
func scopedTextResponseRecorder(t *testing.T, extraction textExtraction, maxChars int, format string, scope *frameDisclosure) *httptest.ResponseRecorder {
	t.Helper()
	h := New(&mockBridge{}, &config.RuntimeConfig{}, nil, nil, nil)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/text", nil)
	h.writeTextResponse(w, r, context.Background(), extraction, maxChars, format, nil, scope)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	return w
}

func decodeTextEnvelope(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var envelope map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, w.Body.String())
	}
	return envelope
}

func TestWriteTextResponse_EnvelopeReportsExtractionAndLengths(t *testing.T) {
	extraction := textExtraction{
		Text:      landingPageRawText,
		Mode:      extractionReadabilityFallback,
		RawLength: utf8.RuneCountInString(landingPageRawText),
		RawKnown:  true,
	}
	w := textResponseRecorder(t, extraction, -1, "")
	envelope := decodeTextEnvelope(t, w)

	if envelope["extraction"] != extractionReadabilityFallback {
		t.Errorf("extraction = %v, want %q", envelope["extraction"], extractionReadabilityFallback)
	}
	if envelope["textLength"] != float64(utf8.RuneCountInString(landingPageRawText)) {
		t.Errorf("textLength = %v, want %d", envelope["textLength"], utf8.RuneCountInString(landingPageRawText))
	}
	if envelope["rawLength"] != float64(utf8.RuneCountInString(landingPageRawText)) {
		t.Errorf("rawLength = %v, want %d", envelope["rawLength"], utf8.RuneCountInString(landingPageRawText))
	}
	if envelope["truncated"] != false {
		t.Errorf("truncated = %v, want false: a fallback is not a truncation", envelope["truncated"])
	}
}

func TestWriteTextResponse_TruncatedOnlyWhenMaxCharsCuts(t *testing.T) {
	extraction := textExtraction{Text: landingPageRawText, Mode: extractionReadabilityFallback, RawLength: utf8.RuneCountInString(landingPageRawText), RawKnown: true}

	cut := decodeTextEnvelope(t, textResponseRecorder(t, extraction, 50, ""))
	if cut["truncated"] != true {
		t.Errorf("truncated = %v, want true when maxChars cuts", cut["truncated"])
	}
	if cut["textLength"] != float64(50) {
		t.Errorf("textLength = %v, want the returned length 50", cut["textLength"])
	}

	whole := decodeTextEnvelope(t, textResponseRecorder(t, extraction, utf8.RuneCountInString(landingPageRawText)+1, ""))
	if whole["truncated"] != false {
		t.Errorf("truncated = %v, want false when maxChars does not cut", whole["truncated"])
	}
}

func TestWriteTextResponse_PlainFormatKeepsBareBodyAndReportsModeInHeader(t *testing.T) {
	extraction := textExtraction{Text: landingPageRawText, Mode: extractionReadabilityFallback, RawLength: utf8.RuneCountInString(landingPageRawText), RawKnown: true}
	w := textResponseRecorder(t, extraction, -1, "text")

	if w.Body.String() != landingPageRawText {
		t.Errorf("body = %q, want the bare text", w.Body.String())
	}
	if got := w.Header().Get(headerTextExtraction); got != extractionReadabilityFallback {
		t.Errorf("%s = %q, want %q", headerTextExtraction, got, extractionReadabilityFallback)
	}
}

type frameTextBridge struct {
	mockBridge
	frames      []string
	scripts     []string
	readability string
	raw         string
}

func (b *frameTextBridge) EvaluateInFrame(_ context.Context, frameID, expression string, result any, _ bridge.EvalOpts) error {
	b.frames = append(b.frames, frameID)
	b.scripts = append(b.scripts, expression)
	out, ok := result.(*string)
	if !ok {
		return fmt.Errorf("evaluate result is %T, want *string", result)
	}
	if expression == rawTextScript {
		*out = b.raw
		return nil
	}
	*out = b.readability
	return nil
}

func TestExtractDocumentText_BaselineUsesTheSameFrameTraversal(t *testing.T) {
	b := &frameTextBridge{readability: collapsedReadabilityText, raw: landingPageRawText}
	h := New(b, &config.RuntimeConfig{}, nil, nil, nil)

	extraction, err := h.extractDocumentText(context.Background(), "", "FRAME7")
	if err != nil {
		t.Fatalf("extractDocumentText: %v", err)
	}
	if extraction.Mode != extractionReadabilityFallback {
		t.Fatalf("Mode = %q, want %q", extraction.Mode, extractionReadabilityFallback)
	}
	if len(b.frames) != 2 || b.frames[0] != "FRAME7" || b.frames[1] != "FRAME7" {
		t.Fatalf("frames = %v, want both extractions scoped to FRAME7", b.frames)
	}
	if b.evaluateCalls != 0 {
		t.Errorf("baseline escaped the frame scope: %d top-frame evaluates (%v)", b.evaluateCalls, b.evaluateExprs)
	}
	if b.scripts[1] != rawTextScript {
		t.Errorf("baseline script = %q, want the raw text script", b.scripts[1])
	}
}

func TestExtractDocumentText_CoverageFloorCountsCharactersNotBytes(t *testing.T) {
	raw := strings.Repeat("測", 200)
	if len(raw) < readabilityCoverageFloorChars {
		t.Fatalf("fixture must exceed the floor in bytes to discriminate: %d", len(raw))
	}
	if utf8.RuneCountInString(raw) >= readabilityCoverageFloorChars {
		t.Fatalf("fixture must sit under the floor in characters: %d", utf8.RuneCountInString(raw))
	}
	collapsed := string([]rune(raw)[:5])

	m := textScriptBridge(t, collapsed, raw)
	h := New(m, &config.RuntimeConfig{}, nil, nil, nil)

	extraction, err := h.extractDocumentText(context.Background(), "", "")
	if err != nil {
		t.Fatalf("extractDocumentText: %v", err)
	}
	if extraction.Mode != extractionReadability {
		t.Fatalf("Mode = %q, want %q: a page under the character floor must not trigger the fallback", extraction.Mode, extractionReadability)
	}
	if extraction.RawLength != utf8.RuneCountInString(raw) {
		t.Fatalf("RawLength = %d, want %d characters", extraction.RawLength, utf8.RuneCountInString(raw))
	}
}

// markdownFixtureHTML is a rendered article the seaportal converter turns into
// Markdown: a heading, two subheadings, an inline link, a list and a 3-column
// table, with enough prose that extraction does not treat it as thin.
const markdownFixtureHTML = `<!doctype html><html><head><title>Doc Title</title>` +
	`<meta name="description" content="A short description of the doc."></head><body><article>` +
	`<h1>Main Heading</h1>` +
	`<h2>First Section</h2>` +
	`<p>A paragraph with an <a href="https://example.com/link">inline link</a> in it that keeps ` +
	`going with more words so extraction triggers properly and does not collapse to nothing.</p>` +
	`<h2>Second Section</h2>` +
	`<ul><li>Alpha item one here</li><li>Beta item two here</li><li>Gamma item three here</li></ul>` +
	`<table><thead><tr><th>Col A</th><th>Col B</th><th>Col C</th></tr></thead>` +
	`<tbody><tr><td>a1</td><td>b1</td><td>c1</td></tr><tr><td>a2</td><td>b2</td><td>c2</td></tr></tbody></table>` +
	`<p>More trailing text so the article body sits comfortably above any thin-content threshold ` +
	`the extractor applies before it will hand back a document.</p></article></body></html>`

// markdownBridge answers the frame-scoped HTML inspect read with a fixed
// document and the raw-text script with fixed text, so the markdown extraction
// path can be exercised without a real browser.
type markdownBridge struct {
	mockBridge
	html string
	url  string
	raw  string
}

func (b *markdownBridge) EvaluateInFrame(_ context.Context, _ string, _ string, result any, _ bridge.EvalOpts) error {
	switch out := result.(type) {
	case *inspectPayload:
		out.HTML = b.html
		out.URL = b.url
		return nil
	case *string:
		*out = b.raw
		return nil
	default:
		return fmt.Errorf("evaluate result is %T, want *inspectPayload or *string", result)
	}
}

func TestExtractDocumentMarkdown_ConvertsRenderedHTMLToMarkdown(t *testing.T) {
	b := &markdownBridge{html: markdownFixtureHTML, url: "http://example.test/page"}
	h := New(b, &config.RuntimeConfig{}, nil, nil, nil)

	extraction, err := h.extractDocumentText(context.Background(), extractionMarkdown, "FRAME1")
	if err != nil {
		t.Fatalf("extractDocumentText: %v", err)
	}
	if extraction.Mode != extractionMarkdown {
		t.Fatalf("Mode = %q, want %q", extraction.Mode, extractionMarkdown)
	}
	if extraction.Title != "Doc Title" {
		t.Errorf("Title = %q, want the converter title", extraction.Title)
	}
	if extraction.Description != "A short description of the doc." {
		t.Errorf("Description = %q, want the converter description", extraction.Description)
	}
	for _, syntax := range []string{"# ", "## ", "[inline link](https://example.com/link)", "- Alpha item one here", "|"} {
		if !strings.Contains(extraction.Text, syntax) {
			t.Errorf("markdown does not contain %q:\n%s", syntax, extraction.Text)
		}
	}
	if extraction.RawKnown {
		t.Errorf("RawKnown = true; markdown has no raw-length comparison to report")
	}
}

func TestExtractDocumentMarkdown_EmptyConversionFallsBackToRaw(t *testing.T) {
	b := &markdownBridge{html: "<html><body></body></html>", url: "http://example.test/empty", raw: landingPageRawText}
	h := New(b, &config.RuntimeConfig{}, nil, nil, nil)

	extraction, err := h.extractDocumentText(context.Background(), extractionMarkdown, "FRAME1")
	if err != nil {
		t.Fatalf("extractDocumentText: %v", err)
	}
	if extraction.Mode != extractionMarkdownFallback {
		t.Fatalf("Mode = %q, want %q when the converter yields nothing", extraction.Mode, extractionMarkdownFallback)
	}
	if extraction.Text != landingPageRawText {
		t.Errorf("Text = %q, want the raw document text", extraction.Text)
	}
	if !extraction.RawKnown || extraction.RawLength != utf8.RuneCountInString(landingPageRawText) {
		t.Errorf("RawLength = %d (known=%v), want %d", extraction.RawLength, extraction.RawKnown, utf8.RuneCountInString(landingPageRawText))
	}
}

// A table row that overruns is dropped, never split: every returned line must be
// a whole source line, and the result must not be empty (the header rows fit).
func TestTruncateCharsLine_NeverSplitsTableRow(t *testing.T) {
	body := "| Col A | Col B | Col C |\n|-------|-------|-------|\n| a1 | b1 | c1 |\n| a2 | b2 | c2 |\n| a3 | b3 | c3 |"
	cut, truncated := truncateCharsLine(body, 60)
	if !truncated {
		t.Fatalf("expected a cut at 60 chars of a %d-char body", utf8.RuneCountInString(body))
	}
	if cut == "" {
		t.Fatalf("result is empty; the header rows fit within 60 chars and must survive")
	}
	sourceLines := map[string]bool{}
	for _, line := range strings.Split(body, "\n") {
		sourceLines[line] = true
	}
	for _, line := range strings.Split(cut, "\n") {
		if !sourceLines[line] {
			t.Fatalf("returned line %q is not a whole source line; a table row was split:\n%s", line, cut)
		}
	}
	if utf8.RuneCountInString(cut) > 60 {
		t.Errorf("cut kept %d chars, over the 60 limit", utf8.RuneCountInString(cut))
	}
}

// seaportal emits each paragraph as ONE line, so a page opening with a long
// paragraph must still return a rune-cut of it — not an empty body. This fails on
// the whole-lines-only helper, which dropped the first overrunning line.
func TestTruncateCharsLine_RuneCutsLongFirstLine(t *testing.T) {
	body := strings.Repeat("word ", 60) + "end" // one ~303-char paragraph, no newlines
	if utf8.RuneCountInString(body) <= 200 {
		t.Fatalf("fixture must exceed the limit; got %d chars", utf8.RuneCountInString(body))
	}
	cut, truncated := truncateCharsLine(body, 200)
	if !truncated {
		t.Fatal("expected a cut")
	}
	if cut == "" {
		t.Fatal("a long first paragraph must be rune-cut, not dropped to empty")
	}
	if n := utf8.RuneCountInString(cut); n == 0 || n > 200 {
		t.Fatalf("cut kept %d chars, want 1..200", n)
	}
	if !strings.HasPrefix(body, cut) {
		t.Fatalf("the partial line must be a prefix of the source paragraph:\n%q", cut)
	}
}

// A cut that would land inside a [..](..) link is pulled back to before the
// link, never emitting a half-open one — and the text before the link survives
// rather than the whole line being dropped.
func TestTruncateCharsLine_NeverSplitsLink(t *testing.T) {
	assertNoPartialLink := func(t *testing.T, cut string) {
		t.Helper()
		if strings.Contains(cut, "](") || strings.Count(cut, "[") != strings.Count(cut, "]") {
			t.Fatalf("a partial link survived: %q", cut)
		}
	}

	t.Run("cut pulls back to before the link, keeping the lead", func(t *testing.T) {
		line2 := "Read [the annual report](https://example.com/reports/2026/annual.pdf) today"
		body := "# Title\n" + line2
		cut, truncated := truncateCharsLine(body, 30) // budget lands inside the link
		if !truncated {
			t.Fatal("expected a cut")
		}
		if cut != "# Title\nRead" {
			t.Fatalf("expected the lead before the link to survive; got %q", cut)
		}
		assertNoPartialLink(t, cut)
	})

	// A long opening paragraph with an inline link across the limit must still
	// return the prose before the link, not an empty body.
	t.Run("long first line with a link across the limit", func(t *testing.T) {
		lead := strings.Repeat("word ", 38) // 190 chars, trailing space
		body := lead + "[annual report](https://example.com/x/y/z) and more padding text here"
		cut, truncated := truncateCharsLine(body, 200) // budget lands inside the link
		if !truncated {
			t.Fatal("expected a cut")
		}
		if cut == "" {
			t.Fatal("a long first paragraph with a link must return the prose before it, not empty")
		}
		if strings.Contains(cut, "[") {
			t.Fatalf("the cut must end before the link's '['; got %q", cut)
		}
		if !strings.HasPrefix(body, cut) {
			t.Fatalf("the partial line must be a prefix of the source; got %q", cut)
		}
		if n := utf8.RuneCountInString(cut); n > 200 {
			t.Fatalf("cut kept %d chars, over the 200 limit", n)
		}
		assertNoPartialLink(t, cut)
	})
}

func TestTruncateCharsLine_NoCutWhenUnderLimit(t *testing.T) {
	body := "one\ntwo\nthree"
	got, truncated := truncateCharsLine(body, 100)
	if truncated || got != body {
		t.Fatalf("under-limit body was altered: got=%q truncated=%v", got, truncated)
	}
}

func TestWriteTextResponse_MarkdownPlainFormatIsTextMarkdown(t *testing.T) {
	extraction := textExtraction{Text: "# Heading\n\nBody text.", Mode: extractionMarkdown}
	w := textResponseRecorder(t, extraction, -1, "text")

	if got := w.Header().Get("Content-Type"); got != "text/markdown; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/markdown", got)
	}
	if w.Body.String() != "# Heading\n\nBody text." {
		t.Errorf("body = %q, want the bare Markdown", w.Body.String())
	}
	if got := w.Header().Get(headerTextExtraction); got != extractionMarkdown {
		t.Errorf("%s = %q, want %q", headerTextExtraction, got, extractionMarkdown)
	}
}

func TestWriteTextResponse_MarkdownEnvelopeCarriesTitleAndDescription(t *testing.T) {
	extraction := textExtraction{
		Text:        "## Section\n\n| a | b |\n|---|---|\n| 1 | 2 |",
		Mode:        extractionMarkdown,
		Title:       "Converter Title",
		Description: "Converter description.",
	}
	w := textResponseRecorder(t, extraction, -1, "")
	envelope := decodeTextEnvelope(t, w)

	if envelope["extraction"] != extractionMarkdown {
		t.Errorf("extraction = %v, want %q", envelope["extraction"], extractionMarkdown)
	}
	if envelope["title"] != "Converter Title" {
		t.Errorf("title = %v, want the converter title", envelope["title"])
	}
	if envelope["description"] != "Converter description." {
		t.Errorf("description = %v, want the converter description", envelope["description"])
	}
	if _, ok := envelope["rawLength"]; ok {
		t.Errorf("rawLength present for a markdown response with no raw comparison")
	}
}

// maxChars over a Markdown table: the JSON textLength counts the truncated body
// and the last line stays a whole table row.
func TestWriteTextResponse_MarkdownMaxCharsCutsOnLineBoundary(t *testing.T) {
	body := "| Col A | Col B |\n|-------|-------|\n| a1 | b1 |\n| a2 | b2 |\n| a3 | b3 |"
	extraction := textExtraction{Text: body, Mode: extractionMarkdown}
	w := textResponseRecorder(t, extraction, 40, "")
	envelope := decodeTextEnvelope(t, w)

	if envelope["truncated"] != true {
		t.Fatalf("truncated = %v, want true", envelope["truncated"])
	}
	text, _ := envelope["text"].(string)
	for _, line := range strings.Split(text, "\n") {
		if !strings.Contains(body, line) {
			t.Fatalf("returned line %q is not a whole source line", line)
		}
	}
	if envelope["textLength"] != float64(utf8.RuneCountInString(text)) {
		t.Errorf("textLength = %v, want the truncated body length %d", envelope["textLength"], utf8.RuneCountInString(text))
	}
}

// The guard runs on markdown exactly as on readability: reuse the injected
// corpus the /text and /html guard tests share and assert the same warn-and-wrap.
func TestWriteTextResponse_MarkdownReusesTheTextGuard(t *testing.T) {
	h := inspectIDPIHandlers(false)
	h.Bridge = &mockBridge{}

	for _, mode := range []string{extractionReadability, extractionMarkdown} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/text", nil)
		h.writeTextResponse(w, r, context.Background(), textExtraction{Text: injectedMarkup, Mode: mode}, -1, "", nil, nil)

		if w.Code != http.StatusOK {
			t.Fatalf("mode %q: status = %d, want 200 (warn, not block) body=%s", mode, w.Code, w.Body.String())
		}
		envelope := decodeTextEnvelope(t, w)
		if envelope["idpiWarning"] == nil || envelope["idpiWarning"] == "" {
			t.Errorf("mode %q: idpiWarning missing; the guard did not run on the body", mode)
		}
		text, _ := envelope["text"].(string)
		if !strings.Contains(text, "untrusted_web_content") {
			t.Errorf("mode %q: body is not wrapped as untrusted content:\n%s", mode, text)
		}
	}
}

func textFrameResponse(t *testing.T, f frameFixture, query string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/text?tabId=tab-frame&"+query, nil)
	w := httptest.NewRecorder()
	f.handlers.HandleText(w, req)
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return w.Code, body
}

func TestHandleText_SelectorReadsInsideTheScopedFrame(t *testing.T) {
	f := newFrameFixture(t)
	tree, err := page.GetFrameTree().Do(cdp.WithExecutor(f.ctx, chromedp.FromContext(f.ctx).Target))
	if err != nil {
		t.Fatal(err)
	}
	if len(tree.ChildFrames) == 0 {
		t.Fatal("fixture has no child frame")
	}

	code, body := textFrameResponse(t, f, "selector=button")
	if code != http.StatusOK || body["text"] != "Alert" {
		t.Fatalf("top-level read = %d %v, want 200 Alert", code, body)
	}

	f.handlers.frameScopes().SetFrameScope("tab-frame", bridge.FrameScope{FrameID: string(tree.ChildFrames[0].Frame.ID)})
	code, body = textFrameResponse(t, f, "selector=button")
	if code != http.StatusOK || body["text"] != "frame button" {
		t.Fatalf("frame-scoped read = %d %v, want 200 %q", code, body, "frame button")
	}
}

func TestHandleText_EmptyElementIsAMatchNotA404(t *testing.T) {
	f := newFrameFixture(t)

	code, body := textFrameResponse(t, f, "selector=%23f")
	if code != http.StatusOK {
		t.Fatalf("status = %d %v, want 200", code, body)
	}
	if text, ok := body["text"].(string); !ok || text != "" {
		t.Fatalf("text = %#v, want empty string", body["text"])
	}
}

func TestHandleText_UnmatchedSelectorIs404(t *testing.T) {
	f := newFrameFixture(t)

	if code, body := textFrameResponse(t, f, "selector=%23missing"); code != http.StatusNotFound {
		t.Fatalf("status = %d %v, want 404", code, body)
	}
}
