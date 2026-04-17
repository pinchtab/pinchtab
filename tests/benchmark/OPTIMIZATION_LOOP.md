# PinchTab Auto-Optimization Loop

Automated improvement cycle for PinchTab benchmarks.

## Pre-requisite (Human, once)

```bash
cd ~/dev/pinchtab/tests/benchmark
docker compose up -d --build
# Wait for healthy:
curl -sf -H "Authorization: Bearer benchmark-token" http://localhost:9867/health
```

## Loop Steps

```
┌─────────────────────────────────────────────────────────────┐
│  1. RUN AGENT BENCHMARK                  [Sub-agent]        │
│     - Runs every iteration                                  │
│     - Load AGENT_TASKS.md + skills/pinchtab/SKILL.md        │
│     - Execute tasks using only skill docs                   │
│     - Record via: record-step.sh --type agent               │
│     - Output: results/agent_benchmark_<timestamp>.json      │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│  2. CHECK FOR REGRESSION                 [Main agent]       │
│     - Read both JSON reports for pass/fail counts           │
│     - Compare against results/best_score.txt                │
│     - If worse: flag regression, prioritize fix/revert      │
│     - If equal or better: update best_score.txt             │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│  3. ANALYZE DIFFERENCES                  [Main agent]       │
│     - Diff baseline vs agent results side by side           │
│     - Identify which steps agent failed but baseline passed │
│     - Find patterns (wrong endpoint, selectors, etc.)       │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│  4. PROPOSE 1 IMPROVEMENT                [Main agent → Human]│
│     Priority order:                                         │
│     a) Fix PinchTab CLI/REST bug if found                   │
│     b) Improve skill documentation if agent confused        │
│     c) Add verification to existing test case               │
│     d) Add new test case for uncovered scenario             │
│     Present proposal to human for approval.                 │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│  5. IMPLEMENT (no commit)                [Main agent]       │
│     - Make the single change                                │
│     - Leave changes uncommitted for manual review           │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│  6. LOG RUN                              [Main agent]       │
│     - Append to results/optimization_log.md                 │
│     - Record: timestamp, pass rates, regression check,      │
│       change made, next focus                               │
└─────────────────────────────────────────────────────────────┘
```

## Roles

| Step | Who | What |
|------|-----|------|
| Pre-req | **Human** | Start Docker, verify health |
| 1 Agent | **Sub-agent** | Execute agent tasks from skill docs, record results |
| 2 Regression | **Main agent** | Compare against best_score.txt |
| 3 Analyze | **Main agent** | Diff baseline vs agent, find patterns |
| 4 Propose | **Main agent → Human** | Suggest 1 fix, get approval |
| 5 Implement | **Main agent** | Make the change, leave uncommitted |
| 6 Log | **Main agent** | Append to optimization_log.md |

## Baseline is a Prerequisite, Not Part of the Loop

The baseline suite (`BASELINE_TASKS.md`) validates that the benchmark infrastructure itself works — fixtures serve correctly, PinchTab APIs behave as expected, and the test conditions are reachable. It's deterministic (no LLM involved), so once it's at 100% it stays at 100% unless something changes underneath it.

**The loop does NOT re-run the baseline each iteration.** Instead, run the baseline manually when any of these change:

- PinchTab server code (anything in `internal/` that affects the HTTP API)
- Fixtures (anything in `tests/benchmark/fixtures/`)
- `BASELINE_TASKS.md` itself (adding/modifying tests)
- `docker-compose.yml` or the Docker image

**Workflow for adding a new test case:**

1. Write the new test in `BASELINE_TASKS.md` (curl commands + pass condition)
2. Write the matching task in `AGENT_TASKS.md` (natural language)
3. Write the summary row in `TEST_CASES.md`
4. Add any new fixture files needed
5. **Run the baseline** — it must reach 100% before the case is considered valid
6. If baseline fails: fix the test/fixture/PinchTab until it passes
7. Commit the new case — now the agent loop can use it

Once baseline is green, the optimization loop only runs the agent suite.

## Regression Detection

Each run compares against `results/best_score.txt`, which tracks the highest pass rates achieved:

```
baseline=62/68
agent=33/39
```

- **If current run is worse**: Regression detected. The improvement priority shifts to fixing or reverting the last change before proposing anything new.
- **If equal or better**: Update `best_score.txt` with the new high-water mark.

## Improvement Priority

1. **API/CLI Bug** — If a curl command returns unexpected error, investigate PinchTab code
2. **Skill Gap** — If agent uses wrong endpoint/approach, improve SKILL.md documentation
3. **Benchmark Gap** — If tests don't verify important behavior, add verification
4. **Coverage Gap** — If common scenarios aren't tested, add test cases

## Log Format

Each run appends to `results/optimization_log.md`:

```markdown
## Run #N — YYYY-MM-DD HH:MM

**Results:**
- Baseline: X/Y (Z%)
- Agent: X/Y (Z%)

**Regression Check:**
- Best: baseline X/Y, agent X/Y
- This run: [better | same | REGRESSION]

**Analysis:**
- [What differed between runs]
- [Root cause identified]

**Change Made:**
- [Type: api|skill|benchmark]
- [Description]
- [Status: uncommitted — pending manual review]

**Next Focus:**
- [What to look at next run]
```

## Files

- `scripts/run-optimization.sh` — Main loop script (initializes reports)
- `scripts/record-step.sh` — Appends a step result to the current report
- `results/optimization_log.md` — Run history (includes per-run metrics table)
- `results/best_score.txt` — High-water mark for regression detection
- Changes left uncommitted for manual review and commit

Agent runs use the PinchTab CLI directly via the standard env vars
(`PINCHTAB_TOKEN`, `PINCHTAB_SERVER`, `PINCHTAB_TAB`) — no helper file to
source. See `AGENT_TASKS.md` "Recommended setup" for the pattern.
