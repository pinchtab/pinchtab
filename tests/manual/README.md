# Manual Tests

This directory contains **optional** developer reference material only.

Automated coverage is provided by `tests/integration/`. You should not need to run anything here as part of normal development or CI.

## Contents

| File | Purpose |
|------|---------|
| `quick-start.md` | Step-by-step guided walkthrough of core orchestrator features |

## When to use

- Learning how the orchestrator works for the first time
- Visual confirmation that a headed Chrome instance opens correctly
- Reproducing a bug you can't isolate in integration tests

## Running integration tests instead

```bash
./pinchtab &
sleep 3
go test -v ./tests/integration
```

See [`TESTING.md`](../../TESTING.md) for the full test strategy.
