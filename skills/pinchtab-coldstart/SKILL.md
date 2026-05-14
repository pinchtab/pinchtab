---
name: pinchtab-coldstart
description: "Run the PinchTab cold-start test. Spawns a subagent that follows tests/coldstart/subagent-context.md to validate the documented first-install user journey. Use when asked to 'run cold start', 'cold-start test', or 'test the agent onboarding flow'."
---

# PinchTab Cold-Start Test

Validate that an AI agent can go from zero to working with PinchTab using only the skill docs — no hand-holding.

## Prerequisites

Before spawning the subagent, clean the environment:

```bash
PROJECT_ROOT=$(git rev-parse --show-toplevel)
# Stop Docker benchmark services if running — they bind port 9867 and
# the cold-start native server will collide with them.
docker compose -f "$PROJECT_ROOT/tests/tools/docker-compose.yml" down 2>/dev/null
pkill -f 'pinchtab' 2>/dev/null
pkill -f 'Google Chrome.*pinchtab' 2>/dev/null
sleep 2
rm -f ~/.local/state/pinchtab/current-tab 2>/dev/null
rm -f "$PROJECT_ROOT/pinchtab" 2>/dev/null
rm -f ~/.pinchtab/coldstart-config-*.json 2>/dev/null
lsof -ti:9867 2>/dev/null | xargs kill 2>/dev/null
```

The cold-start uses `PINCHTAB_CONFIG=~/.pinchtab/coldstart-config-<timestamp>.json` so it never touches the user's real `~/.pinchtab/config.json`. Stale coldstart-config files from previous runs are removed above so `./pinchtab config init` writes a truly fresh OOTB config every time.

Wait 2 seconds after cleanup before spawning the agent.

## Execution

Spawn a single subagent **on Haiku 4.5** (`model: "haiku"` in the Agent tool call). Cold-start is a doc-quality test: if Haiku can complete 11/11 from the SKILL docs alone, the onboarding flow is genuinely cold-start-ready. Opus passing is unsurprising and not the signal we want. Do not let the subagent inherit Opus from the parent.

Use the prompt below. Replace `{PROJECT_ROOT}` and `{TIMESTAMP}` with actual values.

```
You are running a PinchTab cold-start validation. Your working directory is {PROJECT_ROOT}.

Start by reading the context file, then follow its instructions:

1. Read `tests/coldstart/subagent-context.md` — your full instructions.
2. Read the skill files it references.
3. Read the group files it references.
4. Execute all steps in groups 0 and 1.

Report pass/fail for every step. Write your full results to `/tmp/pinchtab-coldstart-{TIMESTAMP}.md`.
```

## Interpreting results

- **11/11 PASS**: The skill docs are sufficient for a cold start.
- **Any failure**: Check what the agent got stuck on — that's a gap in the skill docs or the CLI ergonomics.

Key things to look for in the report:
- Did the agent use the default port (9867) or pick a custom one?
- Did the agent read the server's READY output or poll health in a loop?
- Did the agent use `./pinchtab` CLI or fall back to curl/HTTP API?
- Did the agent leave `~/.pinchtab/config.json` untouched and run from a `PINCHTAB_CONFIG=~/.pinchtab/coldstart-config-*.json` throwaway path?
- Did the agent **avoid** running `./pinchtab config init`, `./pinchtab server`, and `./pinchtab session create`? The auto-flow on the first `nav` should handle all three. If the agent ran any of them, the cold-start docs failed to communicate that the auto-flow is the test.
- Did step 0.1 (cold nav) auto-create the config AND auto-start the server in a single command?
- Did step 0.5 (IDPI rejection) get a clean `idpi_domain_blocked`-style error against `https://example.com`?
- Did step 1.2 (click follows link) pass without an eval workaround?
- Did step 1.5 (fill+press login) reach `VERIFY_LOGIN_SUCCESS_DASHBOARD`?

## Comparing runs

Track token usage and tool call counts across runs to measure improvements. Thresholds are calibrated for Haiku 4.5 — recalibrate if you switch models.

| Metric | Good | Needs work |
|--------|------|------------|
| Total tokens | < 60k | > 80k |
| Tool calls | < 50 | > 60 |
| Port | 9867 (default) | Custom port |
| Server wait | Read READY | Polled health |
| API usage | CLI only | curl/HTTP fallback |

(Earlier runs on Opus 4.7 came in around 45-53k tokens / 29-38 tool calls. Haiku tends to use more tool calls for the same task; expect higher numbers and adjust thresholds after the first few Haiku runs.)
