# PinchTab Benchmark

Structured benchmarks to measure AI agent performance with PinchTab and other
browser-control surfaces against the same fixture suite.

## Quick Start

```bash
cd tests/benchmark

# PinchTab lane
./scripts/run-optimization.sh

# agent-browser lane
./scripts/run-agent-browser-benchmark.sh
# Then read AGENT_BROWSER_INSTRUCTIONS.md and run tasks from AGENT_BROWSER_TASKS.md with ./scripts/ab
./scripts/finalize-report.sh
```

## MANDATORY: Docker Environment

**The benchmark MUST run against Docker.** Do not use a local pinchtab server.

Reasons:
- Reproducible: Same environment every run
- Clean state: No leftover profiles, instances, or sessions
- Latest build: Builds from current source
- Isolated: No interference from local config

If Docker build fails or is skipped, the benchmark is **INVALID**.

## Files

| File | Purpose |
|------|---------|
| `../../skills/pinchtab/SKILL.md` | PinchTab skill (same as shipped product) |
| `AGENT_BROWSER_INSTRUCTIONS.md` | Benchmark-local instructions for the `agent-browser` lane |
| `BASELINE_TASKS.md` | Standalone task list (same as skill) |
| `AGENT_BROWSER_TASKS.md` | Equivalent task lane for `agent-browser` |
| `scripts/run-optimization.sh` | Initialize PinchTab benchmark reports |
| `scripts/run-agent-browser-benchmark.sh` | Start fixtures + `agent-browser` and initialize a fresh report |
| `scripts/ab` | Docker-backed `agent-browser` wrapper with tool-call logging |
| `scripts/record-step.sh` | Record step results and tool-call counts |
| `scripts/finalize-report.sh` | Generate final summary report |
| `config/pinchtab.json` | PinchTab configuration |
| `agent-browser/Dockerfile` | `agent-browser` benchmark image |
| `docker-compose.yml` | Docker environment definition |
| `results/` | Output directory for reports |

## Execution

The benchmark is designed to run in a fresh agent context:

1. Initialize the relevant benchmark lane
2. Execute the natural-language tasks with the target browser surface
3. Record each step's raw answer/result
4. Verify observed steps in a separate pass
5. Let the harness count browser/tool calls where possible

This measures the **real cost** of using a browser tool with an agent, including:
- Context loading overhead
- Browser/tool-call count
- Total benchmark cost

For agent lanes, the recommended flow is two-phase:

1. execution agent records each step as `answer`
2. verifier agent reads the report and stamps each step with `verify-step.sh`

## Environment

The benchmark runs PinchTab in Docker with:

- **Port**: 9867
- **Token**: `benchmark-token`
- **Stealth**: Full (for protected sites)
- **Headless**: Yes
- **Multi-instance**: Enabled (2 instances)

## Step Recording

Every step must record the answer/result:

```bash
./scripts/record-step.sh <group> <step> <pass|fail|skip|answer> "notes"
```

Example:
```bash
./scripts/record-step.sh 1 1 pass "Navigation completed in 1.2s"
./scripts/record-step.sh 2 3 fail "Element not found"
```

Deferred-verification example:

```bash
./scripts/record-step.sh --type agent 1 1 answer \
  "Found categories Programming Languages: 12, Databases: 8" \
  "raw answer"
./scripts/verify-step.sh --type agent 1 1 pass \
  "Answer satisfies the benchmark oracle"
```

## Reports

Reports are generated in `results/`:

- `benchmark_YYYYMMDD_HHMMSS.json` - Raw JSON data
- `benchmark_YYYYMMDD_HHMMSS_summary.md` - Human-readable summary
- `agent_browser_commands.ndjson` - `agent-browser` command log for tool-call attribution

### Example Summary

```
# PinchTab Benchmark Results

## Results
| Metric | Value |
|--------|-------|
| Steps Passed | 30 |
| Steps Failed | 2 |
| Pass Rate | 93.7% |

## Tooling
| Metric | Value |
|--------|-------|
| Tool Calls | 128 |
```

## Running Programmatically

For automated benchmarks, you can:

1. Parse `BASELINE_TASKS.md` for curl commands
2. Execute each command
3. Parse responses for pass/fail
4. Call `scripts/record-step.sh` with results
5. Run `scripts/finalize-report.sh`

## Reproducibility

For consistent results:

1. Always start with a fresh Docker-backed benchmark lane
2. Use the same model/temperature for comparisons
3. Run benchmarks at similar times (site load varies)
4. Record exact PinchTab version from `/version` endpoint
5. Clear browser state between full benchmark runs
