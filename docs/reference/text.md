# Text

Extract text from the current page.

By default, PinchTab runs a Readability-style extraction against the current
document. Use full/raw mode when you want `document.body.innerText` instead.

`/text` is frame-aware:

- `frameId=<id>` targets a specific iframe for a one-shot read
- otherwise, `/text` inherits the tab's current frame scope from [`/frame`](./frame.md)
- if no frame is selected, `/text` reads from the top-level document

```bash
curl "http://localhost:9867/text?mode=raw"
# CLI Alternative
pinchtab text --raw
# Response
{
  "url": "https://example.com",
  "title": "Example Domain",
  "text": "Example Domain\nThis domain is for use in illustrative examples in documents.",
  "truncated": false
}
```

Useful flags:

- CLI: `--full`, `--raw`, `--frame <frameId>`
- API query: `mode=raw`, `maxChars`, `format=text`, `frameId=<frameId>`

Examples:

```bash
# Default Readability extraction
pinchtab text

# Full page text (same behavior as --raw)
pinchtab text --full

# One-shot iframe read by frame id
pinchtab text --frame FRAME123

# HTTP form of the same idea
curl "http://localhost:9867/text?frameId=FRAME123&format=text"
```

Use default mode for article-like pages. Use `--full` / `mode=raw` for UI-heavy
pages such as dashboards, SERPs, grids, pricing tables, or short log panes that
Readability may trim away.

## Related Pages

- [Snapshot](./snapshot.md)
- [Frame](./frame.md)
- [PDF](./pdf.md)
