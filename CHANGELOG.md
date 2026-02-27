# Changelog

## Unreleased

### Added
- **PDF export parameters** — `/pdf` API now supports full PrintToPDFParams: `paperWidth`, `paperHeight`, `marginTop`, `marginBottom`, `marginLeft`, `marginRight`, `pageRanges`, `displayHeaderFooter`, `headerTemplate`, `footerTemplate`, `generateTaggedPDF`, `generateDocumentOutline`, `preferCSSPageSize` (#45, PR #48)
- **Complete PDF CLI options** — `pinchtab pdf` command now supports all PDF export parameters matching the API functionality. Includes paper dimensions, margins, page ranges, headers/footers, and accessibility options (#54)
- **Installer branding** — install.sh now uses yellow accent color (#fbbf24) and crab icon 🦀 for consistency (PR #48)
- **Ad blocking** — New `blockAds` option blocks 100+ tracking/analytics domains for cleaner snapshots. Available via env var `BRIDGE_BLOCK_ADS=true`, CLI flag `--block-ads`, and API parameter (#50)

### Changed
- **Human package testability** — Added `Config` struct and `TypeWithConfig` function to inject custom random source for deterministic testing (#50)
- **WebSocket proxy standards** — Improved HTTP header formatting using `fmt.Fprintf` and `textproto.CanonicalMIMEHeaderKey` (#50)
- **Integration test stability** — Added retry logic, increased timeouts, and better error reporting for flaky tests (#50)

### Docs
- **Definition of Done** — Added DEFINITION_OF_DONE.md and GitHub PR template with streamlined, actionable checklist (PR #48)

## v0.5.0

### Added
- **Dashboard orchestrator mode** — `pinchtab dashboard` runs profiles/instances without launching a browser in the dashboard process
- **Profile lifecycle APIs** — launch, stop-by-profile, per-profile instance status, and aggregated tabs across running instances
- **`pinchtab connect <profile>`** — resolve a running profile instance URL from dashboard state
- **Direct launch backup command** in dashboard launch modal for manual fallback
- **Run helper script** — `scripts/run_pinchtab.sh` for local build + start convenience

### Changed
- **Default runtime mode** — `pinchtab` now starts headless by default; use `BRIDGE_HEADLESS=false` for headed mode
- **Headed-first dashboard UX** — profile launches from dashboard are headed, with profile state and account details shown in cards
- **Live view UX** — live screencast moved from dedicated screen to profile-scoped popup modal
- **Profiles view defaults** — dashboard opens on Profiles, with profile/status actions prioritized
- **UI refresh** — icon-based branding + updated color system and action hierarchy

### Fixed
- **Startup/health handling** for orchestrated instances with clearer timeout errors and stale-start conflict handling
- **Profile stop flow** now supports graceful stop semantics from the dashboard
- **Status consistency** between process state and dashboard instance cards

### Docs
- Updated run mode documentation (headed/headless/dashboard)
- Expanded architecture and skill docs for headed mode workflows and environment variables
- Refreshed third-party license coverage

## v0.4.0

### Added
- **Tab locking** — `POST /tab/lock`, `POST /tab/unlock` with timeout-based deadlock prevention for multi-agent coordination
- **Tab ownership** — `/tabs` shows `owner` and `lockedUntil` on locked tabs
- **Token optimization** — `maxTokens`, `selector`, `format=compact` params on `/snapshot`
- **CSS animation disabling** — `BRIDGE_NO_ANIMATIONS` env var + `?noAnimations=true` per-request
- **Stealth levels** — `BRIDGE_STEALTH=light` (default) vs `full`; light mode works with X.com and most sites
- **Welcome page** — headed mode shows 🦀 branding on startup
- **`CHROME_BINARY`** — custom Chrome/Chromium path support
- **`CHROME_FLAGS`** — extra Chrome flags (space-separated)
- **`BRIDGE_BLOCK_MEDIA`** — block all media (images + fonts + CSS + video)
- **`/welcome` endpoint** — serves embedded welcome page

### Fixed
- **K10** — Profile hang on startup (lock file cleanup, unclean exit detection, 15s timeout, auto-retry)
- **K11** — File output `path` param now honored, auto-creates parent dirs
- **`blockImages` on `CreateTab`** — global image/media blocking applied to new tabs
- **`Date.getTimezoneOffset` infinite recursion** — stealth script was calling itself; saved original method reference
- **UA mismatch detection** — removed hardcoded User-Agent override, Chrome uses native UA

### Changed
- Default stealth level changed from `full` to `light` (compatibility over fingerprint resistance)
- Dockerfile Go version updated to 1.26
- Coverage improved from 28.9% to 36%+

## v0.3.0

- Stealth suite (navigator, WebGL, canvas noise, font metrics, WebRTC, timezone, plugins, Chrome flags)
- Human interaction (bezier mouse, typing simulation)
- Fingerprint rotation via CDP
- Image blocking (`BRIDGE_BLOCK_IMAGES`)
- Stealth injection on all tabs (CreateTab + TabContext)
- Multi-agent concurrency verified (MA1-MA8)
- 92 unit tests + ~100 integration tests

## v0.2.0

- Session persistence (cookies, tabs survive restarts)
- Config file support
- Readability `/text` endpoint
- Smart diff (`?diff=true`)
- YAML/file output formats
- Handler split (4 files)

## v0.1.0

- Initial release
- Core HTTP API (16 endpoints)
- Accessibility tree snapshots with stable refs
- Chrome DevTools Protocol bridge
- Tab management
- Basic stealth (webdriver, chrome.runtime, plugins)
