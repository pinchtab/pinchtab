# Security Policy

For the user-facing security guide, see
[docs/guides/security.md](./docs/guides/security.md).

## Supported Versions

We currently support the following versions of Pinchtab with security updates:

| Version | Supported          |
| ------- | ------------------ |
| v0.8.4  | :white_check_mark: |
| < v0.8.3  | :x: |

## Current Posture

The current codebase has the following security controls in place:

- query-string token auth has been removed
- browser dashboard auth uses a server-side `HttpOnly` session cookie instead
  of raw-token URL or browser-storage flows
- cookie-authenticated browser requests are same-origin enforced
- sensitive dashboard actions use elevation rather than normal browser session
  auth alone
- login attempts are rate-limited and important auth/admin actions are audited
- current-tab domain policy is enforced after load, not only at initial
  navigation time
- popup/opener abuse is hardened in the browser runtime
- safer default Chrome flags are used than in the earlier posture
- scheduler callback destinations are validated before use
- attach is disabled by default

## Reporting a Vulnerability

We take the security of our browser automation bridge seriously. If you believe you have found a security vulnerability, please do not report it via a public GitHub issue.

Instead, please report vulnerabilities privately by:

1.  Opening a **Private Vulnerability Report** on GitHub (if available for this repo).
2.  Or emailing the maintainer directly at [INSERT EMAIL ADDRESS].

Please include:
- A description of the vulnerability.
- Steps to reproduce (proof of concept).
- Potential impact.

We will acknowledge receipt of your report within 48 hours and provide a timeline for a fix if necessary.
