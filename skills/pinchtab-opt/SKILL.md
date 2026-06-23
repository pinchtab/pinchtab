---
name: pinchtab-opt
description: "Run the PinchTab optimization loop (Docker, 3 blind subagents on the runner's HIGH model, 108 steps across 47 groups) against chrome, cloak, ghost-chrome, or all three providers. Pass `setup` (optionally followed by a provider or `all`) to run only the setup test (native binary, single subagent forced to the runner's LOW model) that validates the fresh-install OOTB flow per provider. Use when asked to 'run optimization', 'run the opt loop', 'benchmark the agent', '/pinchtab-opt', '/pinchtab-opt cloak', '/pinchtab-opt ghost-chrome', '/pinchtab-opt setup', '/pinchtab-opt setup all', or 'test pinchtab agent'."
---

# PinchTab Optimization Loop

Two independent modes selected by the argument. They use different runtimes, different models, and answer different questions — only one runs per invocation.

Think of the arg surface as a matrix: **mode × provider**. Model role is fixed by mode (not user-selectable).

| Mode | Providers | Runtime | Model role | Asks |
|---|---|---|---|---|
| **Optimization** (default) | `chrome` (default), `cloak`, `ghost-chrome`, `all` | Docker, 3 parallel subagents | `HIGH` (default/strong) | how few browser ops does the agent need across 108 steps vs baseline |
| **Setup** (`setup` keyword) | `chrome` (default), `cloak`, `ghost-chrome`, `all` | native binary, 1 subagent | `LOW` (small/fast) | can an agent go zero→working from the skill docs alone (OOTB doc-quality gate) |

## Model roles

This skill names model tiers abstractly so any runner (Claude, OpenAI, …) can map them at launch time:

- **`LOW`** — small/fast/cheap model. Used by the setup test because a weak model passing is the actual doc-quality signal; a strong model passing is unsurprising.
- **`HIGH`** — the runner's default/strong model. Used by the optimization benchmark because we want the realistic agent performance, not a deliberately handicapped run.

Suggested mappings (pick whatever the runner has available at the time it executes):

| Runner | `LOW` | `HIGH` |
|---|---|---|
| Claude Code | Haiku (e.g. `claude-haiku-4-5`) | inherit parent (Opus / Sonnet) |
| OpenAI Agents | `gpt-*-mini` tier | `gpt-*` flagship tier |
| Other | smallest capable model | default/best model |

The thresholds below were calibrated for Claude Haiku 4.5 as `LOW`; if you use a different `LOW`, recalibrate the token / tool-call numbers on the first run.

## Argument Parsing

`/pinchtab-opt [setup] [chrome|cloak|ghost-chrome|all]`

Positional args, in order. The first token is either a provider (optimization mode) or the literal `setup` keyword (setup mode); if `setup`, the second token is the provider.

**Optimization mode** (default — no `setup` keyword):
- `/pinchtab-opt` → opt on chrome
- `/pinchtab-opt chrome` → opt on chrome
- `/pinchtab-opt cloak` → opt on CloakBrowser
- `/pinchtab-opt ghost-chrome` → opt on ghost-chrome (Chrome image, ghost-chrome config)
- `/pinchtab-opt all` → opt on chrome, then cloak, then ghost-chrome

**Setup mode** (when first token is `setup`):
- `/pinchtab-opt setup` → setup on chrome (default)
- `/pinchtab-opt setup chrome` → setup on chrome
- `/pinchtab-opt setup cloak` → setup on cloak
- `/pinchtab-opt setup ghost-chrome` → setup on ghost-chrome
- `/pinchtab-opt setup all` → setup on each of the three, in order

Legacy `both` is **removed** (no alias) — use `all` for multi-provider runs. Anything else → print this section and abort.

## Path Resolution

All paths are relative to the **project root** (git root):

```bash
PROJECT_ROOT=$(git rev-parse --show-toplevel)
TOOLS_DIR="$PROJECT_ROOT/tests/tools"
OPT_DIR="$PROJECT_ROOT/tests/optimization"
SETUP_DIR="$PROJECT_ROOT/tests/optimization-setup"
```

The optimization subagents must run with `$TOOLS_DIR` as their working directory because `./scripts/pt` and `./scripts/runner` live there. The setup subagent runs with `$PROJECT_ROOT` as its working directory and builds a native binary.

`up.sh` / `down.sh` live in `$OPT_DIR`.

---

# Mode: setup (`/pinchtab-opt setup`)

Validate that an AI agent can go from zero to working with PinchTab using only the skill docs — no hand-holding.

## Clean slate

The setup test simulates a true first-install OOTB experience. To get there:

