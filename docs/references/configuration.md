# Configuration

Complete reference for all PinchTab environment variables and configuration options.

## Environment Variables

### Port & Network

| Variable | Default | Description |
|---|---|---|
| `BRIDGE_PORT` | `9867` | HTTP server port |
| `BRIDGE_BIND` | `127.0.0.1` | Bind address (127.0.0.1 = localhost only, 0.0.0.0 = all interfaces) |

### Browser & Chrome

| Variable | Default | Description |
|---|---|---|
| `CHROME_BINARY` | Auto-detect | Path to Chrome/Chromium binary |
| `BRIDGE_HEADLESS` | `true` | Run Chrome headless (no visible window) |
| `BRIDGE_PROFILE` | Default profile | Chrome profile name (stored in `~/.pinchtab/profiles/{name}`) |

### Stealth & Detection

| Variable | Default | Description |
|---|---|---|
| `BRIDGE_STEALTH` | `light` | Stealth level: `light`, `medium`, `full` (higher = more bot detection bypass, slower) |

### Content Filtering

| Variable | Default | Description |
|---|---|---|
| `BRIDGE_BLOCK_ADS` | `false` | Block ad domains (speeds up loading) |
| `BRIDGE_BLOCK_IMAGES` | `false` | Block image loading |
| `BRIDGE_BLOCK_MEDIA` | `false` | Block video/audio resources |

### Security & Authentication

| Variable | Default | Description |
|---|---|---|
| `BRIDGE_TOKEN` | Disabled | API authentication token (if set, all requests must include `Authorization: Bearer {token}`) |

### Debugging & Logging

| Variable | Default | Description |
|---|---|---|
| `BRIDGE_DEBUG` | `false` | Enable debug logging (verbose output) |
| `BRIDGE_LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, `error` |

### Dashboard

| Variable | Default | Description |
|---|---|---|
| `BRIDGE_DASHBOARD_PORT` | Same as `BRIDGE_PORT` | Dashboard HTTP port (usually same as API server) |
| `BRIDGE_NO_DASHBOARD` | `false` | Disable dashboard (API-only mode) |

---

## Usage Examples

### Basic Setup

```bash
# Default (headless, localhost:9867)
./pinchtab
```

### Custom Port

```bash
BRIDGE_PORT=9868 ./pinchtab
```

### Network Access (External)

```bash
# Allow connections from other machines
BRIDGE_BIND=0.0.0.0 BRIDGE_PORT=9867 ./pinchtab
```

**⚠️ Security Warning:** Only use `0.0.0.0` on trusted networks. Consider using `BRIDGE_TOKEN` for authentication.

### Headed Mode with Profile

```bash
BRIDGE_HEADLESS=false BRIDGE_PROFILE=work ./pinchtab
```

Opens visible Chrome window with "work" profile.

### Stealth Mode (Bypass Bot Detection)

```bash
BRIDGE_STEALTH=full ./pinchtab
```

Options:
- `light` — Basic patches (default, minimal overhead)
- `medium` — More aggressive patches (some overhead)
- `full` — Maximum stealth (significant overhead, slowest)

### Ad Blocking

```bash
BRIDGE_BLOCK_ADS=true ./pinchtab
```

Speeds up page loading by blocking ad domains.

### API Authentication

```bash
BRIDGE_TOKEN=my-secret-token ./pinchtab
```

Then all API requests must include:
```bash
curl -H "Authorization: Bearer my-secret-token" http://localhost:9867/health
```

### Multiple Settings

```bash
BRIDGE_PORT=9868 \
BRIDGE_HEADLESS=false \
BRIDGE_STEALTH=full \
BRIDGE_BLOCK_ADS=true \
BRIDGE_PROFILE=dev \
BRIDGE_TOKEN=secret \
./pinchtab
```

### Debug Mode

```bash
BRIDGE_DEBUG=true BRIDGE_LOG_LEVEL=debug ./pinchtab
```

Produces verbose logs for troubleshooting.

---

## Configuration Priority

If multiple sources set the same value:

1. **Command-line flags** (highest priority)
2. **Environment variables**
3. **Config file** (if supported)
4. **Built-in defaults** (lowest priority)

Example:
```bash
# BRIDGE_PORT=9868 from env, but --port flag overrides it
./pinchtab --port 9870  # Uses 9870
```

---

## Chrome Profile Directory Structure

Profiles are stored in `~/.pinchtab/profiles/{id}/`:

```text
~/.pinchtab/profiles/
├── prof_9f86d081/
│   ├── Default/
│   │   ├── Preferences
│   │   ├── Cookies
│   │   ├── History
│   │   └── ... (other Chrome data)
│   └── ...
├── prof_dc34vewr/
│   ├── Default/
│   │   └── ... (Chrome data)
│   └── ...
└── prof_34ff6ks9/
    └── ... (default profile)
