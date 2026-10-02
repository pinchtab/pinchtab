# Text

Extract text from the current page or a specific element.

By default, PinchTab runs a Readability-style extraction against the current
document. Use full/raw mode when you want `document.body.innerText` instead.
The default extraction leaves out content the page hides with CSS
(`display: none` or `visibility: hidden`), such as inactive tab panels, filtered
rows, and hidden error templates.

## Element Selection

Extract text from a specific element using a selector:

```bash
# Positional selector argument
pinchtab text "#article-body"
pinchtab text "text:Welcome"

# Or use --selector flag
pinchtab text --selector "#article-body"
pinchtab text -s "xpath://div[@class='content']"
```

Supported selector types: ref (`e5`), CSS (`#id`), XPath (`xpath://...`), text (`text:...`).

## Frame Scope

`/text` is frame-aware:

- `--frame <id>` or `frameId=<id>` targets a specific iframe for a one-shot read
- otherwise, `/text` inherits the tab's current frame scope from [`/frame`](./frame.md)
- if no frame is selected, `/text` reads from the top-level document

## Output Format

Default output is human-readable text. Use `--json` for structured output:

```bash
pinchtab text                           # Plain text output
pinchtab text --json                    # JSON envelope: url, title, text, truncated, extraction, textLength, rawLength, ...
```

An element read (`selector`/`ref`) returns just `{url, title, text}` — the element's
`innerText` — and ignores `mode`.

When a modal dialog is open in the page, a whole-page read returns the topmost dialog's
text (reported as `raw`) and element selectors resolve inside it. If the topmost dialog
changes twice during the read the request fails with `409`; a pending JavaScript dialog
(alert, confirm, prompt) blocks it with `409 dialog_blocked`.

## Extraction Mode

Readability is an article heuristic, so on a landing page, dashboard, or docs
index it can return one block instead of the page. `/text` measures its output
against `document.body.innerText` and, when the coverage is too low, returns the
raw text instead. The JSON envelope always says which extractor produced the
text:

| Field | Description |
|-------|-------------|
| `extraction` | `readability`, `raw` (explicitly requested), `markdown` (page converted to Markdown), `markdown_fallback` (converter yielded nothing, raw text returned), or `readability_fallback` (Readability collapsed, raw text returned) |
| `textLength` | Length of the returned text |
| `rawLength` | Length of `document.body.innerText`, so coverage is computable |

With `format=text` the body stays bare and the mode is reported in the
`X-PT-Text-Extraction` response header (`text/markdown` content type for the
Markdown modes). `mode=raw` never runs the comparison.
`truncated` keeps its meaning — text cut by `maxChars` — and is unaffected by a
fallback. The CLI prints a one-line note on stderr when a fallback fired; stdout
stays text-only.

`--markdown` (`mode=markdown`) converts the rendered page to Markdown, preserving
headings, inline links, and tables — the best mode for article-shaped pages you
want to keep or re-read. It is mutually exclusive with `--full`/`--raw`. Pair it
with `--output <file>` (`-o`) to write the Markdown to disk; the command then
prints only a one-line confirmation so a long page never floods an agent's
context.

## Examples

```bash
# Default Readability extraction
pinchtab text

# Full page text (document.body.innerText)
pinchtab text --full
pinchtab text --raw                     # Alias of --full

# Markdown (preserves headings, links, tables)
pinchtab text --markdown
pinchtab text --markdown --output page.md   # writes the file, prints a one-line confirmation

# Extract text from specific element
pinchtab text "#main-content"
pinchtab text --selector ".article-body"

# One-shot iframe read by frame id
pinchtab text --frame FRAME123

# API equivalent
curl "http://localhost:9867/text?mode=raw"
curl "http://localhost:9867/text?mode=markdown"
curl "http://localhost:9867/text?selector=%23article-body"
curl "http://localhost:9867/text?frameId=FRAME123&format=text"
```

## Flags

| Flag | Description |
|------|-------------|
| `--selector`, `-s` | Element selector (ref/CSS/XPath/text) |
| `--frame` | Extract from specific iframe by frameId |
| `--full` | Full page innerText instead of Readability |
| `--raw` | Alias for --full |
| `--markdown` | Convert the page to Markdown (headings, links, tables); mutually exclusive with `--full`/`--raw` |
| `--output`, `-o` | Write the extracted text to this file and print a one-line confirmation |
| `--json` | Output JSON instead of plain text |
| `--tab` | Target specific tab |

## API Parameters

| Parameter | Description |
|-----------|-------------|
| `selector` | Element selector for text extraction |
| `ref` | Snapshot ref (e.g., `e5`) |
| `frameId` | Target iframe ID |
| `tabId` | Target tab (defaults to the current tab) |
| `mode` | `raw` or its alias `full` for innerText, `markdown` for Markdown, omitted for Readability; any other value is a 400 |
| `maxChars` | Truncate output to this many characters (positive integer; Markdown is cut on line boundaries) |
| `format` | `text` or `plain` for a bare text response; default is the JSON envelope |

`GET /tabs/{id}/text` is the same handler with the tab in the path.

Use default mode for article-like pages. Use `--full` / `mode=raw` for UI-heavy
pages such as dashboards, SERPs, grids, pricing tables, or short log panes that
Readability may trim away.

## Related Pages

- [Snapshot](./snapshot.md)
- [Frame](./frame.md)
- [PDF](./pdf.md)
