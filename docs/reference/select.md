# Select

Choose a value in a select element by selector or ref.

```bash
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"select","ref":"e12","value":"it"}'
# CLI Alternative
pinchtab select e12 it
# Response
{
  "success": true,
  "result": {
    "success": true
  }
}
```

The `value` should match the option value expected by the page. The raw action endpoint accepts `ref` or `selector`, and the CLI accepts the same unified selector forms as the other action commands.

## Related Pages

- [Snapshot](./snapshot.md)
- [Focus](./focus.md)
