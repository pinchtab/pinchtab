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

For production deployments, use Docker secrets instead of env vars for `PINCHTAB_TOKEN`:

```bash
# Create a secret
echo "your-secret-token" | docker secret create pinchtab_token -

# Use it in docker-compose.yml
services:
  pinchtab:
    image: pinchtab/pinchtab
    secrets:
      - pinchtab_token
    environment:
      PINCHTAB_TOKEN_FILE: /run/secrets/pinchtab_token
    # ... rest of config
```

Or with `docker run`:

```bash
docker run -d \
  --name pinchtab \
  --secret pinchtab_token \
  -e PINCHTAB_TOKEN_FILE=/run/secrets/pinchtab_token \
  pinchtab/pinchtab
```

Secrets are mounted read-only and never appear in `docker ps` or logs.

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
