# Agent Browser Instructions

These are benchmark-specific instructions for running the `agent-browser` lane.
They are not the official `agent-browser` skill. For the live CLI skill, load it
through the benchmark wrapper:

```bash
./scripts/ab skills get agent-browser --full
```

## Wrapper

For benchmark runs in this repo, do not call `agent-browser` directly. Use:

```bash
./scripts/ab ...
```

That wrapper:

- executes `agent-browser` inside the benchmark Docker service
- preserves the shared browser session across commands
- logs tool calls to `results/agent_browser_commands.ndjson`

## Benchmark Workflow

1. Start the lane:

```bash
cd tests/benchmark
./scripts/run-agent-browser-benchmark.sh
```

2. Load the live CLI skill:

```bash
./scripts/ab skills get agent-browser --full
```

3. Run browser actions through the wrapper:

```bash
./scripts/ab open http://fixtures/
./scripts/ab snapshot -i -c
./scripts/ab click @e2
./scripts/ab fill @e3 "agent@benchmark.test"
```

4. Record execution results:

```bash
./scripts/record-step.sh --type agent-browser 1 1 answer \
  "opened fixtures home and got refs e1-e13" "completed"
```

5. Verify later in a separate pass:

```bash
./scripts/verify-step.sh --type agent-browser 1 1 pass \
  "matched expected homepage state"
```

## Environment

- Fixtures: `http://fixtures/`
- Session name: `benchmark` by default (`AGENT_BROWSER_SESSION` overrides)
- Browser driver: Docker service `agent-browser`

## Operating Guidance

- Treat `./scripts/ab skills get agent-browser --full` as the source of truth for
  current command syntax and workflows
- Prefer refs from `snapshot -i -c` over brittle selectors
- Re-snapshot after navigation or any DOM-changing action
- Keep one session for the whole benchmark lane unless a task explicitly needs a reset
