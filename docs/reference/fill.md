# Fill

Set an input value directly without relying on the same event sequence as `type`.

```bash
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","ref":"e8","text":"ada@pinchtab.com"}'
# CLI Alternative
pinchtab fill e8 "ada@pinchtab.com"
# Response
{
  "success": true,
  "result": {
    "success": true
  }
}
```

Notes:

- the top-level CLI accepts unified selector forms such as `e8`, `#email`, or `text:Email`
- refs returned for iframe descendants can be filled directly; no manual frame switch is required
- selector lookup is limited to the current frame scope; the default scope is `main`
- use [`/frame`](./frame.md) or `pinchtab frame` before selector-based iframe fills
- missing selectors now fail immediately with `element not found: ...`; use
  [`pinchtab wait`](./wait.md) or `/wait` first when the field is expected to
  appear asynchronously
- for the raw HTTP action endpoint, use `selector` for CSS, XPath, text, or semantic selectors

## Related Pages

- [Frame](./frame.md)
- [Type](./type.md)
- [Snapshot](./snapshot.md)
