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

## Related Pages

- [Fill](./fill.md)
- [Snapshot](./snapshot.md)
