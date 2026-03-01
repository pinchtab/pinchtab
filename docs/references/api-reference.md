# API Reference (Auto-Generated)

Generated: 2026-03-01T00:24:41Z

---

## Endpoints Summary

| Method | Path | Handler | Notes |
|--------|------|---------|-------|
| DELETE | `/profiles/{name}` | handleDeleteByPath |  |
| GET | `/` |  |  |
| GET | `/cookies` | HandleGetCookies |  |
| GET | `/dashboard` |  |  |
| GET | `/dashboard/` |  |  |
| GET | `/dashboard/agents` | handleAgents |  |
| GET | `/dashboard/events` | handleSSE |  |
| GET | `/download` | HandleDownload |  |
| GET | `/health` | HandleHealth |  |
| GET | `/instances` | handleList |  |
| GET | `/instances/tabs` | handleAllTabs |  |
| GET | `/instances/{id}/cookies` | proxyToInstance |  |
| GET | `/instances/{id}/download` | proxyToInstance |  |
| GET | `/instances/{id}/logs` | handleLogsByID |  |
| GET | `/instances/{id}/pdf` | proxyToInstance |  |
| GET | `/instances/{id}/proxy/screencast` | handleProxyScreencast |  |
| GET | `/instances/{id}/screencast` | proxyToInstance |  |
| GET | `/instances/{id}/screenshot` | proxyToInstance |  |
| GET | `/instances/{id}/snapshot` | proxyToInstance |  |
| GET | `/instances/{id}/tabs` | proxyToInstance |  |
| GET | `/instances/{id}/text` | proxyToInstance |  |
| GET | `/pdf` | HandlePDF |  |
| GET | `/profiles` | handleList |  |
| GET | `/profiles/{id}/instance` | handleProfileInstance |  |
| GET | `/profiles/{name}/analytics` | handleAnalyticsByPath |  |
| GET | `/profiles/{name}/logs` | handleLogsByPath |  |
| GET | `/screencast` | HandleScreencast |  |
| GET | `/screencast/tabs` | HandleScreencastAll |  |
| GET | `/screenshot` | HandleScreenshot |  |
| GET | `/snapshot` | HandleSnapshot |  |
| GET | `/stealth/status` | HandleStealthStatus |  |
| GET | `/tabs` | HandleTabs |  |
| GET | `/text` | HandleText |  |
| GET | `/welcome` |  |  |
| PATCH | `/profiles/meta` | handleUpdateMeta |  |
| PATCH | `/profiles/{name}` | handleUpdateByPath |  |
| POST | `/action` | HandleAction |  |
| POST | `/actions` | HandleActions |  |
| POST | `/cookies` | HandleSetCookies |  |
| POST | `/ensure-chrome` | HandleEnsureChrome |  |
| POST | `/evaluate` | HandleEvaluate |  |
| POST | `/fingerprint/rotate` | HandleFingerprintRotate |  |
| POST | `/instances/launch` | handleLaunchByName |  |
| POST | `/instances/{id}/action` | proxyToInstance |  |
| POST | `/instances/{id}/actions` | proxyToInstance |  |
| POST | `/instances/{id}/cookies` | proxyToInstance |  |
| POST | `/instances/{id}/ensure-chrome` | proxyToInstance |  |
| POST | `/instances/{id}/evaluate` | proxyToInstance |  |
| POST | `/instances/{id}/navigate` | proxyToInstance |  |
| POST | `/instances/{id}/stop` | handleStopByInstanceID |  |
| POST | `/instances/{id}/tab` | proxyToInstance |  |
| POST | `/instances/{id}/tab/lock` | proxyToInstance |  |
| POST | `/instances/{id}/tab/unlock` | proxyToInstance |  |
| POST | `/instances/{id}/upload` | proxyToInstance |  |
| POST | `/navigate` | HandleNavigate |  |
| POST | `/profiles/create` | handleCreate |  |
| POST | `/profiles/import` | handleImport |  |
| POST | `/profiles/{id}/start` | handleStartByID |  |
| POST | `/profiles/{id}/stop` | handleStopByID |  |
| POST | `/profiles/{name}/reset` | handleResetByPath |  |
| POST | `/start/{id}` | handleStartByID |  |
| POST | `/stop/{id}` | handleStopByID |  |
| POST | `/tab` | HandleTab |  |
| POST | `/tab/lock` | HandleTabLock |  |
| POST | `/tab/unlock` | HandleTabUnlock |  |
| POST | `/upload` | HandleUpload |  |

