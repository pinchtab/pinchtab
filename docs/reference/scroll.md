# Scroll

Scroll the current tab or a specific element.

```bash
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"scroll","scrollY":800}'
# Response: {"success":true,"result":{"scrolled":true,...},"route":{...}}
# result also carries scrollX/scrollY (where the scroller ended up), deltaX/deltaY (the scroll
# requested) and targetX/targetY (the point scrolled at); an element scroll returns only scrolled:true

# CLI Alternative (human-readable by default)
pinchtab scroll down
# Output: OK

pinchtab scroll down --snap        # scroll and output snapshot
pinchtab scroll 800 --snap-diff    # scroll and output snapshot diff
pinchtab scroll 800 --json         # Full JSON response

pinchtab scroll -300               # scroll up 300px
pinchtab scroll --dy -300          # the same, as a flag
pinchtab scroll --dx -120          # scroll left 120px
```

Notes:

- a negative pixel count works either way — `pinchtab scroll -300` and `pinchtab scroll --dy -300`
  are the same scroll, and `--tab` may sit in any position in both
- give either the flags or one positional, never both, and only one positional is accepted —
  so `--tab` stays a flag and must not be placed after `--`, where it would be read as a
  positional
- a delta of zero on both axes is refused, by the CLI before sending and by the server
  (an explicit `scrollX`/`scrollY` of zero on both axes is an error). An explicit zero on one
  axis (`--dy 0 --dx 500`) is a real scroll and passes through. Only a request with no delta
  and no target at all scrolls down by the default 120px
- a direction keyword (`up`, `down`, `left`, `right`) scrolls 800px per step
- the scroll moves the nearest scrollable ancestor of the element at `targetX`/`targetY` — an
  inner list or panel under that point — or the page when there is none. `scrollX`/`scrollY` in
  the result are that scroller's position afterwards, clamped to its extent, not the delta asked for
- `scrolled` is `true` only when that position changed, so a scroll at the end of the page or list
  answers `scrolled:false` — the stop condition for a "scroll until nothing new loads" loop

- use `--snap` to output an interactive snapshot after scrolling
- use `--snap-diff` to output only the changes from the previous snapshot
- the top-level CLI also accepts a pixel value such as `pinchtab scroll 800`
- the raw API uses `scrollY` and `scrollX` for page scrolling
- the raw API can also target an element with `ref` or `selector`
- selector lookup is limited to the current frame scope; the default scope is `main`
- use [`/frame`](./frame.md) or `pinchtab frame` before selector-based iframe scrolling

## Related Pages

- [Frame](./frame.md)
- [Snapshot](./snapshot.md)
- [Text](./text.md)
