# Screenshot

Capture the current page as an image. Defaults to **JPEG** format.

```bash
# Get raw PNG bytes
curl "http://localhost:9867/screenshot?format=png&raw=true" > page.png

# Capture a specific element (selector supports ref/CSS/XPath/text)
curl "http://localhost:9867/screenshot?selector=%23checkout-button&raw=true" > button.jpg

# Capture element at CSS 1x size (instead of device pixels)
curl "http://localhost:9867/screenshot?selector=%23checkout-button&css1x=true&raw=true" > button-1x.jpg

# Get JSON with base64 JPEG (default)
curl "http://localhost:9867/screenshot"

# Save to server state directory
curl "http://localhost:9867/screenshot?output=file"
```

## Response (JSON)

```json
{
  "path": "/path/to/state/screenshots/screenshot-20260308-120001.jpg",
  "size": 34567,
  "format": "jpeg",
  "timestamp": "20260308-120001"
}
```

## Useful flags

### API Query Parameters

- `format`: `jpeg` (default) or `png`.
- `quality`: JPEG quality `0-100` (default: `80`). Ignored for PNG.
- `selector`: Unified selector to capture one element (e.g. `e5`, `#id`, `xpath://...`, `text:Submit`).
- `css1x`: `true` to output selector screenshots at CSS pixel size (1x). Ignored when `selector` is omitted.
- `raw`: `true` to return image bytes directly instead of JSON.
- `output`: `file` to save to state directory.
- `tabId`: Target a specific tab.

### CLI

- `-o <path>`: Save to specific path.
- `-q <0-100>`: Set JPEG quality.
- `-s <selector>`: Capture a specific element.
- `--css-1x`: With `-s/--selector`, export at CSS 1x size.
- `--tab <id>`: Target a specific tab.

## Related Pages

- [Snapshot](./snapshot.md)
- [PDF](./pdf.md)
