# Terminology: Browser Selection Concepts

Canonical reference for vocabulary used in browser selection, configuration,
and provider routing.

## Core terms

| Term            | Scope    | Definition |
|-----------------|----------|------------|
| **browser**     | Public   | The user-facing selection concept. Users choose a browser in config, CLI flags, and API calls. Replaces "engine" in all user-facing contexts. |
| **provider**    | Public   | One of `chrome | cloak | ghost-chrome`. The implementation behind a browser selection. Each provider maps to a distinct launch and routing strategy. |
| **static fetch**| Public   | The lightweight HTTP+DOM path used by `ghost-chrome` before escalating to Chrome. Replaces "lite engine" in user-facing language. |
| **engine**      | Internal | DEPRECATED as a public concept. Kept internally in `internal/engine/` as an implementation detail. Will be removed from public config in a future phase. |
| **target**      | Internal | A named profile for launch settings (binary path, proxy, flags, etc.). Not exposed publicly through API, CLI, or docs. |

## Provider descriptions

### `chrome`

Full Chrome via CDP. Default provider. Launches a local Chromium-based browser
and connects over the Chrome DevTools Protocol.

### `cloak`

CloakBrowser — anti-detection Chrome fork. Uses a patched Chromium binary with
fingerprint randomization, timezone/locale spoofing, and WebRTC leak prevention.

### `ghost-chrome`

Static-first routing. Tries a lightweight HTTP fetch and DOM parse first;
escalates to a full Chrome session when the content is thin, dynamic, or
requires JavaScript execution.

## Migration notes

- The `engine` field in `ServerConfig` and `RuntimeConfig` is deprecated.
  Use `browsers.default` in the config file instead.
- The `provider` field inside `browser {}` config blocks is deprecated in
  favor of the top-level `browsers.default` key.
- Public documentation, CLI help text, and API responses should use
  "browser" and "provider" — never "engine" or "lite engine".
