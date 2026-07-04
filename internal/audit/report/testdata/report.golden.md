# Site Audit Report

- Schema version: 1.0
- Generated: 2026-07-04 12:00:00 UTC
- Pages audited: 2

## Summary

**Summary score: 85/100**

| Page | Score | Load | Broken assets | Console errors | Status |
|---|---|---|---|---|---|
| Audit Fixture Site | 70 | 200 ms | 2 | 1 | ok |
| http://fixtures/audit-site/down.html | 0 | - | 0 | 0 | error: navigation failed: net::ERR_CONNECTION_REFUSED |

## SEO & Metadata

### Audit Fixture Site

- confidence: 90
- description: Deterministic audit fixture site.
- title: Audit Fixture Site

## Content & Functionality

| Page | Title | Interactive elements |
|---|---|---|
| http://fixtures/audit-site/index.html | Audit Fixture Site | 1 |
| http://fixtures/audit-site/down.html |  | 0 |

## Visual Differences

| Page | Changed | Diff pixels | Diff ratio | Diff image |
|---|---|---|---|---|
| Audit Fixture Site | true | 120 | 0.0200 | diffs/index.diff.png |

## Performance

| Page | TTFB | FCP | LCP | CLS | DOMContentLoaded | Load |
|---|---|---|---|---|---|---|
| Audit Fixture Site | 5 ms | 120 ms | 300 ms | 0.05 | 90 ms | 200 ms |

## Console & JS Errors

### Audit Fixture Site

- `error` boom
- `warning` careful

## Broken Assets

| Page | Asset | Type | Status |
|---|---|---|---|
| Audit Fixture Site | http://fixtures/audit-site/assets/missing.png | image | 404 |
| Audit Fixture Site | http://fixtures/api | xhr | net::ERR_FAILED |

## Usability Issues

- Audit Fixture Site: accessibility score 70/100

## Security Findings

- **insecure-form-action** [medium]: password posts over http (http://fixtures/audit-site/forms.html)

## Recommendations

1. Serve the site over https.
2. Fix 2 broken asset reference(s) — see Broken Assets.
3. Investigate 1 console error(s) — see Console & JS Errors.
4. Improve accessibility on 1 page(s) scoring below 100 — see Usability Issues.
5. Address 1 security finding(s) — see Security Findings.
6. Re-check 1 page(s) that failed to audit.