1. **Stop any pre-existing server** — first try the recorded PID, then fall back to `pkill -f` for anything spawned outside the PID file's tracking. Killing via PID file is more reliable than `pkill -f` (which can miss processes and won't reap dashboard children).
2. **Stash the user's real `~/.pinchtab/` aside** — so the auto-flow truly creates a config from zero, not on top of an existing profile/activity history that warms Chrome and confuses results. Restored automatically on completion via a trap.
3. **Free port 9867 and remove the stale binary** in the project root.

```bash
# 1. Stop any prior server (PID-file first, then pkill fallback)
if [ -f ~/.pinchtab/server.pid ]; then
  prior_pid=$(jq -r '.pid // empty' ~/.pinchtab/server.pid 2>/dev/null)
  [ -n "$prior_pid" ] && kill "$prior_pid" 2>/dev/null
fi
docker compose -f "$TOOLS_DIR/docker-compose.yml" down 2>/dev/null
docker rm -f optimization-pinchtab >/dev/null 2>&1 || true
pkill -f 'pinchtab' 2>/dev/null
pkill -f 'Google Chrome.*pinchtab' 2>/dev/null
lsof -ti:9867 2>/dev/null | xargs kill 2>/dev/null
sleep 2

# 2. Stash the real ~/.pinchtab aside for the duration of the test
PINCHTAB_BACKUP="$HOME/.pinchtab.backup-$(date +%s)"
if [ -d ~/.pinchtab ]; then
  mv ~/.pinchtab "$PINCHTAB_BACKUP"
fi
# Always restore on exit, even on failure or Ctrl-C
trap '
  if [ -d "'"$PINCHTAB_BACKUP"'" ]; then
    rm -rf ~/.pinchtab 2>/dev/null
    mv "'"$PINCHTAB_BACKUP"'" ~/.pinchtab
  fi
' EXIT INT TERM

# 3. Misc state
rm -f ~/.local/state/pinchtab/current-tab 2>/dev/null
rm -f "$PROJECT_ROOT/pinchtab" 2>/dev/null
# Defensive: clear any stray *config*.json the agent might leave in a real ~/.pinchtab
# (no-op because we stashed it above — but kept for runs that skip the stash).
find ~/.pinchtab -maxdepth 1 -name '*config*.json' ! -name 'config.json' -delete 2>/dev/null
```

The setup test uses `PINCHTAB_CONFIG=~/.pinchtab/setup-config-<timestamp>.json` so even without the stash it never touches the user's real `~/.pinchtab/config.json`. The stash adds true first-install fidelity (no warmed Chrome profile, no activity history) and the trap guarantees the real `~/.pinchtab/` is restored regardless of how the run ends.

Wait 2 seconds after cleanup before spawning the agent.

## Spawn the setup subagent (LOW model) — for each requested provider

If the provider arg is `all`, repeat this section once per provider in order: `chrome`, `cloak`, `ghost-chrome`. Otherwise run it exactly once for the single named provider (default `chrome`).

Spawn a single subagent on the runner's **`LOW`** model (see "Model roles" above). For Claude Code that means `model: "haiku"` in the Agent tool call; for other runners pick the equivalent small/fast tier. Setup is a doc-quality test: if the `LOW` model can complete 11/11 from the SKILL docs alone, the onboarding flow is genuinely OOTB-ready. A `HIGH` model passing is unsurprising and not the signal we want — do **not** let the subagent inherit the parent's default model.

Use the prompt below. Replace `{PROJECT_ROOT}`, `{TIMESTAMP}`, and `{PROVIDER}` with actual values.

```
You are running a PinchTab setup validation against PROVIDER={PROVIDER}.
Your working directory is {PROJECT_ROOT}.

Start by reading the context file, then follow its instructions:

1. Read `tests/optimization-setup/subagent-context.md` — your full instructions, including the "Provider switch" section that applies when PROVIDER is not `chrome`.
2. Read the skill files it references.
3. Read the group files it references.
4. Execute all steps in groups 0 and 1 against PROVIDER={PROVIDER}.

Report pass/fail for every step. Write your full results to `/tmp/pinchtab-setup-{PROVIDER}-{TIMESTAMP}.md`.
```

## Interpret the result

- **11/11 PASS**: the skill docs are sufficient for a fresh-install start.
- **Any failure**: a gap in the skill docs or the CLI ergonomics.

