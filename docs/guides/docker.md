# Docker Deployment

PinchTab can run in Docker with a mounted data volume for config, profiles, and state.
The bundled image now manages its default config under `/data/.config/pinchtab/config.json`.
If you want full control over the config file path, you can still mount your own file and point `PINCHTAB_CONFIG` at it.

## Quick Start

Build the image from this repository:

```bash
docker build -t pinchtab .
```

Run the container with a persistent data volume:

```bash
docker run -d \
  --name pinchtab \
  -p 127.0.0.1:9867:9867 \
  -v pinchtab-data:/data \
  --shm-size=2g \
  pinchtab
```

On first boot, the image creates `/data/.config/pinchtab/config.json` with `bind: 0.0.0.0` (required for Docker port publishing) and generates a token if needed.

If you inspect the startup security summary from inside Docker, the loopback bind check will still report the effective runtime bind as non-loopback. That is expected: the process is listening on `0.0.0.0` inside the container so Docker port publishing can forward traffic to it.

This does not automatically mean the service is exposed beyond your machine. Host exposure still depends on how you publish the container port. For example:

- `-p 127.0.0.1:9867:9867` keeps the service reachable only from the host machine
- `-p 9867:9867` exposes it on the host's network interfaces

Treat the Docker runtime bind and the host-published address as separate layers. If you expose PinchTab beyond localhost, keep an auth token set and put it behind TLS or a trusted reverse proxy.

## Health Check and Readiness

PinchTab has a two-stage readiness model in Docker:

1. **Dashboard ready**: `/health` returns HTTP 200 — the server process is up
2. **Browser ready**: `/health` response has `defaultInstance.status == "running"` — Chrome is ready

### Why Two Stages?

With the `always-on` strategy (default), PinchTab launches a managed Chrome instance at startup. The dashboard becomes healthy immediately, but Chrome takes a few seconds to initialize. If your application hits `/navigate` or `/snapshot` before Chrome is ready, it gets HTTP 503.

### Docker Compose Healthcheck

The standard healthcheck marks the container as "healthy" when the dashboard responds:

```yaml
healthcheck:
  test: ["CMD-SHELL", "wget -q -O /dev/null http://localhost:9867/health"]
  interval: 3s
  timeout: 10s
  retries: 20
  start_period: 15s
```

This is correct for container orchestration — Docker knows the process is alive and the service is reachable.

### Application-Level Readiness

If your application needs Chrome to be ready before making requests, poll `/health` and check for `defaultInstance.status`:

```bash
# Wait for browser to be ready
until curl -sf http://localhost:9867/health | jq -e '.defaultInstance.status == "running"' > /dev/null 2>&1; do
  sleep 1
done
echo "Browser ready"
```

Or in code:

```javascript
async function waitForBrowser(baseUrl, timeoutMs = 60000) {
  const start = Date.now();
  while (Date.now() - start < timeoutMs) {
    try {
      const res = await fetch(`${baseUrl}/health`);
      const data = await res.json();
      if (data.defaultInstance?.status === "running") return;
    } catch {}
    await new Promise(r => setTimeout(r, 1000));
  }
  throw new Error("Browser not ready within timeout");
}
```

### Full Health Response (Server Mode)

```json
{
  "status": "ok",
  "mode": "dashboard",
  "version": "0.8.0",
  "uptime": 12345,
  "profiles": 1,
  "instances": 1,
  "defaultInstance": {
    "id": "inst_abc12345",
    "status": "running"
  },
  "agents": 0,
  "restartRequired": false
}
```

See [Health Reference](../reference/health.md) for full details.

## Supplying Your Own `config.json`

If you want to manage the config file yourself, mount it and point `PINCHTAB_CONFIG` at it:

```text
docker-data/
└── config.json
```

Example `docker-data/config.json`:

```json
{
  "server": {
    "bind": "0.0.0.0",
    "port": "9867",
    "stateDir": "/data/state"
  },
  "profiles": {
    "baseDir": "/data/profiles",
    "defaultProfile": "default"
  },
  "instanceDefaults": {
    "mode": "headless",
    "noRestore": true
  }
}
```

Run with an explicit config file:

```bash
docker run -d \
  --name pinchtab \
  -p 127.0.0.1:9867:9867 \
  -e PINCHTAB_CONFIG=/config/config.json \
  -v "$PWD/docker-data:/data" \
  -v "$PWD/docker-data/config.json:/config/config.json:ro" \
  --shm-size=2g \
  pinchtab
```

Check it:

```bash
curl http://localhost:9867/health
curl http://localhost:9867/instances
```

## What To Persist

If you want data to survive container restarts, persist:

- the managed config directory or your mounted config file
- the profile directory
- the state directory

Without a mounted volume, profiles and saved session state are ephemeral.

## Runtime Configuration

Supported environment variables:

- `PINCHTAB_CONFIG` — path to custom config file (if not using managed config)
- `PINCHTAB_TOKEN` — auth token (prefer Docker secrets; see below)

Everything else, including bind address and port, should go in `config.json`.

### About `bind: 0.0.0.0` in Containers

The entrypoint sets `bind: 0.0.0.0` in the config on first boot. This is necessary because Docker port publishing requires the process to listen on `0.0.0.0` inside the container.

Example: `docker run -p 127.0.0.1:9867:9867` keeps PinchTab reachable only from your host machine, even though the process internally listens on `0.0.0.0`.

### Docker Secrets (Sensitive Configuration)

The image only consumes `PINCHTAB_TOKEN` from the environment — it does not read a `PINCHTAB_TOKEN_FILE` indirection. To use Docker secrets, source the secret file in your own entrypoint or wrapper before running the image:

