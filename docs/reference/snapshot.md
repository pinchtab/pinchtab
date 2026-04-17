# Snapshot

Get an accessibility snapshot of the current page, including element refs that can be reused by action commands.

Iframe content is detected automatically during snapshot capture. Same-origin iframe descendants are included beneath the iframe owner element, and their refs can be reused directly with action commands. Cross-origin iframes currently remain as owner nodes only.

Selector scoping is explicit. `selector=...` only searches the current frame scope, which defaults to `main`. To scope selector-based snapshots into an iframe, set the frame first with [`/frame`](./frame.md) or `pinchtab frame`.

```bash
curl "http://localhost:9867/snapshot?filter=interactive"
# CLI Alternative
pinchtab snap -i
# Response
{
  "url": "https://pinchtab.com",
  "title": "Example Domain",
  "nodes": [
    {
      "ref": "e5",
      "role": "link",
      "name": "More information..."
    }
  ],
  "count": 1
}
```

Useful flags:

- CLI: `-i`, `-c`, `-d`, `--selector`, `--max-tokens`, `--depth`
- API query: `filter`, `format`, `diff`, `selector`, `maxTokens`, `depth`

## Related Pages

- [Click](./click.md)
- [Frame](./frame.md)
- [Tabs](./tabs.md)
