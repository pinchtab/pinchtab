# Capture

Paired screenshot and accessibility snapshot from the **same DOM epoch** in
one HTTP call. Use this when the model needs to read pixels AND act on refs
in the same turn — the unpaired `/screenshot` + `/snapshot` sequence drifts
when the page mutates between calls.

```bash
# Default: file output, wait for page quiescence, bounds included
curl "http://localhost:9867/capture"

# CLI alternative — writes the image locally and prints a summary
pinchtab capture -o /tmp/cap.jpg

# Half-size image (snapshot/bounds unchanged)
pinchtab capture --scale 0.5

# Fail with 409 if the main frame navigates mid-capture
pinchtab capture --require-pair

# Full-document image; bounding boxes in page coords
pinchtab capture --beyond-viewport

# Scope to one element: image clips to it, snapshot subtree filters to it.
# Bounding boxes are relative to the clipped image origin.
pinchtab capture -s "#checkout-form"
```

## Response (JSON)

```json
{
  "status": "ok",
  "tabId": "tab_abc",
  "url": "https://example.com/checkout",
  "title": "Checkout",
  "capturedAt": "2026-05-29T15:44:12.431Z",
  "epoch": {
    "frameId": "8E2F...A1",
    "loaderId": "5C9D...0B",
    "domEpoch": "ep_..."
  },
  "pairing": {
    "navigated": false,
    "captureDurationMs": 312
  },
  "image": {
    "format": "jpeg",
    "path": "/.../state/captures/cap-20260529-154412.jpg",
    "bytes": 184223,
    "coordinateSpace": "viewport",
    "devicePixelRatio": 2,
    "viewport": { "w": 1440, "h": 900, "scrollX": 0, "scrollY": 0 }
  },
  "snapshot": {
    "filter": "interactive",
    "nodeCount": 14,
    "nodes": [
      {
        "ref": "e4", "role": "textbox", "name": "Email",
        "boundingBox": { "x": 520, "y": 312, "w": 280, "h": 36 },
        "visible": true
      },
      {
        "ref": "e9", "role": "button", "name": "Submit",
        "boundingBox": { "x": 520, "y": 1480, "w": 96, "h": 40 },
        "visible": false
      }
    ]
  }
}
```

## What pairing guarantees

The atomicity contract is **"no main-frame navigation between the two CDP
calls"** — `pairing.navigated` flips to `true` when the main frame's
`loaderId` changes during the capture window. Drift inside the same
document (React re-renders, `IntersectionObserver` mutations) is not
detected; `wait=stable` reduces but does not eliminate it.

`epoch.domEpoch` is an opaque server-minted token cached on the tab's
ref-cache alongside the snapshot refs. Future action endpoints will accept
an `expectedEpoch` query param to reject stale refs at use time.

## Bounding boxes and coordinate space

When `withBounds=true` (the default), each snapshot node with a non-zero
backend node id gets a `boundingBox` and a `visible` flag, and a node that
cannot be measured gets neither.

`boundingBox` is the **border box** — the painted edge of the element, the
same rectangle `pinchtab box`, `screenshot?annotate=true` and
`getBoundingClientRect` report. Overlays and crop rectangles drawn from it
cover the control a viewer sees, and a box from `/capture` can be compared
directly with one from any of those surfaces.

The coordinate space depends on `selector` and `beyondViewport`:

- **`viewport`** (default): boxes are viewport-relative CSS pixels. The
  image is the visible viewport. `image.devicePixelRatio` tells you the
  ratio of image pixels to CSS pixels.
- **`clip`** (when `selector` is set): boxes are relative to the cropped
  image origin. The response also includes `image.clip` with the original
  document-relative clip rectangle.
- **`document`** (when `beyondViewport=true`): boxes use page coordinates
  (`box.x` and `box.y` include scroll offset). The image is the full
  document.

In all three, the image measures exactly the reported space times
`image.devicePixelRatio`: scaling a `boundingBox` by that ratio, from the
origin the space names, lands on the image's pixels. This holds under
`pinchtab set viewport` as well, so an overlay never needs a mode-specific
factor. Keeping it costs the default capture the faster read-the-view path —
that path returns the real window surface, whose scale factor is the screen's
rather than the page's and which viewport emulation does not touch — so an
idle headed browser can make a `/capture` slower than a `/screenshot`.

`visible` is true when the box has positive area and intersects the
viewport — a cheap heuristic, not a strict occlusion check. A node
scrolled past, in either direction, is measured and reports
`"visible": false`.

**Absent means not measured, never "no".** `visible` appears exactly when
`boundingBox` does, so the key is missing only where no measurement was
taken: `withBounds=false`, a node with no backend node id, or a node whose
box query failed. Treat a missing `visible` as unknown and a present
`false` as off-screen; they are different answers.

### `visible` here is not `GET /visible`

`GET /visible` (and `pinchtab visible <ref>`) answers a different question
under the same word: **CSS rendered-ness** — `display`, `visibility`,
`opacity`, and a laid-out or positioned box with non-zero size. **Scroll
position is not an input**, so an element far below the fold is `"visible":
true` there while this snapshot reports `"visible": false` for the same node
at the same moment. Both answers are correct and both are wanted: one says
the element is rendered at all, the other says it is on screen right now.

`GET /visible` also returns `onScreen`, which is this snapshot's predicate
computed for that one element, so a single call answers both questions.
`onScreen` follows the same absent-means-not-measured rule as `visible`
above.

## Useful flags

### API Query Parameters

| Parameter | Description |
|-----------|-------------|
| `tabId` | Target a specific tab |
| `selector` | Scope: clips image and filters snapshot subtree to the same element |
| `filter` | `interactive` (default) or `all` |
| `format` | `jpeg` (default) or `png` |
| `quality` | JPEG quality 0-100 |
| `depth` | Snapshot tree depth limit |
| `output` | `file` (default), `inline` (base64 in JSON), or `raw` (bytes only — drops the snapshot) |
| `wait` | `stable` (default) waits for `Page.lifecycleEvent` quiescence (250ms silence / 750ms ceiling); `load` polls `document.readyState` until `complete` (2s ceiling); `none` skips the wait |
| `withBounds` | `true` (default) — populate `boundingBox` (the border box) + `visible` on every measurable snapshot node; `false` omits both keys everywhere |
| `beyondViewport` | `true` — capture the full scrollable document; coordinate space becomes `document` |
| `scale` | Rescale the output bitmap. Default `1`. `0.5` halves each axis (quarter the pixels) |
| `requirePair` | `true` returns 409 if `pairing.navigated` would be true |
| `noAnimations` | `true` — inject `prefers-reduced-motion` CSS for the capture window |

### CLI

| Flag | Description |
|------|-------------|
| `-o <path>` | Save the captured image locally (default: `capture-<ts>.jpg`) |
| `-s <selector>` | Scope: clips image and filters snapshot subtree |
| `--filter <name>` | Snapshot filter |
| `--format <fmt>` | `jpeg` or `png` |
| `-q <0-100>` | JPEG quality |
| `--depth <n>` | Snapshot depth limit |
| `--wait <mode>` | `stable` (default) / `load` / `none` |
| `--with-bounds` | Boolean (default true) |
| `--beyond-viewport` | Capture full document |
| `--scale <f>` | Bitmap rescale (e.g. `0.5`) |
| `--require-pair` | Fail with 409 on mid-capture navigation |
| `--tab <id>` | Target a specific tab |

## Related Pages

- [Screenshot](./screenshot.md)
- [Snapshot](./snapshot.md)
- [Frame](./frame.md)
