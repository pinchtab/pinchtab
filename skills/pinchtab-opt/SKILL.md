---
name: pinchtab-opt
description: "Run the PinchTab optimization loop. Spawns blind subagents that execute 99 browser automation steps across 45 groups using only the PinchTab skill, then reports pass/fail results and operation counts vs baseline. Use when asked to 'run optimization', 'run the opt loop', 'benchmark the agent', or 'test pinchtab agent'."
---

# PinchTab Optimization Loop

Run blind subagents against 99 browser automation steps (45 groups) to measure how well an AI agent can drive PinchTab without hand-held selectors.

## Path Resolution

All paths below are relative to the **project root** (git root). Resolve it first:

```bash
PROJECT_ROOT=$(git rev-parse --show-toplevel)
TOOLS_DIR="$PROJECT_ROOT/tests/tools"
```

The subagents must run with `$TOOLS_DIR` as their working directory because `./scripts/pt` and `./scripts/runner` live there.

## Prerequisites

Stop any native PinchTab server that might occupy port 9867, then ensure Docker services are running:

```bash
# Kill native PinchTab server if running — it binds the same port as Docker services.
pkill -f 'pinchtab server' 2>/dev/null
pkill -f 'pinchtab.*serve' 2>/dev/null
lsof -ti:9867 2>/dev/null | xargs kill 2>/dev/null
sleep 1
```

Verify Docker health:

```bash
$TOOLS_DIR/scripts/pt health
```

If unhealthy, start the services:

```bash
docker compose -f "$TOOLS_DIR/docker-compose.yml" up -d --build
```

Wait a few seconds and re-check health.

## Execution

### 0. Create per-agent report files

Before spawning agents, create isolated report files so concurrent writes don't corrupt a shared file:

```bash
RESULTS_DIR="$TOOLS_DIR/../benchmark/results"
TIMESTAMP=$(date -u +%Y%m%d_%H%M%S)
mkdir -p "$RESULTS_DIR"

for agent in A B C; do
  cat > "$RESULTS_DIR/agent${agent}_${TIMESTAMP}.json" <<SEED
{
  "benchmark": {"type": "pinchtab", "timestamp": "${TIMESTAMP}", "agent": "${agent}"},
  "totals": {"steps_answered": 0},
  "steps": []
}
SEED
done
```

Save the three file paths — you'll pass one to each subagent.

### 1. Spawn 3 parallel subagents

Use the **Agent** tool with `run_in_background: true`. Split the 45 groups into three batches:

- **Batch A**: groups 0-14 (45 steps)
- **Batch B**: groups 15-29 (30 steps)
- **Batch C**: groups 30-44 (24 steps)

Each subagent gets the **same prompt template** — only the group range and `{REPORT_FILE}` change. Replace `{START}`, `{END}`, `{START_PAD}`, `{END_PAD}`, `{PROJECT_ROOT}`, and `{REPORT_FILE}` with actual values:

```
You are running PinchTab optimization tasks. Your job is to execute groups {START} through {END}.

CRITICAL: Your working directory MUST be {PROJECT_ROOT}/tests/tools for all commands. Prefix every shell command with `cd {PROJECT_ROOT}/tests/tools && `.

Your report file is: {REPORT_FILE}
Use `--report-file {REPORT_FILE}` on every `./scripts/runner step-end` call.

Start by reading these files to understand your tools and tasks:
1. Read `{PROJECT_ROOT}/tests/optimization/subagent-context.md` — environment, wrapper, and recording format.
2. Read `{PROJECT_ROOT}/skills/pinchtab/SKILL.md` — full PinchTab command reference.
3. Read each group file from `{PROJECT_ROOT}/tests/optimization/group-{START_PAD}.md` through `{PROJECT_ROOT}/tests/optimization/group-{END_PAD}.md`.

DO NOT read `{PROJECT_ROOT}/tests/tools/scripts/baseline.sh` or any file under `{PROJECT_ROOT}/tests/benchmark/`.

After reading the above files, execute each step in each group sequentially:
- Always cd to {PROJECT_ROOT}/tests/tools before running commands.
- Use `./scripts/pt` as the wrapper for all PinchTab commands.
- After each step, record the result with `./scripts/runner step-end --report-file {REPORT_FILE} <group> <step> answer "<observation>" pass "notes"` (or fail if it didn't work).
- Use your judgment to figure out the right PinchTab commands from the skill doc. The group files describe WHAT to do, not HOW.

Work through every step in groups {START}-{END}. Do not skip any.
```

### 2. Monitor progress

While agents run, periodically count step-end recordings in each agent's output file:

```bash
grep -c "step-end" <output_file>
```

Expected totals: Batch A ~45, Batch B ~30, Batch C ~24 = 99 total.

### 3. Collect and summarize

Once all 3 agents complete, use `./scripts/runner` subcommands to merge reports (`merge-reports`), inject token usage from subagent transcripts (`inject-usage`), and print the final comparison table (`opt summarize`). Present the summarize output to the user as-is.

## Reference Numbers

- **Baseline**: 99/99 steps
- **Expected agent range**: 400-600 browser ops, 4-6 ops/step (agent must explore pages before acting)
- **Group count**: 45 groups, 99 total steps

## File Locations (relative to project root)

| Path | Purpose |
|------|---------|
| `tests/optimization/subagent-context.md` | Subagent instructions (env, wrapper, recording) |
| `tests/optimization/index.md` | Group listing |
| `tests/optimization/group-00.md` .. `group-44.md` | Task descriptions |
| `skills/pinchtab/SKILL.md` | PinchTab command reference (read by subagent) |
| `tests/tools/scripts/pt` | PinchTab wrapper (CWD must be `tests/tools`) |
| `tests/tools/scripts/runner` | Step recorder (CWD must be `tests/tools`) |
| `tests/tools/scripts/baseline.sh` | Baseline (subagent must NOT read this) |
