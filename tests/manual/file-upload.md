# UP1-UP11: File Upload Tests (Manual)

**Status:** ⚠️ Manual test — headless Chrome limitations

## Why Manual?

Headless Chrome doesn't support:
- `file://` URL navigation (security restriction)
- File picker UI interactions
- Local file access in headless mode

These limitations make automated testing impractical in CI.

## Test Steps

### Setup
1. Start Pinchtab in headed mode (with display):
   ```bash
   BRIDGE_HEADLESS=false ./pinchtab
   ```

2. Have test files ready:
   - `tests/assets/upload-test.html` (form with `#single` and `#multi` inputs)
   - `tests/assets/test-upload.png` (1x1 PNG test image)

### UP1: Single File Upload
1. Navigate to `file:///<repo>/tests/assets/upload-test.html`
2. POST `/upload` with:
   ```json
   {
     "selector": "#single",
     "paths": ["/path/to/test-upload.png"]
   }
   ```
3. Verify response: `{"status":"ok","files":1}`

### UP4: Multiple Files
1. Same setup as UP1
2. POST to `#multi` selector with multiple file paths
3. Verify: `{"status":"ok","files":2}`

### UP6: Default Selector
1. POST `/upload` without selector (should use first `input[type=file]`)
2. Verify: 200 response

### UP7: Invalid Selector
1. POST with selector `#nonexistent`
2. Verify: 400 or 500 error

### UP8: Missing Files
1. POST without `paths` field
2. Verify: 400 error

### UP9: File Not Found
1. POST with non-existent file path
2. Verify: 400 error

### UP11: Bad JSON
1. POST malformed JSON
2. Verify: 400 error

## Notes
- Use headed Chrome (not headless) for these tests
- File picker security may still block access depending on system
- Alternative: mock the file input via JS if `file://` still blocked
