# Hover

Move the pointer over an element by selector or ref.

```bash
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"hover","ref":"e5"}'
# CLI Alternative
pinchtab hover e5
# Response
{
  "success": true,
  "result": {
    "success": true
  }
}
```

Use this when menus or tooltips appear only after hover.

The raw action endpoint accepts either `ref` or `selector`. The CLI accepts unified selector forms such as `e5`, `#menu`, `xpath://button`, `text:Menu`, and `find:account menu`.

Selector lookup is limited to the current frame scope. The default scope is `main`; use [`/frame`](./frame.md) or `pinchtab frame` before selector-based iframe hover calls.

## Related Pages

- [Click](./click.md)
- [Frame](./frame.md)
- [Snapshot](./snapshot.md)
