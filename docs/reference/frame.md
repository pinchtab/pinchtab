# Frame

Get or set the current frame scope for selector-based snapshots and actions.

By default, selector lookup stays in the main document. To target iframe content with CSS, XPath, or text selectors, set the frame first.

Refs from `/snapshot` are different: if a snapshot includes same-origin iframe descendants, those refs can still be used directly without setting frame scope.

```bash
curl http://localhost:9867/frame

curl -X POST http://localhost:9867/frame \
  -H "Content-Type: application/json" \
  -d '{"target":"#payment-frame"}'

curl -X POST http://localhost:9867/frame \
  -H "Content-Type: application/json" \
  -d '{"target":"main"}'

# CLI Alternative
pinchtab frame
pinchtab frame "#payment-frame"
pinchtab frame main
```

Targets accepted by `POST /frame` and `pinchtab frame`:

- `main` to clear frame scope
- a snapshot ref for an iframe owner
- a selector for an iframe element
- a frame name or frame URL

Typical iframe flow:

```bash
pinchtab snap -i
pinchtab frame "#payment-frame"
pinchtab snap -i
pinchtab fill "#card-number" "4111111111111111"
pinchtab click "#pay-button"
pinchtab frame main
```

Notes:

- selector scope is explicit; unscoped selectors do not automatically pierce into iframes
- same-origin iframe content is supported; cross-origin iframe descendants are not currently exposed as frame scopes
- nested iframes usually require multiple `frame` hops
- the same frame scope applies to selector-based `/snapshot` and `/action` calls, and also to `/text` when `frameId` is not provided explicitly
- `/evaluate` is separate and does not inherit frame scope

## Related Pages

- [Snapshot](./snapshot.md)
- [Click](./click.md)
- [Fill](./fill.md)