```bash
# Create a secret
echo "your-secret-token" | docker secret create pinchtab_token -

# Use it in docker-compose.yml — read the secret file and export PINCHTAB_TOKEN
services:
  pinchtab:
    image: pinchtab/pinchtab
    secrets:
      - pinchtab_token
    entrypoint: ["/bin/sh", "-c"]
    command:
      - |
        export PINCHTAB_TOKEN="$(cat /run/secrets/pinchtab_token)"
        exec /usr/local/bin/docker-entrypoint.sh pinchtab
```

Secrets mounted at `/run/secrets/...` are read-only and never appear in `docker ps` or logs.

## Compose

The repository includes a `docker-compose.yml` that follows the managed-config pattern:

1. mount a persistent `/data` volume
2. let the entrypoint create and maintain `/data/.config/pinchtab/config.json`
3. optionally pass `PINCHTAB_TOKEN`

If you prefer a fully user-managed config file, mount it separately and set `PINCHTAB_CONFIG`.

If you expose PinchTab beyond localhost, set an auth token and put it behind TLS or a trusted reverse proxy.

## Security

### Chrome Sandbox Disabled in Containers

PinchTab runs Chrome with `--no-sandbox` in containers. This is standard practice because:

- **User namespaces unavailable**: Containers don't have the full namespace isolation Chrome's sandbox requires
- **Container security compensates**: The Docker image uses:
  - `cap_drop: ALL` (no capabilities)
  - `read_only: true` (immutable filesystem)
  - `seccomp` default profile (syscall filtering)
  - Non-root user
- **Isolation at container layer**: The container runtime (cgroups, seccomp, AppArmor/SELinux) provides the security boundary

This configuration is used by major headless browser services (Puppeteer, Playwright, Browserless).

PinchTab manages this compatibility at runtime. Do not put `--no-sandbox` in `browser.extraFlags`.

## Resource Notes

Chrome in containers usually needs:

- larger shared memory, such as `--shm-size=2g`
- enough RAM for your tab count and workload

For heavier scraping or testing workloads, also consider:

- lowering `instanceDefaults.maxTabs`
- setting block options like `blockImages` in config
- running multiple smaller containers instead of one oversized browser

## Multi-Instance In Containers

You can run orchestrator mode inside one container and start managed instances from the API, but many teams prefer one browser service per container because:

- lifecycle is simpler
- container-level resource limits are clearer
- restart behavior is easier to reason about

Choose based on whether you want container-level isolation or PinchTab-managed multi-instance orchestration.

## CloakBrowser In Docker (local image only)

The published `pinchtab/pinchtab` and `ghcr.io/pinchtab/pinchtab` images ship with stock Chromium. They do **not** include CloakBrowser, and PinchTab does not publish a CloakBrowser-bundled image. See [cloakbrowser.md → Licensing](cloakbrowser.md#licensing) for why.

For local testing, the repository contains a self-built Dockerfile that layers CloakBrowser onto the PinchTab image. It is labelled "smoke" because it is intended for local smoke testing and parity validation only — not for distribution or production.

### Build the local CloakBrowser smoke image

```bash
docker build \
  -f tests/tools/docker/cloakbrowser-smoke.Dockerfile \
  -t pinchtab-cloakbrowser:local \
  .
```

This image is local-only:

- it is not pushed to any registry
- it is not produced by `./dev binaries`
- `./dev smoke cloakbrowser` reuses it when present; set `SKIP_BUILD=0` or remove the image to force a rebuild

### Run a CloakBrowser-backed container locally

Create a config that points PinchTab at the in-image CloakBrowser binary
(`"provider": "cloak"` in the config file is equivalent to `--browser cloak`
on the CLI):

```json
{
  "server": {
    "bind": "0.0.0.0",
    "port": "9867",
    "token": "replace-me",
    "stateDir": "/data"
  },
  "browser": {
    "provider": "cloak",
    "binary": "/opt/cloakbrowser/chrome",
    "cloak": {
      "fingerprintSeed": "42069",
      "platform": "linux",
      "timezone": "UTC",
      "locale": "en-US",
      "disableDefaultStealthArgs": true
    }
  },
  "instanceDefaults": {
    "mode": "headless",
    "humanize": true
  },
  "profiles": {
    "baseDir": "/data/profiles",
    "defaultProfile": "default"
  }
}
```

Run it with the config mounted read-only and a persistent profile volume:

```bash
docker run -d \
  --name pinchtab-cloak \
  -p 127.0.0.1:9867:9867 \
  --shm-size=2g \
  -v pinchtab-cloak-data:/data \
  -v "$PWD/pinchtab-cloak.json":/config/pinchtab.json:ro \
  -e PINCHTAB_CONFIG=/config/pinchtab.json \
  pinchtab-cloakbrowser:local
```

The `pinchtab-cloak-data` named volume holds `/data/profiles` across container restarts, so CloakBrowser keeps cookies, local storage, and history between runs. Delete the volume to start clean:

```bash
docker volume rm pinchtab-cloak-data
```

### Headless-only design

The smoke image follows the same headless-only design as the bundled `pinchtab/pinchtab` image: no X11, no Wayland, no Xvfb. Headed CloakBrowser inside Docker is not a supported configuration. If you need a visible browser window for debugging, run PinchTab + CloakBrowser directly on the host — see [headed-mode.md](headed-mode.md) for the manual local setup.

### Related guides

- [cloakbrowser.md](cloakbrowser.md) — full CloakBrowser configuration, fingerprint flags, and troubleshooting
- [attach-chrome.md](attach-chrome.md) — attach to an externally managed CloakBrowser via CDP
- [headed-mode.md](headed-mode.md) — manual headed setup outside the bundled image
- [security.md](security.md) — security model for container and non-local deployments
