# Type

Type text into an element, sending key events as the text is entered.

```bash
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"type","ref":"e8","text":"Ada Lovelace"}'
# CLI Alternative
pinchtab type e8 "Ada Lovelace"
# Response
{
  "success": true,
  "result": {
    "success": true
  }
}
```

Notes:

- use `fill` when you want to set the value more directly
- the top-level CLI accepts unified selector forms such as `e8`, `#name`, `xpath://input`, or `text:Name`
- selector lookup is limited to the current frame scope; the default scope is `main`
- use [`/frame`](./frame.md) or `pinchtab frame` before selector-based iframe typing
- missing selectors now fail immediately with `element not found: ...`; use
  [`pinchtab wait`](./wait.md) or `/wait` first when the field is expected to
  appear asynchronously
- for typing without a target element (into whatever is focused), use `keyboard type`

## Related Pages

- [Frame](./frame.md)
- [Fill](./fill.md) — Set input value directly
- [Keyboard](./keyboard.md) — Low-level keyboard input (type at focused element)
- [Snapshot](./snapshot.md)
