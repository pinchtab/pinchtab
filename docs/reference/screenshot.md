# Screenshot

Capture the current page as an image. Defaults to **JPEG** format.

```bash
# Get raw PNG bytes
curl "http://localhost:9867/screenshot?format=png&raw=true" > page.png

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
- `raw`: `true` to return image bytes directly instead of JSON.
- `output`: `file` to save to state directory.
- `tabId`: Target a specific tab.

### CLI

- `-o <path>`: Save to specific path.
- `-q <0-100>`: Set JPEG quality.
- `--tab <id>`: Target a specific tab.

## Related Pages

- [Snapshot](./snapshot.md)
- [PDF](./pdf.md)
