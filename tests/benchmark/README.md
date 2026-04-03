# PinchTab Agent Benchmark

Structured benchmarks to measure AI agent performance with PinchTab browser automation.

## Quick Start

```bash
# 1. Start Docker + spawn subagent benchmark
./run-agent-benchmark.sh

# Or manually:
./run-benchmark.sh        # Start Docker (REQUIRED)
# Then run tasks from ../../skills/pinchtab/SKILL.md
./finalize-report.sh      # Generate report
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
| `BENCHMARK_TASKS.md` | Standalone task list (same as skill) |
| `AGENT_SCRIPT.md` | Extended: 100+ comprehensive tasks |
| `run-agent-benchmark.sh` | **Recommended**: Start Docker + spawn subagent |
| `run-benchmark.sh` | Start Docker environment only |
| `record-step.sh` | Record step results with token counts |
| `finalize-report.sh` | Generate final summary report |
| `config/pinchtab.json` | PinchTab configuration |
| `docker-compose.yml` | Docker environment definition |
| `results/` | Output directory for reports |

## Subagent Execution

The benchmark is designed to run in a **fresh subagent with clean context**:

1. Subagent loads `skill/SKILL.md` (this is the skill loading cost)
2. Step 0.1 records the skill loading overhead
3. Steps 1.x - 8.x execute actual benchmark tasks
4. Each step records input/output tokens

This measures the **real cost** of using PinchTab with an agent, including:
- Skill/context loading overhead
- Per-task token usage
- Total benchmark cost

## Environment

The benchmark runs PinchTab in Docker with:

- **Port**: 9867
- **Token**: `benchmark-token`
- **Stealth**: Full (for protected sites)
- **Headless**: Yes
- **Multi-instance**: Enabled (2 instances)

## Token Tracking

Every step must track token usage:

```bash
./record-step.sh <group> <step> <pass|fail|skip> <input_tokens> <output_tokens> "notes"
```

Example:
```bash
./record-step.sh 1 1 pass 150 45 "Navigation completed in 1.2s"
./record-step.sh 2 3 fail 200 80 "Element not found"
```

Token counts should come from your model's API response:
- **Anthropic**: `usage.input_tokens`, `usage.output_tokens`
- **OpenAI**: `usage.prompt_tokens`, `usage.completion_tokens`
- **Gemini**: `usageMetadata.promptTokenCount`, `usageMetadata.candidatesTokenCount`

## Reports

Reports are generated in `results/`:

- `benchmark_YYYYMMDD_HHMMSS.json` - Raw JSON data
- `benchmark_YYYYMMDD_HHMMSS_summary.md` - Human-readable summary

### Example Summary

```
# PinchTab Benchmark Results

## Results
| Metric | Value |
|--------|-------|
| Steps Passed | 30 |
| Steps Failed | 2 |
| Pass Rate | 93.7% |

## Token Usage
| Metric | Value |
|--------|-------|
| Total Tokens | 4,523 |
| Estimated Cost | $0.0158 |
```

## Running Programmatically

For automated benchmarks, you can:

1. Parse `BENCHMARK_TASKS.md` for curl commands
2. Execute each command
3. Parse responses for pass/fail
4. Call `record-step.sh` with results
5. Run `finalize-report.sh`

## Reproducibility

For consistent results:

1. Always start with `./run-benchmark.sh` (fresh Docker)
2. Use the same model/temperature for comparisons
3. Run benchmarks at similar times (site load varies)
4. Record exact PinchTab version from `/version` endpoint
5. Clear browser state between full benchmark runs