---

## Detailed Endpoints

### DELETE /profiles/{name}

**Handler:** `handleDeleteByPath`

### GET /

**Handler:** ``

### GET /cookies

**Handler:** `HandleGetCookies`

### GET /dashboard

**Handler:** ``

### GET /dashboard/

**Handler:** ``

### GET /dashboard/agents

**Handler:** `handleAgents`

### GET /dashboard/events

**Handler:** `handleSSE`

### GET /download

**Handler:** `HandleDownload`

### GET /health

**Handler:** `HandleHealth`

### GET /instances

**Handler:** `handleList`

### GET /instances/tabs

**Handler:** `handleAllTabs`

### GET /instances/{id}/cookies

**Handler:** `proxyToInstance`

### GET /instances/{id}/download

**Handler:** `proxyToInstance`

### GET /instances/{id}/logs

**Handler:** `handleLogsByID`

### GET /instances/{id}/pdf

**Handler:** `proxyToInstance`

### GET /instances/{id}/proxy/screencast

**Handler:** `handleProxyScreencast`

### GET /instances/{id}/screencast

**Handler:** `proxyToInstance`

### GET /instances/{id}/screenshot

**Handler:** `proxyToInstance`

### GET /instances/{id}/snapshot

**Handler:** `proxyToInstance`

### GET /instances/{id}/tabs

**Handler:** `proxyToInstance`

### GET /instances/{id}/text

**Handler:** `proxyToInstance`

### GET /pdf

**Handler:** `HandlePDF`

### GET /profiles

**Handler:** `handleList`

### GET /profiles/{id}/instance

**Handler:** `handleProfileInstance`

### GET /profiles/{name}/analytics

**Handler:** `handleAnalyticsByPath`

### GET /profiles/{name}/logs

**Handler:** `handleLogsByPath`

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

### PATCH /profiles/meta

**Handler:** `handleUpdateMeta`

### PATCH /profiles/{name}

**Handler:** `handleUpdateByPath`

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

### POST /instances/launch

**Handler:** `handleLaunchByName`

### POST /instances/{id}/action

**Handler:** `proxyToInstance`

### POST /instances/{id}/actions

**Handler:** `proxyToInstance`

### POST /instances/{id}/cookies

**Handler:** `proxyToInstance`

### POST /instances/{id}/ensure-chrome

**Handler:** `proxyToInstance`

### POST /instances/{id}/evaluate

**Handler:** `proxyToInstance`

### POST /instances/{id}/navigate

**Handler:** `proxyToInstance`

### POST /instances/{id}/stop

**Handler:** `handleStopByInstanceID`

### POST /instances/{id}/tab

**Handler:** `proxyToInstance`

### POST /instances/{id}/tab/lock

**Handler:** `proxyToInstance`

### POST /instances/{id}/tab/unlock

**Handler:** `proxyToInstance`

### POST /instances/{id}/upload

**Handler:** `proxyToInstance`

### POST /navigate

**Handler:** `HandleNavigate`

### POST /profiles/create

**Handler:** `handleCreate`

### POST /profiles/import

**Handler:** `handleImport`

### POST /profiles/{id}/start

**Handler:** `handleStartByID`

### POST /profiles/{id}/stop

**Handler:** `handleStopByID`

### POST /profiles/{name}/reset

**Handler:** `handleResetByPath`

### POST /start/{id}

**Handler:** `handleStartByID`

### POST /stop/{id}

**Handler:** `handleStopByID`

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

- This documentation is auto-generated from Go code
- For full implementation details, see `internal/handlers/*.go`
- Query parameters and request bodies are defined in each handler
