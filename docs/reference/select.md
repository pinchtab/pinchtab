# Select

Choose an option in a native `<select>` element by selector or ref.

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

Matching is forgiving. PinchTab tries these strategies in order:

1. exact `<option value="...">`
2. exact visible text
3. case-insensitive visible text
4. case-insensitive substring of visible text

That means all of these can work depending on the page:

```bash
pinchtab select e12 uk
pinchtab select e12 "United Kingdom"
pinchtab select e12 "united kingdom"
pinchtab select e12 "Kingdom"
```

Prefer the canonical option value or the full visible text when disambiguation
matters. The raw action endpoint accepts `ref` or `selector`, and the CLI
accepts the same unified selector forms as the other action commands.

Selector lookup is limited to the current frame scope. The default scope is `main`; use [`/frame`](./frame.md) or `pinchtab frame` before selector-based iframe selects.

## Related Pages

- [Frame](./frame.md)
- [Snapshot](./snapshot.md)
- [Focus](./focus.md)