```

Each profile maintains its own:
- Cookies and session data
- Browsing history
- Saved passwords
- Local storage
- Cache

---

## Performance Tuning

### For Speed (Reduce Overhead)

```bash
# Minimal stealth, block ads
BRIDGE_STEALTH=light \
BRIDGE_BLOCK_ADS=true \
./pinchtab
```

### For Security (Detect Bypass)

```bash
# Maximum stealth
BRIDGE_STEALTH=full ./pinchtab
```

Note: Maximum stealth increases latency by 2-3x.

### For Bandwidth (Block Resources)

```bash
# Block ads, images, and media
BRIDGE_BLOCK_ADS=true \
BRIDGE_BLOCK_IMAGES=true \
BRIDGE_BLOCK_MEDIA=true \
./pinchtab
```

---

## Network Configuration

### Localhost Only (Default - Secure)

```bash
BRIDGE_BIND=127.0.0.1 ./pinchtab
```

Only accessible from the same machine.

### All Interfaces (Network Accessible)

```bash
BRIDGE_BIND=0.0.0.0 ./pinchtab
```

Accessible from any machine on the network. **Requires authentication:**

```bash
BRIDGE_BIND=0.0.0.0 BRIDGE_TOKEN=secret ./pinchtab
```

### Specific Interface

```bash
BRIDGE_BIND=192.168.1.100 ./pinchtab
```

Only accessible from that IP address.

---

## Troubleshooting

### Port Already in Use

```bash
# Find what's using the port
lsof -i :9867

# Use different port
BRIDGE_PORT=9868 ./pinchtab
```

### Chrome Not Found

```bash
# Specify custom binary path
CHROME_BINARY=/usr/bin/google-chrome ./pinchtab
```

### High Latency with Stealth Mode

```bash
# Reduce stealth level
BRIDGE_STEALTH=light ./pinchtab
```

Maximum stealth (`full`) adds significant overhead.

### Authentication Errors

```bash
# Check token format
BRIDGE_TOKEN=my-token ./pinchtab

# Request must include the token
curl -H "Authorization: Bearer my-token" http://localhost:9867/health
```

---

## Security Best Practices

1. **Use `BRIDGE_TOKEN` when exposing to network:**
   ```bash
   BRIDGE_BIND=0.0.0.0 BRIDGE_TOKEN=$(openssl rand -hex 32) ./pinchtab
   ```

2. **Use localhost by default:**
   ```bash
   # Good: only accessible locally
   BRIDGE_BIND=127.0.0.1 ./pinchtab
   ```

3. **Use SSH tunneling for remote access:**
   ```bash
   # Remote: start without binding to all interfaces
   ssh user@remote "BRIDGE_BIND=127.0.0.1 pinchtab"

   # Local: tunnel through SSH
   ssh -L 9867:127.0.0.1:9867 user@remote

   # Then use locally
   curl http://localhost:9867/health
   ```

4. **Use firewall rules if exposing to network:**
   ```bash
   # Only allow from specific IPs
   ufw allow from 192.168.1.0/24 to any port 9867
   ```

---

## Related Documentation

- [API Reference](endpoints.md) — HTTP endpoints
- [Getting Started](../get-started.md) — Quick setup
- [Core Concepts](../core-concepts.md) — Instances, profiles, tabs
