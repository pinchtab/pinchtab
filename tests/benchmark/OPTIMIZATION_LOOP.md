# PinchTab Auto-Optimization Loop

Automated improvement cycle that runs every 25 minutes.

## Loop Steps

```
┌─────────────────────────────────────────────────────────────┐
│  1. RUN BENCHMARKS                                          │
│     - Baseline (explicit curl commands)                     │
│     - Agent (natural language tasks)                        │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│  2. ANALYZE DIFFERENCES                                     │
│     - Compare pass rates                                    │
│     - Identify step failures                                │
│     - Find patterns (text vs snapshot, selectors, etc.)     │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│  3. PROPOSE 1 IMPROVEMENT                                   │
│     Priority order:                                         │
│     a) Fix PinchTab CLI/REST bug if found                   │
│     b) Improve skill documentation if agent confused        │
│     c) Add verification to existing test case               │
│     d) Add new test case for uncovered scenario             │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│  4. IMPLEMENT & COMMIT                                      │
│     - Make the change                                       │
│     - Commit with descriptive message                       │
│     - Push to feat/benchmark branch                         │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│  5. LOG RUN                                                 │
│     - Append to optimization_log.md                         │
│     - Record: timestamp, pass rates, change made            │
└─────────────────────────────────────────────────────────────┘
                              ↓
                     [Wait 25 minutes]
                              ↓
                        [Repeat]
```

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

**Analysis:**
- [What differed between runs]
- [Root cause identified]

**Change Made:**
- [Type: api|skill|benchmark]
- [Description]
- [Commit: abc1234]

**Next Focus:**
- [What to look at next run]
```

## Files

- `run-optimization.sh` — Main loop script
- `results/optimization_log.md` — Run history
- All changes committed to `feat/benchmark` branch
