# Pinchtab Security & Trust

**TL;DR**: Pinchtab is a local, sandboxed browser control tool. It does not phone home, steal credentials, or exfiltrate data. Source code is public; binaries are signed and published via GitHub.

## What Pinchtab Does

- Launches a Chrome browser (local, under your control)
- Exposes navigation, clicking, typing, and page inspection via HTTP API
- Extracts the page's accessibility tree (for AI agents)
- Runs screenshots, PDFs, and JavaScript evaluation

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

- **Source**: https://github.com/pinchtab/pinchtab (Apache 2.0)
- **Releases**: https://github.com/pinchtab/pinchtab/releases
- **Latest**: v0.7.0 (Feb 2026)

If you're concerned, audit the source—it's 12MB, zero external dependencies, mostly Go stdlib.

## VirusTotal Flag

Pinchtab may trigger heuristic scanners on VirusTotal because:

- ✓ It launches Chrome (subprocess execution — flagged by AV heuristics)
- ✓ It runs JavaScript evaluation (eval-like operations)
- ✓ It makes HTTP requests (network activity)

These are **intentional design features**, not security flaws. Your browser does all three things by default.

**False positives are common for development tools.** If you're concerned, verify the checksum against the [official GitHub release](https://github.com/pinchtab/pinchtab/releases) before running.

## Sandboxing

Pinchtab runs a separate Chrome process with:

- Isolated profile directory (default: `~/.pinchtab`)
- No access to your user's home files (unless you explicitly navigate to `file://` URLs)
- Standard Chrome security model (site isolation, CSP, etc.)

Set `BRIDGE_PROFILE_DIR` to use a custom directory if needed.

## Questions?

- Source code: https://github.com/pinchtab/pinchtab
- Issues/security reports: https://github.com/pinchtab/pinchtab/issues
- Docs: https://pinchtab.com
