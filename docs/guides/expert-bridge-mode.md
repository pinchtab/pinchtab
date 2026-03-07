# Expert Guide: Bridge Mode

This guide is for advanced users who want to run `pinchtab bridge` directly.

For most users, the right entrypoint is still:

```bash
pinchtab
```

Use bridge mode only when you explicitly want the single-instance runtime without the full server control plane.

## What Bridge Mode Is

`pinchtab bridge` starts the single-instance HTTP runtime.

It:
- wraps one browser
- exposes the browser/tab API directly
- does not serve the dashboard
- does not manage multiple instances
- does not act as the main control plane

In normal managed mode, the server spawns bridge children for you.

## When To Use It

Bridge mode makes sense when:
- you want one browser runtime only
- you do not need profiles/instance orchestration in the parent process
- you want to debug the single-instance runtime directly
- you want to run the bridge as a standalone worker

## When Not To Use It

Do not use bridge mode as the primary onboarding path if your real goal is:
- “give my agent a local browser service”
- “replace an embedded browser runtime in another tool”
- “manage profiles and instances from one endpoint”

In those cases, use:

```bash
pinchtab
```

## Minimal Example

```bash
pinchtab bridge
```

Then call the single-instance API on the configured port:

```bash
curl -X POST http://localhost:9867/navigate \
  -H "Content-Type: application/json" \
  -d '{"url":"https://pinchtab.com"}'

curl http://localhost:9867/snapshot?filter=interactive
```

## Configuration Model

Bridge mode still uses the same `config.json` and operational env surface:

```bash
PINCHTAB_PORT=9868 pinchtab bridge
PINCHTAB_TOKEN=secret pinchtab bridge
CHROME_BIN=/usr/bin/google-chrome pinchtab bridge
```

Behavior defaults still come from config file sections like:
- `browser`
- `instanceDefaults`
- `security`
- `timeouts`

## Mental Model

```text
pinchtab bridge
  -> one runtime
  -> one browser
  -> many tabs
```

Compared with full server mode:

```text
pinchtab
  -> one control plane
  -> many instances
  -> each managed instance may run through a bridge
```

## Tradeoffs

Benefits:
- fewer product layers visible to you
- direct access to the single-instance API
- useful for debugging and specialized deployments

Costs:
- no instance pool
- no profile/instance dashboard workflow
- no attach/launch control plane
- you are choosing the lower-level runtime, not the main product entrypoint

## Recommended Rule

Use bridge mode when you explicitly want the worker runtime.
Use server mode when you want Pinchtab as a browser service.
