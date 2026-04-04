# Benchmark 2 Optimization Cron Task

**Goal: Agent reads SKILL.md once → completes all 6 phases with minimum tokens.
Compare every run to baseline. Improve SKILL.md until agent matches baseline efficiency.**

---

## Task

### Step 1 — Setup
```bash
cd ~/dev/pinchtab
git checkout feat/benchmark-2 && git pull --rebase origin feat/benchmark-2
./tests/benchmark2/scripts/init-run.sh
```

### Step 2 — Start services (if not running)
```bash
# PinchTab
./pinchtab server &>/tmp/pinchtab.log &
sleep 6 && ./pinchtab health

# Fixtures (Docker)
cd tests/benchmark && docker compose up -d && cd ../..
sleep 5
curl -sf -H "Authorization: Bearer benchmark-token" http://localhost:9867/health | jq -r '.status'
```

### Step 3 — Read skill ONCE
Read `skills/pinchtab/SKILL.md` once.
Stop reading after answering:
1. How to auth?
2. nav command?
3. text vs snapshot — when to use each?
4. How to fill + submit a form?
5. How to handle real external sites (IDPI)?

Record: `./tests/benchmark2/scripts/record-phase.sh 0 1 pass <tokens> "answered in N lines"`

### Step 4 — Run all phases
Work through **INIT_TASKS.md Phases 1-5**.
- Do NOT re-read the skill between phases
- Record each phase: `./tests/benchmark2/scripts/record-phase.sh <phase> <step> <pass|fail> <tokens> "notes"`
- Note any wrong turns, retries, or confusion

### Step 5 — Analyze
Find the phase with the highest token cost relative to budget.
Ask: "What in SKILL.md caused the extra tokens?"

Classify root cause:
- **unclear** — guidance exists but was hard to find
- **missing** — no guidance for this scenario  
- **wrong** — guidance led to the wrong approach
- **verbose** — had to read too much to find one fact

### Step 6 — One SKILL.md improvement
Make exactly 1 change. Priority:
1. Fix wrong guidance (causes failures)
2. Add missing guidance (causes retries)
3. Move buried info closer to top (causes slow scan)
4. Compress verbose sections (causes high token load)

Commit: `docs(skill): <what + why it reduces init tokens>`

### Step 7 — Log and push
Append to `tests/benchmark2/results/optimization_log.md`:

```markdown
## Run #N — YYYY-MM-DD HH:MM

| Phase | Budget | Actual | Pass | Root cause |
|-------|--------|--------|------|------------|
| 0 Skill | 400 | N | ✅/❌ | |
| 1 Server | 100 | N | ✅/❌ | |
| 2 Fixtures | 400 | N | ✅/❌ | |
| 3 Real sites | 600 | N | ✅/❌ | |
| 4 Forms | 400 | N | ✅/❌ | |
| 5 Multi-site | 1500 | N | ✅/❌ | |
| **Total** | **3400** | **N** | | |

**Highest cost phase**: X (N tokens, budget N)
**Root cause type**: unclear/missing/wrong/verbose
**Fix**: [description]
**Commit**: [hash]
```

```bash
git add skills/pinchtab/SKILL.md tests/benchmark2/results/
git commit -m "docs(skill): <description>"
git push origin feat/benchmark-2
```

### Step 8 — Report
```
Benchmark 2 Run #N
P0=N P1=N P2=N P3=N P4=N P5=N  Total=N/3400
Highest cost: Phase X (root cause: type)
Fix: [description] (commit: hash)
```

---

## Target Progression

| Run | Total tokens goal |
|-----|-------------------|
| 1 | Baseline (measure) |
| 3 | < 3000 |
| 6 | < 2500 |
| 10 | < 2000 |

## Rules

- ❌ Never re-read the skill mid-run
- ❌ Never make more than 1 SKILL.md change per run
- ❌ Never skip real-site phases (P3) — they expose IDPI/DNS issues
- ✅ If a phase times out, record as fail with 0 tokens and continue
- ✅ If Docker is down, skip P2/P4, run P3/P5 only
