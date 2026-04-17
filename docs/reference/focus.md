# Focus

Move focus to an element by selector or ref.

```bash
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"focus","ref":"e8"}'
# CLI Alternative
pinchtab focus e8
# Response
{
  "success": true,
  "result": {
    "success": true
  }
}
```

This is useful before keyboard-only flows such as `press Enter` or `type`.

The raw action endpoint accepts either `ref` or `selector`. The CLI accepts the same unified selector forms as other top-level action commands.

Selector lookup is limited to the current frame scope. The default scope is `main`; use [`/frame`](./frame.md) or `pinchtab frame` before selector-based iframe focus calls.

## Related Pages

- [Frame](./frame.md)
- [Press](./press.md)
- [Type](./type.md)
