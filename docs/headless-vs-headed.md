# Headless vs Headed

PinchTab instances can run Chrome in two modes:

- **Headless**: no visible browser window
- **Headed**: visible browser window

You usually run one server with `pinchtab`, then start instances in either mode through the API or CLI.

---

## Headless mode

Headless is the default mode for managed instances.

```bash
curl -X POST http://localhost:9867/instances/start \
  -H "Content-Type: application/json" \
  -d '{"mode":"headless"}'
# CLI Alternative
pinchtab instance start
# Response
{
  "id": "inst_0a89a5bb",
  "profileId": "prof_278be873",
  "profileName": "instance-1741400000000000000",
  "port": "9868",
  "mode": "headless",
  "headless": true,
  "status": "starting"
}
```

### Good fit for

- agents and automation
- CI and batch jobs
- containers and remote servers
- higher-throughput workloads

### Tradeoffs

- no visible browser window
- debugging usually happens through snapshots, screenshots, text extraction, and logs

---

## Headed mode

Headed mode launches a visible Chrome window.

```bash
curl -X POST http://localhost:9867/instances/start \
  -H "Content-Type: application/json" \
  -d '{"mode":"headed"}'
# CLI Alternative
pinchtab instance start --mode headed
# Response
{
  "id": "inst_1b9a5dcc",
  "profileId": "prof_278be873",
  "profileName": "instance-1741400000000000001",
  "port": "9869",
  "mode": "headed",
  "headless": false,
  "status": "starting"
}
```

### Good fit for

- development
- debugging
- local testing
- visual verification
- human-in-the-loop workflows

### Tradeoffs

- requires a display environment
- usually uses more CPU and memory than headless mode

---

## Side-by-side comparison

| Aspect | Headless | Headed |
|---|---|---|
| Visibility | No window | Visible window |
| Debugging | Snapshot- and log-based | Direct visual feedback |
| Display required | No | Yes |
| CI use | Strong fit | Usually poor fit |
| Local development | Fine | Usually easier |
| Resource usage | Lower | Higher |

---

## Recommended workflows

## Development workflow

Use a visible browser while you are building and validating the flow:

```bash
curl -X POST http://localhost:9867/instances/start \
  -H "Content-Type: application/json" \
  -d '{"mode":"headed"}'
# CLI Alternative
pinchtab instance start --mode headed
```

When you need persistence, create a profile first:

```bash
curl -X POST http://localhost:9867/profiles \
  -H "Content-Type: application/json" \
  -d '{"name":"dev"}'
# Response
{
  "status": "created",
  "id": "prof_278be873",
  "name": "dev"
}
```

Then launch the profile in headed mode:

```bash
curl -X POST http://localhost:9867/instances/start \
  -H "Content-Type: application/json" \
  -d '{"profileId":"prof_278be873","mode":"headed"}'
# CLI Alternative
pinchtab instance start --profile prof_278be873 --mode headed
# Response
{
  "id": "inst_ea2e747f",
  "profileId": "prof_278be873",
  "profileName": "dev",
  "port": "9868",
  "mode": "headed",
  "headless": false,
  "status": "starting"
}
```

## Production workflow

Use headless mode for repeatable unattended work:

```bash
for i in 1 2 3; do
  curl -s -X POST http://localhost:9867/instances/start \
    -H "Content-Type: application/json" \
    -d '{"mode":"headless"}' | jq .
done
# CLI Alternative
for i in 1 2 3; do
  pinchtab instance start
done
```

---

## Inspecting a headless instance

You can debug a headless instance through tab APIs.

List the instance tabs:

```bash
curl http://localhost:9867/instances/inst_0a89a5bb/tabs | jq .
# Response
[
  {
    "id": "CDP_TARGET_ID",
    "instanceId": "inst_0a89a5bb",
    "url": "https://pinchtab.com",
    "title": "PinchTab"
  }
]
```

Then inspect the tab:

```bash
curl http://localhost:9867/tabs/CDP_TARGET_ID/snapshot | jq .
```

```bash
curl http://localhost:9867/tabs/CDP_TARGET_ID/text | jq .
```

```bash
curl http://localhost:9867/tabs/CDP_TARGET_ID/screenshot > page.jpg
```

---

## Display requirements

Headed instances need a display environment.

### macOS

Headed mode works with the native desktop session.

### Linux

Headless works anywhere.
Headed mode needs X11 or Wayland.

```bash
ssh -X user@server 'pinchtab instance start --mode headed'
```

### Windows

Windows builds are available, but Windows support is currently limited and best-effort.
Headed mode targets the native desktop session.
Prefer direct local runs with `pinchtab server` or `pinchtab bridge`; the daemon workflow is not the primary Windows path.

### Docker

Headless is the normal choice in containers:

```bash
docker run -d -p 9867:9867 pinchtab/pinchtab
curl -X POST http://localhost:9867/instances/start \
  -H "Content-Type: application/json" \
  -d '{"mode":"headless"}'
```

---

## Dashboard

The dashboard lets you monitor running instances and profiles while you use either mode.

Useful views:

- Monitoring: running instances, status, tabs, and optional memory metrics
- Profiles: saved profiles, launch actions, and live details
- Settings: runtime and dashboard preferences

---

## Summary

- Use **headless** for unattended automation and scale.
- Use **headed** for local debugging and human-visible workflows.
- Choose the mode per instance, not per server.
