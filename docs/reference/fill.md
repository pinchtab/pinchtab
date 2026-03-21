# Fill

Set an input value directly without relying on the same event sequence as `type`.

```bash
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"fill","ref":"e8","text":"ada@example.com"}'
# CLI Alternative
pinchtab fill e8 "ada@example.com"
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
- for the raw HTTP action endpoint, use `selector` for CSS, XPath, text, or semantic selectors

## Related Pages

- [Type](./type.md)
- [Snapshot](./snapshot.md)
