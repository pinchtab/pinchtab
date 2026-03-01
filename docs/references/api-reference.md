# API Reference v2

Auto-generated from Go code in `internal/handlers/handlers.go`

Generated: 2026-03-01T00:20:37Z

---

## Endpoints Summary

| Method | Path | Handler | Notes |
|--------|------|---------|-------|
| GET | `/cookies` | HandleGetCookies |  |
| GET | `/download` | HandleDownload |  |
| GET | `/health` | HandleHealth |  |
| GET | `/pdf` | HandlePDF |  |
| GET | `/screencast` | HandleScreencast |  |
| GET | `/screencast/tabs` | HandleScreencastAll |  |
| GET | `/screenshot` | HandleScreenshot |  |
| GET | `/snapshot` | HandleSnapshot |  |
| GET | `/stealth/status` | HandleStealthStatus |  |
| GET | `/tabs` | HandleTabs |  |
| GET | `/text` | HandleText |  |
| GET | `/welcome` |  |  |
| POST | `/action` | HandleAction |  |
| POST | `/actions` | HandleActions |  |
| POST | `/cookies` | HandleSetCookies |  |
| POST | `/ensure-chrome` | HandleEnsureChrome |  |
| POST | `/evaluate` | HandleEvaluate |  |
| POST | `/fingerprint/rotate` | HandleFingerprintRotate |  |
| POST | `/navigate` | HandleNavigate |  |
| POST | `/tab` | HandleTab |  |
| POST | `/tab/lock` | HandleTabLock |  |
| POST | `/tab/unlock` | HandleTabUnlock |  |
| POST | `/upload` | HandleUpload |  |

---

## Detailed Endpoints

### GET /cookies

**Handler:** `HandleGetCookies`

### GET /download

**Handler:** `HandleDownload`

### GET /health

**Handler:** `HandleHealth`

### GET /pdf

**Handler:** `HandlePDF`

### GET /screencast

**Handler:** `HandleScreencast`

### GET /screencast/tabs

**Handler:** `HandleScreencastAll`

### GET /screenshot

**Handler:** `HandleScreenshot`

### GET /snapshot

**Handler:** `HandleSnapshot`

### GET /stealth/status

**Handler:** `HandleStealthStatus`

### GET /tabs

**Handler:** `HandleTabs`

### GET /text

**Handler:** `HandleText`

### GET /welcome

**Handler:** ``

### POST /action

**Handler:** `HandleAction`

### POST /actions

**Handler:** `HandleActions`

### POST /cookies

**Handler:** `HandleSetCookies`

### POST /ensure-chrome

**Handler:** `HandleEnsureChrome`

### POST /evaluate

**Handler:** `HandleEvaluate`

### POST /fingerprint/rotate

**Handler:** `HandleFingerprintRotate`

### POST /navigate

**Handler:** `HandleNavigate`

### POST /tab

**Handler:** `HandleTab`

### POST /tab/lock

**Handler:** `HandleTabLock`

### POST /tab/unlock

**Handler:** `HandleTabUnlock`

### POST /upload

**Handler:** `HandleUpload`

---

## Notes

- This documentation is **auto-generated** from Go code
- For full implementation details, see `internal/handlers/*.go`
- Query parameters and request bodies are defined in each handler
- To regenerate: `go run scripts/gen-api-docs/main.go > docs/references/api-reference.md`

---

## Related Documentation

- [API Reference (Full)](endpoints.md) — Comprehensive API reference with examples and details
- [CLI Reference](cli-commands.md) — Command-line interface documentation
- [Configuration](configuration.md) — Environment variables and settings
