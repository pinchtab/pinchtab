# Agent Browser Benchmark

Natural-language benchmark lane for running the same fixture tasks with
[`agent-browser`](https://github.com/vercel-labs/agent-browser) instead of the
PinchTab CLI.

## Setup

1. Read `./AGENT_BROWSER_INSTRUCTIONS.md` first. This is your benchmark guide to
   the wrapper and command workflow.

2. Load the live CLI skill content before using browser commands:

```bash
./scripts/ab skills get agent-browser --full
```

3. Start the Docker benchmark environment and initialize a report:

```bash
cd tests/benchmark
./scripts/run-agent-browser-benchmark.sh
```

4. Use the Docker-backed wrapper for every browser action:

```bash
./scripts/ab open http://fixtures/
./scripts/ab snapshot -i -c
./scripts/ab click @e2
./scripts/ab fill @e3 "agent@benchmark.test"
```

5. Record each completed step as an answer/result first:

```bash
./scripts/record-step.sh --type agent-browser 1 1 answer \
  "opened fixtures home and got refs e1-e13" "completed"
```

`record-step.sh` will automatically calculate the number of `agent-browser`
tool calls used since the previous recorded step by reading
`results/agent_browser_commands.ndjson`.

6. After the execution lane is done, run a separate verification pass with:

```bash
./scripts/verify-step.sh --type agent-browser 1 1 pass "matched expected homepage state"
```

## Environment

- Fixtures: `http://fixtures/`
- Session name: `benchmark` by default (`AGENT_BROWSER_SESSION` overrides)
- Browser driver: Docker service `agent-browser`

## Tooling Guidance

Use `./AGENT_BROWSER_INSTRUCTIONS.md` as the primary benchmark operating guide.
Reach for `./scripts/ab --help` only when the skill does not already answer the
question.

## Task Set

Reuse the same benchmark task groups from [AGENT_TASKS.md](./AGENT_TASKS.md)
for content extraction, search, forms, SPA state, login, e-commerce, exports,
dialogs, async flows, drag/drop, keyboard, scrolling, and iframe interaction.

The only setup difference is that Group 0 should validate the `agent-browser`
lane instead of the PinchTab server:

- 0.1 `./scripts/ab open http://fixtures/` succeeds
- 0.2 `./scripts/ab snapshot -i -c` returns interactive refs
- 0.3 session state persists across multiple `./scripts/ab ...` commands

After that, continue with the same user-facing tasks from Group 1 onward.
