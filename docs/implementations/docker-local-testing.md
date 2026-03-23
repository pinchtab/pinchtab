# Docker Local Testing

This page is a practical checklist for testing the current Docker setup locally.

It covers two paths:

- the default managed-config flow, where the container owns `/data/.config/pinchtab/config.json`
- the explicit-config flow, where you mount your own `config.json` and set `PINCHTAB_CONFIG`

## Managed Config Flow

Build and start the local Compose service:

```bash
docker compose up --build -d
docker compose logs -f pinchtab
```

Inspect the effective config path and persisted config:

```bash
docker exec pinchtab pinchtab config path
docker exec pinchtab sh -lc 'cat /data/.config/pinchtab/config.json'
```

Expected results:

- the config path is `/data/.config/pinchtab/config.json`
- `server.bind` in the persisted config remains `127.0.0.1`
- a token is present if one was generated on first boot or passed in

Verify the config bind address:

```bash
docker exec pinchtab pinchtab config get server.bind
```

Expected result: `0.0.0.0` (set by entrypoint on first boot)

Verify persistence across restart:

```bash
docker compose down
docker compose up -d
docker exec pinchtab sh -lc 'cat /data/.config/pinchtab/config.json'
```

## Explicit `PINCHTAB_CONFIG` Flow

Create a local config file, for example `./tmp/config.json`:

```json
{
  "server": {
    "bind": "0.0.0.0",
    "port": "9867",
    "token": "local-test-token"
  }
}
```

Run the container with that config mounted read-only:

```bash
docker run --rm -d \
  --name pinchtab-test \
  -p 127.0.0.1:9867:9867 \
  -e PINCHTAB_CONFIG=/config/config.json \
  -v "$PWD/tmp/config.json:/config/config.json:ro" \
  -v pinchtab-data:/data \
  --shm-size=2g \
  pinchtab/pinchtab
```

Verify the explicit config path and auth:

```bash
docker exec pinchtab-test pinchtab config path
docker exec pinchtab-test sh -lc 'cat /config/config.json'
curl -H 'Authorization: Bearer local-test-token' http://127.0.0.1:9867/health
```

Expected results:

- `pinchtab config path` reports `/config/config.json`
- the mounted file is used as-is
- the container entrypoint does not rewrite the custom config

## What To Check When Something Fails

Container logs:

```bash
docker logs pinchtab
docker logs pinchtab-test
```

Config path:

```bash
docker exec pinchtab pinchtab config path
docker exec pinchtab-test pinchtab config path
```

Persisted config content:

```bash
docker exec pinchtab sh -lc 'cat /data/.config/pinchtab/config.json'
```

## Current Caveat

The Docker runtime path owns `--no-sandbox` compatibility now. Do not put it in `browser.extraFlags`.

If Chromium still fails to launch in an unusual Linux container environment, set:

```bash
PINCHTAB_CHROME_NO_SANDBOX=1
```

Use that environment variable instead of saving `--no-sandbox` into config.
