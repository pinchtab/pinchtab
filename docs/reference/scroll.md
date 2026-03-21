# Scroll

Scroll the current tab or a specific element.

```bash
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"scroll","scrollY":800}'
# CLI Alternative
pinchtab scroll down
# Response
{
  "success": true,
  "result": {
    "success": true
  }
}
```

Notes:

- the top-level CLI also accepts a pixel value such as `pinchtab scroll 800`
- the raw API uses `scrollY` and `scrollX` for page scrolling
- the raw API can also target an element with `ref` or `selector`

## Related Pages

- [Snapshot](./snapshot.md)
- [Text](./text.md)