Key things to look for in the report:
- Did the agent use the default port (9867) or pick a custom one?
- Did the agent read the server's READY output or poll health in a loop?
- Did the agent use `./pinchtab` CLI or fall back to curl/HTTP API?
- Did the agent leave `~/.pinchtab/config.json` untouched and run from a `PINCHTAB_CONFIG=~/.pinchtab/setup-config-*.json` throwaway path?
- Did the agent **avoid** running `./pinchtab config init`, `./pinchtab server`, and `./pinchtab session create`? The auto-flow on the first `nav` should handle all three.
- Did step 0.1 (cold nav) auto-create the config AND auto-start the server in a single command?
- Did step 0.5 (IDPI rejection) get a clean `idpi_domain_blocked`-style error against `https://example.com`?
- Did step 1.2 (click follows link) pass without an eval workaround?
- Did step 1.5 (fill+press login) reach `VERIFY_LOGIN_SUCCESS_DASHBOARD`?

Setup thresholds (calibrated for Claude Haiku 4.5 as `LOW` — recalibrate the numeric rows on the first run if your `LOW` is a different model):

| Metric | Good | Needs work |
|--------|------|------------|
| Total tokens | < 60k | > 80k |
| Tool calls | < 50 | > 60 |
| Port | 9867 (default) | Custom port |
| Server wait | Read READY | Polled health |
| API usage | CLI only | curl/HTTP fallback |

The setup subagent cleans up after itself (kills the fixture + native server, deletes the temp config and built binary).

---

# Mode: optimization (`/pinchtab-opt [chrome|cloak|ghost-chrome|all]`)

Run blind subagents against 108 browser automation steps (47 groups) to measure how well an AI agent can drive PinchTab without hand-held selectors.

## Prerequisites

Stop any native PinchTab server that might occupy port 9867, then confirm Docker is running:

```bash
pkill -f 'pinchtab server' 2>/dev/null || true
pkill -f 'pinchtab.*serve' 2>/dev/null || true
lsof -ti:9867 2>/dev/null | xargs kill -9 2>/dev/null || true
sleep 1
docker info >/dev/null 2>&1 || { echo "Docker not running"; exit 1; }
```

## For each requested provider

If the provider arg is `all`, repeat steps 1–5 below in order for `chrome`, then `cloak`, then `ghost-chrome`. Otherwise run them exactly once for the single named provider (default `chrome`). `up.sh` accepts all three provider names; `down.sh` is shared (it removes the standalone `optimization-pinchtab` container and the chrome compose stack regardless of which provider was active).

### 1. Bring up the provider environment

```bash
"$OPT_DIR/up.sh" "$PROVIDER"
```

This returns `READY` with `container=...` and `token=...`. Capture those values.

### 2. Seed isolated per-agent report files

So concurrent agents don't corrupt each other's JSON:

```bash
RESULTS_DIR="$TOOLS_DIR/../benchmark/results"
TIMESTAMP=$(date -u +%Y%m%d_%H%M%S)
mkdir -p "$RESULTS_DIR"

for agent in A B C; do
  cat > "$RESULTS_DIR/agent${agent}_${PROVIDER}_${TIMESTAMP}.json" <<SEED
{
  "benchmark": {"type": "pinchtab", "provider": "${PROVIDER}", "timestamp": "${TIMESTAMP}", "agent": "${agent}"},
  "totals": {"steps_answered": 0},
  "steps": []
}
SEED
done
```

Save the three report file paths — you pass the correct one to each subagent.

### 3. Spawn 3 parallel subagents

Use the **Agent** tool with `run_in_background: true`. Split the 47 groups into three batches:

- **Batch A**: groups 0–14 (45 steps)
- **Batch B**: groups 15–29 (30 steps)
- **Batch C**: groups 30–46 (33 steps)

Each subagent receives a provider-aware prompt (replace the placeholders):

```
You are running PinchTab optimization tasks against PROVIDER={PROVIDER}.

Your job is to execute groups {START} through {END}.

CRITICAL ENVIRONMENT SETUP (do this immediately):
export PINCHTAB_CONTAINER={CONTAINER_NAME}
export PINCHTAB_TOKEN={TOKEN}

CRITICAL: Your working directory MUST be {PROJECT_ROOT}/tests/tools for all commands.
Always prefix shell commands with the exports + cd when needed, e.g.:
  PINCHTAB_CONTAINER={CONTAINER_NAME} PINCHTAB_TOKEN={TOKEN} ./scripts/pt ...

Your report file is: {REPORT_FILE}
Use `--report-file {REPORT_FILE}` on every `./scripts/runner step-end` call.

Start by reading these files to understand your tools and tasks:
1. Read `{PROJECT_ROOT}/tests/optimization/subagent-context.md` — environment, wrapper, recording format, and the Provider & Environment section.
2. Read `{PROJECT_ROOT}/skills/pinchtab/SKILL.md` — full PinchTab command reference.
3. Read each group file from `{PROJECT_ROOT}/tests/optimization/group-{START_PAD}.md` through `{PROJECT_ROOT}/tests/optimization/group-{END_PAD}.md`.

DO NOT read `{PROJECT_ROOT}/tests/tools/scripts/baseline.sh` or any file under `{PROJECT_ROOT}/tests/benchmark/`.

After reading the above files, execute each step in each group sequentially:
- Use the exported PINCHTAB_CONTAINER / TOKEN on every ./scripts/pt call (or keep them in your environment).
- After each step, record the result with:
    ./scripts/runner step-end --report-file {REPORT_FILE} <group> <step> answer "<observation>" pass "notes"
  (or fail / skip as appropriate).
- Use your judgment to figure out the right PinchTab commands from the skill doc. The group files describe WHAT to do, not HOW.

Work through every step in groups {START}-{END}. Do not skip any.
```

