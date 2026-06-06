# Setup Subagent Context

You are running the PinchTab **setup** test. Your job is to build PinchTab from source, then verify that the documented "fresh install" user journey works for a chosen browser provider: `pinchtab nav <url>` should auto-create a config, auto-start the server, and navigate — all in one command, no manual `config init`, no manual `pinchtab server`, no `session create`. Execute groups 0-1 against local fixture HTML files.

You will be told the active **`PROVIDER`** when you are launched: `chrome` (default), `cloak`, or `ghost-chrome`. The auto-flow always produces a chrome-flavoured config first; the documented switch to a different provider is part of what this test validates (see "Provider switch" below).

## What to read

1. **PinchTab dev skill**: `skills/pinchtab-dev/SKILL.md` — how to build the project.
2. **PinchTab skill**: `skills/pinchtab/SKILL.md` — full command reference, configuration, and patterns.
3. **Group files**: `tests/optimization-setup/group-00.md` and `tests/optimization-setup/group-01.md` — step definitions and verification markers, written specifically for the setup lane (native binary, no Docker).

## What NOT to read

- `tests/tools/scripts/baseline.sh` — deterministic baseline; reading it defeats the purpose.
- `tests/benchmark/` — separate benchmark lane, not your concern.
- `tests/optimization/` — the Docker-based opt lane; it uses `./scripts/pt`, which does not apply here.

## Environment

- Project root: the git root (run `git rev-parse --show-toplevel` if needed)
- Go, Python 3, Chrome, curl, and jq are available on the host
- No Docker required — everything runs natively
- Fixture HTML files: `tests/tools/fixtures/` (wiki.html, wiki-go.html, articles.html, dashboard.html, etc.)

## Setup

### 1. Build from source

Use `./dev build` as described in the dev skill. The binary is placed at `./pinchtab` in the project root.

### 2. Start fixture HTTP server

The fixture pages need to be served over HTTP. Use Python's built-in server on a free port:

```bash
FIXTURE_PORT=$(python3 -c 'import socket; s=socket.socket(); s.bind(("",0)); print(s.getsockname()[1]); s.close()')
python3 -m http.server $FIXTURE_PORT --directory tests/tools/fixtures --bind 127.0.0.1 &
```

### 3. Point at a throwaway config — DO NOT init it or start the server

The setup test matches the documented first-use journey from the README: a user installs PinchTab and just runs `pinchtab nav <url>`. The CLI auto-creates a config (with a generated token) and auto-starts the server. The setup test verifies that pathway works, so **do not run any of these manually**:

- `./pinchtab config init` — the auto-flow creates the config on first command. Step 0.1 is the test of that.
- `./pinchtab server` — the auto-flow starts it on first command. Step 0.1 is the test of that.
- `./pinchtab session create` — local single-user CLI calls authenticate from the config token directly; sessions are an advanced feature.

What you DO need to do:

```bash
# Isolate from the user's real ~/.pinchtab/config.json by pointing at a throwaway path.
# The path must NOT exist yet — step 0.1 verifies the CLI creates it.
export PINCHTAB_CONFIG=~/.pinchtab/setup-config-$(date +%s).json
```

`~/.pinchtab/` is preferred over `/tmp` because PinchTab tightens parent-directory perms to 0700 on save; on macOS that fails on `/tmp` (root-owned `/private/tmp`). The user's home dir always works.

If a step in groups 0-1 actually requires a different config value, the agent should discover that from a failure and modify the config with `./pinchtab config set <path> <value>`. The setup groups have been written so this should not be necessary — they verify the OOTB auto-flow.

Use `./pinchtab` CLI commands for everything — never use `./scripts/pt` (that is the Docker wrapper) and never use curl against the HTTP API. The native CLI honors `PINCHTAB_TOKEN`, `PINCHTAB_SERVER`, and `PINCHTAB_CONFIG` env vars, so you can override any of them inline (e.g. `PINCHTAB_TOKEN=wrong-token ./pinchtab health` for the auth-rejection check in step 0.4).

## Provider switch (only if `PROVIDER` is not `chrome`)

Step 0.1 always tests the auto-flow with the default provider (chrome). After step 0.1 succeeds and before continuing to the remaining steps, switch the active browser provider using **only documented commands** — including any install steps the docs prescribe. Installing a provider binary is part of what a fresh user actually does, so it is part of what this test measures.

### `cloak`

The CloakBrowser binary is not bundled with PinchTab and must be installed by the user. The documented install path is in `docs/guides/cloakbrowser.md`:

1. Install the CloakBrowser package and fetch the binary using the documented commands (Python or JS path — whichever the docs list).
2. Capture the absolute path the installer reports for the `chrome` executable.
3. Switch PinchTab to the cloak provider and point it at that binary using `./pinchtab config set`:
   - `browsers.default` → `cloak`
   - `browser.binary` → the absolute path from step 2
   - Any other cloak-specific fields the docs say are required (e.g. `browser.cloak.fingerprintSeed`)
4. Re-run a quick sanity nav (`./pinchtab nav http://localhost:$FIXTURE_PORT/`) to confirm the provider switch took effect.

If `pip install cloakbrowser` (or the equivalent JS install) fails — e.g. no network, package missing for this OS/arch, or the binary download errors — record that as a real setup failure with the exact error and the doc reference you followed. **Do not** patch around it (do not `brew install`, do not download from a different source, do not skip to chrome). The point of the test is to surface friction in the documented onboarding path.

### `ghost-chrome`

Uses the host's Chrome installation (same as `chrome`). Switch via `./pinchtab config set browsers.default ghost-chrome` and verify the provider switch took effect with a sanity nav.

### `chrome`

Skip this section — already the default.

## Running steps

Fixture URLs in the setup group files use `http://localhost:$FIXTURE_PORT/` directly — no substitution needed. Export `FIXTURE_PORT` in your shell before running the steps so the literal `$FIXTURE_PORT` in commands resolves correctly.

Execute every step in groups 0 and 1. For each step:

1. Run the appropriate PinchTab commands
2. Verify the expected markers appear in the output
3. Record pass/fail with the command used and output

## Cleanup

When finished:
- Kill the fixture server and PinchTab server.
- Delete the temp config: `rm -f "$PINCHTAB_CONFIG"`.
- Delete the built binary: `rm -f ./pinchtab`.

You should not need to restore anything — the setup test only wrote to `~/.pinchtab/setup-config-<timestamp>.json`, never to the user's real `~/.pinchtab/config.json`.
