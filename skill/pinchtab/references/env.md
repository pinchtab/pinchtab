# Pinchtab Environment Variables

## Core runtime

| Var | Default | Description |
|---|---|---|
| `BRIDGE_BIND` | `127.0.0.1` | Bind address. Set `0.0.0.0` for network access |
| `BRIDGE_PORT` | `9867` | HTTP port |
| `BRIDGE_HEADLESS` | `true` | Run Chrome headless |
| `BRIDGE_TOKEN` | (none) | Bearer auth token (recommended with `0.0.0.0`) |
| `BRIDGE_PROFILE` | `~/.pinchtab/chrome-profile` | Chrome profile dir |
| `BRIDGE_STATE_DIR` | `~/.pinchtab` | State/session storage |
| `BRIDGE_NO_RESTORE` | `false` | Skip tab restore on startup |
| `BRIDGE_STEALTH` | `light` | Stealth level: `light` or `full` |
| `BRIDGE_MAX_TABS` | `20` | Max open tabs (0 = unlimited) |
| `BRIDGE_BLOCK_IMAGES` | `false` | Block image loading |
| `BRIDGE_BLOCK_MEDIA` | `false` | Block all media (images + fonts + CSS + video) |
| `BRIDGE_NO_ANIMATIONS` | `false` | Disable CSS animations/transitions |
| `BRIDGE_TIMEZONE` | (none) | Force browser timezone (IANA tz) |
| `BRIDGE_CHROME_VERSION` | `144.0.7559.133` | Chrome version for fingerprint rotation |
| `BRIDGE_USER_AGENT` | (none) | Custom User-Agent string; also overrides Sec-Ch-Ua client hints via CDP |
| `CHROME_BINARY` | (auto) | Path to Chrome/Chromium binary |
| `CHROME_FLAGS` | (none) | Extra Chrome flags (space-separated) |
| `BRIDGE_CONFIG` | `~/.pinchtab/config.json` | Path to config JSON file |
| `BRIDGE_TIMEOUT` | `15` | Action timeout (seconds) |
| `BRIDGE_NAV_TIMEOUT` | `30` | Navigation timeout (seconds) |
| `CDP_URL` | (none) | Connect to existing Chrome DevTools |
| `BRIDGE_NO_DASHBOARD` | `false` | Disable dashboard endpoints on instance processes |

## Dashboard mode (`pinchtab dashboard`)

| Var | Default | Description |
|---|---|---|
| `PINCHTAB_AUTO_LAUNCH` | `false` | Auto-launch default profile at startup |
| `PINCHTAB_DEFAULT_PROFILE` | `default` | Profile name for auto-launch |
| `PINCHTAB_DEFAULT_PORT` | `9867` | Port for auto-launched profile |
| `PINCHTAB_HEADED` | (unset) | If set, auto-launched profile is headed |
| `PINCHTAB_DASHBOARD_URL` | `http://localhost:$BRIDGE_PORT` | Base URL for `pinchtab connect` |
