# Click

Click an element using a snapshot ref, CSS selector, XPath selector, text selector, or semantic selector.

```bash
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","ref":"e5"}'
# CLI Alternative
pinchtab click e5
# Response
{
  "success": true,
  "result": {
    "success": true
  }
}
```

Notes:

- element refs come from `/snapshot`
- refs returned for iframe descendants can be clicked directly; no manual frame switch is required
- selector lookup is limited to the current frame scope; the default scope is `main`
- use [`/frame`](./frame.md) or `pinchtab frame` before selector-based iframe actions
- missing selectors now fail immediately with `element not found: ...`; if you
  want wait semantics for dynamic UI, use [`pinchtab wait`](./wait.md) or
  `/wait` before the click
- the raw action endpoint also accepts `selector`, for example `{"kind":"click","selector":"#login"}`
- the CLI also accepts `#login`, `xpath://button`, `text:Submit`, and `find:login button`
- `--wait-nav` exists on the top-level CLI command

## Related Pages

- [Frame](./frame.md)
- [Snapshot](./snapshot.md)
- [Navigate](./navigate.md)
