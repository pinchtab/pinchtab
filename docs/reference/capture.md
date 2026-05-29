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

When `withBounds=true` (the default), each snapshot node with a
non-zero backend node id gets a `boundingBox` and a `visible` flag. The
coordinate space depends on `selector` and `beyondViewport`:

- **`viewport`** (default): boxes are viewport-relative CSS pixels. The
  image is the visible viewport. `image.devicePixelRatio` tells you the
  ratio of image pixels to CSS pixels.
- **`clip`** (when `selector` is set): boxes are relative to the cropped
  image origin. The response also includes `image.clip` with the original
  document-relative clip rectangle.
- **`document`** (when `beyondViewport=true`): boxes use page coordinates
  (`box.x` and `box.y` include scroll offset). The image is the full
  document.

`visible` is true when the box has positive area and intersects the
viewport — a cheap heuristic, not a strict occlusion check.

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
| `withBounds` | `true` (default) — populate `boundingBox` + `visible` on every snapshot node |
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
