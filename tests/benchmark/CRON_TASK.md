# PinchTab Optimization Cron Task

**Goal: Close the gap between baseline (explicit curl) and agent (natural language).**

Every run must bring the agent closer to 100%. If already at 100%, expand tests.

---

## Task for Agent

### Step 1 — Setup
```bash
cd ~/dev/pinchtab/tests/benchmark
git checkout feat/benchmark && git pull --rebase origin feat/benchmark
./run-optimization.sh
# Note the TIMESTAMP from output
```

### Step 2 — Run Baseline Benchmark
Execute BENCHMARK_TASKS.md exactly as written.
- Token: `benchmark-token`
- Record each step: `./record-step.sh <group> <step> <pass|fail> <in> <out> "notes"`
- For failures: include HTTP status, expected string, actual response in notes

### Step 3 — Run Agent Benchmark
Execute AGENT_TASKS.md using SKILL.md as your only guide.
- **Do NOT look at BENCHMARK_TASKS.md**
- Figure out commands from skill documentation
- Record: `./record-agent-step.sh <group> <step> <pass|fail> <in> <out> "commands used" "result"`
- Log every curl/command executed

### Step 4 — Gap Analysis
Compare results and classify each agent failure:

| Failure Type | Cause | Fix |
|---|---|---|
| **Wrong endpoint** | Agent used `/text` when should use `/snapshot` | Improve SKILL.md |
| **Wrong selector** | Agent guessed selector incorrectly | Improve SKILL.md or fixture |
| **Missing step** | Agent skipped a required action | Clarify AGENT_TASKS.md |
| **Wrong URL** | Agent used wrong fixture path | Clarify AGENT_TASKS.md |
| **API bug** | Endpoint behaves unexpectedly | Fix PinchTab code |
| **Test ambiguity** | Verification string hard to find | Fix fixture or test |

### Step 5 — Make Exactly 1 Change

Priority order:
1. **API Bug** → Fix PinchTab Go code, commit as `fix: <description>`
2. **Skill Gap** → Improve SKILL.md with clearer guidance or example, commit as `docs(skill): <description>`
3. **Test Ambiguity** → Fix fixture HTML or BENCHMARK_TASKS.md verification, commit as `test: <description>`
4. **Agent Task Clarity** → Improve AGENT_TASKS.md phrasing, commit as `test(agent): <description>`
5. **No Gaps Found** → Add 2-3 new test cases covering uncovered scenarios, commit as `test: add cases for <scenario>`

**If agent is already at 100%**: Add harder cases (nested interactions, state persistence, multi-page flows).

### Step 6 — Verify the Fix Makes Sense
Before committing, ask: "Will this change help the agent pick the right tool next time?"
- If you fixed a skill doc: check that the added text directly addresses the failure
- If you fixed a test: check the verification string is reachable via snapshot

### Step 7 — Commit & Push
```bash
cd ~/dev/pinchtab
git add -A
git commit -m "<type>: <clear description of what changed and why>"
git push origin feat/benchmark
```

### Step 8 — Log the Run
Append to `results/optimization_log.md`:

```markdown
## Run #N — YYYY-MM-DD HH:MM

**Results:**
- Baseline: X/Y (Z%)
- Agent: X/Y (Z%)
- Gap: N steps

**Agent Failures:**
- Step X.Y: [failure type] — [what went wrong]
- Step X.Y: [failure type] — [what went wrong]

**Root Cause:**
[One clear sentence explaining the pattern]

**Change Made:**
- Type: api|skill|test|agent-task
- Description: [What changed]
- Expected impact: [Why this should close the gap]
- Commit: [hash]

**Next Focus:**
[What to watch in the next run]
```

### Step 9 — Report to User
Send a concise update:
```
Run #N complete
Baseline: X% | Agent: Y% | Gap: N steps
Failure: [brief description]
Fix: [what changed] ([commit])
```

---

## Success Criteria

- **Short term**: Agent pass rate ≥ 95%
- **Long term**: Agent matches baseline on all existing tests
- **Ongoing**: When gap closes, increase test complexity

## What NOT to do

- ❌ Don't make multiple changes per run — one focused fix only
- ❌ Don't skip the analysis step — root cause first, then fix
- ❌ Don't change tests just to make them easier — the goal is better skill/API
- ❌ Don't commit if the change doesn't directly address an observed failure
