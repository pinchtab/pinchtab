# PinchTab Security & Trust

**TL;DR**: PinchTab is a local, sandboxed browser control tool. It does not phone home, steal credentials, or exfiltrate data. Source code is public; binaries are signed and published via GitHub.

## What PinchTab Does

- Launches a Chrome browser (local, under your control)
- Exposes navigation, clicking, typing, and page inspection via HTTP API
- Extracts the page's accessibility tree (for AI agents)
- Runs screenshots and PDFs; JavaScript evaluation is available but **disabled by default** (`security.allowEvaluate = false`)

High-risk operations such as JavaScript evaluation, local-file upload, file downloads, cookie access, and network export should be treated as explicit opt-in actions for the current task, not the default workflow. These are gated by security policy and disabled by default.

**All of this stays local.** No telemetry. No external API calls (except to sites you navigate to).

## What PinchTab Does NOT Do

- ❌ Doesn't access your saved passwords/credentials (Chrome sandboxing)
- ❌ Doesn't exfiltrate data to remote servers
- ❌ Doesn't inject ads, malware, or miners
- ❌ Doesn't track browsing or send analytics
- ❌ Doesn't modify system files outside its state directory (`~/.pinchtab`)

## Security Policy (Defaults)

High-impact capabilities are **disabled by default** and require explicit configuration:

| Capability | Default | Config Key |
|---|---|---|
| JavaScript evaluation | **Disabled** | `security.allowEvaluate` |
| File downloads | **Disabled** | `security.allowDownloads` |
| File uploads | **Disabled** | `security.allowUploads` |
| Network interception | **Disabled** | `security.allowNetworkIntercept` |
| `file://` navigation | **Disabled** | `security.allowFileScheme`; grants read access to local files the server can read, and `file://` has no host so it is **not** constrained by `allowedDomains` or the SSRF guard — enable only on trusted, single-tenant hosts |
| Challenge solving (autoSolver) | **Disabled** | `autoSolver.enabled`; requires explicit opt-in per deployment |
| Stealth level | **Light** (minimal) | `instanceDefaults.stealthLevel`; `light` applies only basic fingerprint normalization — anti-bot bypass requires `medium` or `full`, which must be set explicitly |
| Navigation domains | **Local-only allowlist** | `security.allowedDomains` (restrict or expand deliberately) |
| Cookie access | **Disabled** | `security.allowCookies`; use only when task requires it; do not log or expose session tokens |

Agents reusing authenticated browser sessions should use dedicated low-privilege profiles and confirm with the user before performing account-changing actions.

## Network Export Defaults

Network exports capture request/response metadata but apply conservative defaults:

- **Response bodies are excluded** unless explicitly requested with `body=true`
- **Sensitive headers are redacted by default** (`redact=true`) — `Cookie`, `Set-Cookie`, `Authorization`, `Proxy-Authorization`, `X-API-Key`, and `X-CSRF-Token` are replaced with `[REDACTED]`
- Exported files are written with **0600 permissions** (owner-only access)
- Writes are atomic (temp file + rename) with path traversal prevention

## Daemon Lifecycle

The background daemon (`pinchtab daemon install`) is a convenience for persistent local use, not a requirement. When browser automation is no longer needed:

- `pinchtab daemon stop` — stop the daemon without uninstalling
- `pinchtab daemon uninstall` — disable and remove the service file entirely

As an alternative to the daemon, run `pinchtab server` in the foreground — it stops when the terminal closes.

Agent sessions expire after **30 minutes of idle** by default (`sessions.agent.idleTimeoutSec`), and tabs can auto-close via `tabPolicy.lifecycle: "close_idle"`.

## Builds & Verification

Every release includes **checksums** alongside binaries:

```bash
# After downloading, verify:
sha256sum -c checksums.txt
```

Binaries are built automatically from tagged commits via GitHub Actions (publicly visible at https://github.com/pinchtab/pinchtab/actions).

## Open Source

- **Source**: https://github.com/pinchtab/pinchtab (MIT)
- **Releases**: https://github.com/pinchtab/pinchtab/releases

If you're concerned, audit the source—it's ~15MB, zero external dependencies, mostly Go stdlib.

## VirusTotal Flag

PinchTab may trigger heuristic scanners on VirusTotal because:

- ✓ It launches Chrome (subprocess execution — flagged by AV heuristics)
- ✓ It runs JavaScript evaluation (eval-like operations)
- ✓ It makes HTTP requests (network activity)

These are **intentional design features**, not security flaws. Your browser does all three things by default.

**False positives are common for development tools.** The VT flag is a known false positive for chromedp-based tools (subprocess + HTTP server). Always verify SHA256 checksums from GitHub releases before running.

For maximum confidence, use the npm package (`npm install -g pinchtab`) or Docker image, which undergo additional validation.

## Sandboxing

PinchTab runs a separate Chrome process with:

- Isolated profile directory (default: `~/.pinchtab`)
- No access to your user's home files — `file://` navigation is rejected unless `security.allowFileScheme` is explicitly enabled
- Standard Chrome security model (site isolation, CSP, etc.)

Use `profiles.baseDir`, `profiles.defaultProfile`, or `PINCHTAB_CONFIG` if you need to control where PinchTab stores browser state.

## Security History

| Advisory | Severity | Fixed In |
| --- | --- | --- |
| [GHSA-p8mm-644p-phmh / CVE-2026-33623](https://github.com/advisories/GHSA-p8mm-644p-phmh) | Medium | 0.8.5 |
| [GHSA-w5pc-m664-r62v / CVE-2026-33622](https://github.com/advisories/GHSA-w5pc-m664-r62v) | Medium | 0.8.5 |
| [GHSA-j65m-hv65-r264 / CVE-2026-33621](https://github.com/advisories/GHSA-j65m-hv65-r264) | Medium | 0.8.4 |
| [GHSA-mrqc-3276-74f8 / CVE-2026-33620](https://github.com/advisories/GHSA-mrqc-3276-74f8) | Medium | 0.8.4 |
| [GHSA-xqq2-4j46-vwp7 / CVE-2026-33619](https://github.com/advisories/GHSA-xqq2-4j46-vwp7) | Medium | 0.8.4 |
| [GHSA-qwxp-6qf9-wr4m / CVE-2026-33081](https://github.com/advisories/GHSA-qwxp-6qf9-wr4m) | Medium | v0.8.3 |
| [GHSA-rw8p-c6hf-q3pg / CVE-2026-30834](https://github.com/advisories/GHSA-rw8p-c6hf-q3pg) | High | v0.7.7 |

## Questions?

- Source code: https://github.com/pinchtab/pinchtab
- Issues/security reports: https://github.com/pinchtab/pinchtab/issues
- Docs: https://pinchtab.com