When launching the subagent, also tell it the concrete values for `{CONTAINER_NAME}`, `{TOKEN}`, and `{PROVIDER}`.

### 4. Monitor progress

While agents run, periodically count step-end recordings:

```bash
grep -c "step-end" <output_file>
```

Expected totals: Batch A ~45, Batch B ~30, Batch C ~33 = 108 total per provider.

### 5. Tear down + collect results

Once all 3 agents complete, the Agent tool returns each subagent's output file path — save all three as `TRANSCRIPT_A`, `TRANSCRIPT_B`, `TRANSCRIPT_C`.

```bash
"$OPT_DIR/down.sh"

SKILL_DIR="$PROJECT_ROOT/skills/pinchtab-opt"
MERGED="$RESULTS_DIR/merged_${PROVIDER}_${TIMESTAMP}.json"

# Merge the three agent reports into one JSON (strip non-JSON header lines)
cd "$TOOLS_DIR" && \
  ./scripts/runner opt merge-reports \
    "$RESULTS_DIR/agentA_${PROVIDER}_${TIMESTAMP}.json" \
    "$RESULTS_DIR/agentB_${PROVIDER}_${TIMESTAMP}.json" \
    "$RESULTS_DIR/agentC_${PROVIDER}_${TIMESTAMP}.json" \
  2>/dev/null | grep -v '^Loaded\|^Merged' > "$MERGED"

# Inject token usage from the subagent JSONL transcripts
./scripts/runner opt inject-usage \
  -r "$MERGED" \
  "$TRANSCRIPT_A" "$TRANSCRIPT_B" "$TRANSCRIPT_C"

# Print the comparison table for this provider — present this output as-is
./scripts/runner opt summarize \
  -r "$MERGED" \
  -b "$SKILL_DIR/baseline-ref.json" \
  "$TRANSCRIPT_A" "$TRANSCRIPT_B" "$TRANSCRIPT_C"
```

The `-b` flag loads stored reference timing and ops from `baseline-ref.json` so the Baseline column is fully populated. The transcripts enable the Browser ops and Ops/step rows.

If running `all`, repeat steps 1–5 for each remaining provider, then present chrome / cloak / ghost-chrome side by side in the final summary.

## Reference Numbers

- **Baseline**: 108/108 steps, 272 ops, ~49s total, 2.5 ops/step (stored in `baseline-ref.json`)
- **Expected agent range**: 250–400 browser ops, 2.5–4 ops/step
- **Group count**: 47 groups (`group-00` … `group-46`), 108 total steps

---

## File Locations (relative to project root)

| Path | Purpose |
|------|---------|
| `tests/optimization-setup/subagent-context.md` | Setup subagent instructions (native build, OOTB flow) |
| `tests/optimization-setup/group-00.md` … `group-01.md` | Setup task descriptions |
| `tests/optimization/subagent-context.md` | Optimization subagent instructions (env, wrapper, recording) |
| `tests/optimization/index.md` | Optimization group listing |
| `tests/optimization/group-00.md` … `group-46.md` | Optimization task descriptions |
| `tests/optimization/up.sh` | Provider-aware setup (chrome vs cloak) |
| `tests/optimization/down.sh` | Tear down after a provider run |
| `skills/pinchtab/SKILL.md` | PinchTab command reference (read by subagents) |
| `skills/pinchtab-dev/SKILL.md` | Build instructions (read by the setup subagent) |
| `tests/tools/scripts/pt` | PinchTab Docker wrapper (CWD must be `tests/tools`) |
| `tests/tools/scripts/runner` | Step recorder (CWD must be `tests/tools`) |
| `tests/tools/scripts/baseline.sh` | Baseline (subagents must NOT read this) |
| `skills/pinchtab-opt/baseline-ref.json` | Stored baseline timing/ops reference for the table |
