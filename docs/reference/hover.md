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

## Related Pages

- [Click](./click.md)
- [Snapshot](./snapshot.md)
