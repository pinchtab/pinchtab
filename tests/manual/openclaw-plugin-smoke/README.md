# OpenClaw + Pinchtab Docker Mock Test

Deterministic end-to-end harness for the Pinchtab OpenClaw plugin.

What it does:
- builds a local Pinchtab image from this repo
- starts a local fixture server inside Docker
- **simulates the plugin release flow**: runs `npm pack` against `plugin/` (which triggers the same `prepack` chain — clean → sync skills → `tsc` build → emit `dist/` — that runs during a real `npm publish`), then installs the resulting tarball into the OpenClaw container exactly as a downstream consumer would
- disables the bundled OpenClaw browser plugin so `browser` resolves to Pinchtab's compatibility alias
- runs several `openclaw agent` prompts against the fixture site
- verifies the returned answers
- verifies fixture access logs show real browser traffic from Pinchtab
- proves same-agent session reuse (alpha) and per-agent tab independence (beta) via dedicated smoke turns

Because the plugin is consumed from a packed tarball (not via `--link`), this harness catches release-blocking issues — missing compiled output, wrong `openclaw.extensions` path, files-allowlist gaps — before they reach `clawhub package publish` in CI.

## Requirements

This is a **manual** smoke test — it expects to be run on a developer machine that already has OpenClaw installed and signed in. It does not bootstrap auth itself.

- Docker
- `openclaw` CLI installed locally and on `PATH` (used to detect the version pinned for the in-container install, and as the source of auth state)
- valid OpenClaw auth in `~/.openclaw/agents/main/agent/auth-profiles.json`
- a usable `~/.openclaw/openclaw.json`

If `openclaw` is not available, you can still run the smoke by pinning the version explicitly and pointing at an alternate state directory:

```bash
OPENCLAW_VERSION=2026.5.2 \
OPENCLAW_STATE_SOURCE=/path/to/openclaw-state \
./tests/manual/openclaw-plugin-smoke/run.sh
```

## Run

```bash
./tests/manual/openclaw-plugin-smoke/run.sh
# or
./dev e2e smoke-plugin
```

### With an Anthropic API key (no host openclaw state required)

```bash
./dev e2e smoke-plugin --anthropic-key sk-ant-...
# or
./tests/manual/openclaw-plugin-smoke/run.sh --anthropic-key sk-ant-...
# or via env
ANTHROPIC_API_KEY=sk-ant-... ./dev e2e smoke-plugin --anthropic-key "$ANTHROPIC_API_KEY"
```

In this mode the runner bootstraps a fresh OpenClaw config inside the container via `openclaw onboard --auth-choice anthropic-api-key`, so you don't need a local `openclaw` install or `~/.openclaw` state.

Optional overrides:

```bash
OPENCLAW_STATE_SOURCE=$HOME/.openclaw \
OPENCLAW_VERSION=2026.5.2 \
./tests/manual/openclaw-plugin-smoke/run.sh
```

## Artifacts

The script copies results into `tests/manual/openclaw-plugin-smoke/artifacts/<timestamp>/`:

- `summary.json` — scenario results + log checks, including `sessionProof`
- `agent-*.json` — raw OpenClaw CLI output per prompt
- `gateway.log` — OpenClaw gateway log
- `plugin-install.log` — plugin install log
- `fixtures-access.log` — JSONL access log from the fixture server
- `docker-compose.log` — combined compose output

## Why this proves Pinchtab was used

Two layers:

1. The bundled OpenClaw browser plugin is disabled, while the Pinchtab plugin re-registers `browser`.
2. The fixture log must show Chrome/Chromium-style traffic for the exercised pages, including a JS-driven cookie/state flow that plain `web_fetch` cannot satisfy.
