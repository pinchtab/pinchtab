# Pinchtab Security & Trust

**TL;DR**: Pinchtab is a local, sandboxed browser control tool. It does not phone home, steal credentials, or exfiltrate data. Source code is public; binaries are signed and published via GitHub.

## What Pinchtab Does

- Launches a Chrome browser (local, under your control)
- Exposes navigation, clicking, typing, and page inspection via HTTP API
- Extracts the page's accessibility tree (for AI agents)
- Runs screenshots, PDFs, and JavaScript evaluation

High-risk operations such as JavaScript evaluation, local-file upload, and direct file writes should be treated as explicit opt-in actions for the current task, not the default workflow.

**All of this stays local.** No telemetry. No external API calls (except to sites you navigate to).

## What Pinchtab Does NOT Do

- ❌ Doesn't access your saved passwords/credentials (Chrome sandboxing)
- ❌ Doesn't exfiltrate data to remote servers
- ❌ Doesn't inject ads, malware, or miners
- ❌ Doesn't track browsing or send analytics
- ❌ Doesn't modify system files outside its state directory (`~/.pinchtab`)

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
- **Latest**: v0.8.4 (Mar 2026)

If you're concerned, audit the source—it's ~15MB, zero external dependencies, mostly Go stdlib.

## VirusTotal Flag

Pinchtab may trigger heuristic scanners on VirusTotal because:

- ✓ It launches Chrome (subprocess execution — flagged by AV heuristics)
- ✓ It runs JavaScript evaluation (eval-like operations)
- ✓ It makes HTTP requests (network activity)

These are **intentional design features**, not security flaws. Your browser does all three things by default.

**False positives are common for development tools.** The VT flag is a known false positive for chromedp-based tools (subprocess + HTTP server). Always verify SHA256 checksums from GitHub releases before running.

For maximum confidence, use the npm package (`npm install -g pinchtab`) or Docker image, which undergo additional validation.

## Sandboxing

Pinchtab runs a separate Chrome process with:

- Isolated profile directory (default: `~/.pinchtab`)
- No access to your user's home files (unless you explicitly navigate to `file://` URLs)
- Standard Chrome security model (site isolation, CSP, etc.)

Use `profiles.baseDir`, `profiles.defaultProfile`, or `PINCHTAB_CONFIG` if you need to control where PinchTab stores browser state.

## Security History

| CVE | Severity | Affected | Fixed In | Endpoint |
| --- | --- | --- | --- | --- |
| [CVE-2026-30834](https://github.com/advisories/GHSA-rw8p-c6hf-q3pg) | High (7.5) | < 0.7.7 | 0.7.7 | `/download` |

**Type:** Server-Side Request Forgery (SSRF) — allowed exfiltration of internal files and network probing via crafted download URLs.

**Fix PRs:** [#135](https://github.com/pinchtab/pinchtab/pull/135) (SafePath validation), [#288](https://github.com/pinchtab/pinchtab/pull/288) (expanded URL validation).

**Minimum recommended version:** 0.8.3+ (includes full SSRF hardening).

## Questions?

- Source code: https://github.com/pinchtab/pinchtab
- Issues/security reports: https://github.com/pinchtab/pinchtab/issues
- Docs: https://pinchtab.com
