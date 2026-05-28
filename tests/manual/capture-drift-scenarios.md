# /capture drift scenarios — operator smoke

Manual checks for `pinchtab capture`. Run against a local daemon
(`pinchtab serve` in another terminal) and a fixture server. The fixtures
live in `tests/tools/fixtures/`; serve them however you usually do (e.g.
`python3 -m http.server 9000 -d tests/tools/fixtures`).

These mirror §9 of the design doc. Real-Chrome Go tests are out of scope —
the repo's pattern for integration is agent-driven via browserbench plus
operator smoke. When `pinchtab_capture` is exercised through browserbench
the same fixtures apply.

## 1. Happy path

```bash
pinchtab nav http://localhost:9000/capture-known-bounds.html
pinchtab capture --with-bounds -o /tmp/cap-known.jpg
```

Expect:

- file `/tmp/cap-known.jpg` exists.
- response prints `epoch: ep_...`, `pairing: navigated=false`.
- the snapshot's button "Known Position Button" has a `boundingBox` near
  `{x: 100, y: 200, w: 240, h: 48}` (within 1 CSS pixel × `devicePixelRatio`).
- `image.coordinateSpace == "viewport"`.

## 2. wait:stable for deferred mount

```bash
pinchtab nav 'http://localhost:9000/capture-deferred-mount.html?mountAfter=200'
pinchtab capture --wait stable
```

Expect the snapshot to include the `button "Pay now"` ref. Compare:

```bash
pinchtab nav 'http://localhost:9000/capture-deferred-mount.html?mountAfter=200'
pinchtab capture --wait none
```

With `wait=none` the button may be missing — the capture window opened
before the deferred mount fired. With `wait=stable` the lifecycle quiet
window covers the mount.

## 3. Navigation race + requirePair

In one terminal:

```bash
pinchtab nav http://localhost:9000/capture-known-bounds.html
while true; do pinchtab capture --require-pair -o /tmp/cap-race.jpg; sleep 0.5; done
```

In another, hammer navigation:

```bash
while true; do
  pinchtab nav http://localhost:9000/capture-modal-race.html
  pinchtab nav http://localhost:9000/capture-known-bounds.html
  sleep 0.3
done
```

Expect some `pinchtab capture` runs to fail with HTTP 409
(`pairing broken: navigation observed during capture window`). With
`--require-pair` removed, the same races surface as `pairing.navigated:
true` in the response instead of failing.

## 4. Beyond-viewport coordinate space

```bash
pinchtab nav http://localhost:9000/capture-tall.html
pinchtab capture --beyond-viewport -o /tmp/cap-tall.jpg
```

Expect:

- `/tmp/cap-tall.jpg` is the full document (much taller than the viewport).
- response prints `coordinateSpace: document`.
- the `cta-below-fold` ref's `boundingBox.y` is around 4800 (page coords),
  not 4800 minus the viewport height.

## 5. Selector scope parity

```bash
pinchtab nav http://localhost:9000/capture-modal-race.html
sleep 1   # let the modal mount
pinchtab capture -s '#consent' -o /tmp/cap-modal.jpg
```

Expect:

- the image is clipped to the modal card (`#consent`).
- snapshot nodes are all descendants of `#consent` (consent-title,
  consent-accept, consent-decline).
- url and title in the response still reflect the underlying page.

## 6. Performance budget (rough)

```bash
pinchtab nav http://localhost:9000/capture-known-bounds.html
for i in 1 2 3 4 5; do
  pinchtab capture --with-bounds=false -o /tmp/cap-$i.jpg | grep duration
done
```

Expect `captureDurationMs` < 600ms on a baseline workstation. With
`--with-bounds=true` and an `interactive`-filter snapshot of ~20 nodes,
add ~100ms. Hard regressions (>1.5s without obvious cause) deserve
investigation.

## 7. Skill-layer dispatch

Invoke `pinchtab_capture` through an MCP client. Confirm:

- the result has both an image content block (rendered by the client)
  and a text content block (JSON envelope with `snapshot.nodes`).
- the `boundingBox` on a known node lines up with where that node sits
  in the rendered image.

This is the most useful end-to-end check — it's the path real users hit.
