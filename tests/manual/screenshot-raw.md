# SS2: Raw Screenshot Test (Manual)

**Status:** ⚠️ Manual test — headless Chrome CDP limitations

## Why Manual?

Headless Chrome's CDP implementation has limitations with raw screenshot output:
- May fail under certain display configurations
- Requires proper GPU/display context that's not always available in headless
- Works fine in headed mode or with GPU-enabled Docker

## Test Steps

### Setup
1. Start Pinchtab normally (can be headed or headless, but with display support):
   ```bash
   ./pinchtab
   ```

2. Ensure Chrome is running with display context (or GPU if headless)

### SS2: Raw Screenshot

1. Navigate to a page:
   ```bash
   curl -X POST http://localhost:9867/navigate \
     -H "Content-Type: application/json" \
     -d '{"url":"https://example.com"}'
   ```

2. Request raw screenshot:
   ```bash
   curl http://localhost:9867/screenshot?raw=true \
     --output screenshot.jpg
   ```

3. Verify:
   - Status code: 200
   - File exists and is valid JPEG
   - File starts with JPEG magic bytes: `FF D8`
   - Can be opened in image viewer

### Alternative: Base64 Screenshot (Works in CI)

If raw fails, the base64 version works reliably:
```bash
curl http://localhost:9867/screenshot
```

This returns JSON with `base64` field containing the image data.

## Debugging

If `/screenshot?raw=true` returns 500:

1. Check Pinchtab logs for CDP errors
2. Try base64 version instead (SS1)
3. Ensure display is available (check `DISPLAY` env var)
4. Try in headed mode: `BRIDGE_HEADLESS=false ./pinchtab`
