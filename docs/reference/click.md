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
- the raw action endpoint also accepts `selector`, for example `{"kind":"click","selector":"#login"}`
- the CLI also accepts `#login`, `xpath://button`, `text:Submit`, and `find:login button`
- `--wait-nav` exists on the top-level CLI command

## Related Pages

- [Snapshot](./snapshot.md)
- [Navigate](./navigate.md)
