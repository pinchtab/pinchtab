# Pinchtab: Clean Slate Agent Test

## Scenario

An agent with **zero prior context** spawns, calls Pinchtab once per site, extracts content. Measures total token cost including agent overhead.

**Real-world use case:** Distributed agents that spawn fresh for each task, call Pinchtab, return results, die.

---

## Test Setup

- **Agent context:** Minimal (system prompt only, no conversation history)
- **Pinchtab state:** Running in background (already initialized)
- **Endpoint:** `/snapshot` with `format=compact&maxTokens=2000`
- **Sites:** BBC.com, Corriere.it, Daily Mail.co.uk

---

## Results

### Agent Overhead (Per Task)

| Component | Tokens | Notes |
|-----------|--------|-------|
| System prompt | ~300 | OpenClaw agent skeleton |
| Task description | ~150 | "Call Pinchtab, extract titles" |
| HTTP request formation | ~50 | curl command construction |
| **Total agent OH** | **~500** | Minimal, cold start |

---

### Pinchtab Response Size & Tokens

#### BBC.com
- **HTTP response size:** ~3-4 KB (compact format, limited titles)
- **Est tokens:** ~800-1,000
- **Titles extracted:** 10
- **Sample:** Breaking News, World News, Sports, Business, Innovation...
- **Agent + Pinchtab total:** 500 + 900 = **1,400 tokens**

#### Corriere.it
- **HTTP response size:** ~2-3 KB (compact, curated)
- **Est tokens:** ~600-800
- **Titles extracted:** 10
- **Sample:** Sanremo scaletta, Iran Asse Cina, Legge elettorale, Epstein deposizione...
- **Agent + Pinchtab total:** 500 + 700 = **1,200 tokens**

#### Daily Mail.co.uk
- **HTTP response size:** ~4-5 KB (compact, dense)
- **Est tokens:** ~1,000-1,300
- **Titles extracted:** 10
- **Sample:** Ian Huntley assault, Hillary Clinton testimony, Starmer polls, dental condition...
- **Agent + Pinchtab total:** 500 + 1,150 = **1,650 tokens**

---

## Summary Table

### Per-Site Breakdown (Agent + Pinchtab)

| Site | Pinchtab | Agent OH | **Total** | vs Snapshot | vs web_fetch |
|------|----------|----------|----------|-------------|-------------|
| BBC | 900 | 500 | **1,400** | 11,250 / 1,400 = 8x lighter | 4,700 / 1,400 = 3.4x lighter |
| Corriere | 700 | 500 | **1,200** | 95,000 / 1,200 = 79x lighter | 3,275 / 1,200 = 2.7x lighter |
| Daily Mail | 1,150 | 500 | **1,650** | 87,500 / 1,650 = 53x lighter | 12,500 / 1,650 = 7.6x lighter |
| **Average** | **920** | **500** | **1,417** | **~47x lighter** | **~4.6x lighter** |

---

## Cost Implications

### Single Task (Fresh Agent)

| Method | Agent OH | Service | Total Tokens | Cost (Sonnet) |
|--------|----------|---------|--------------|---------------|
| **Snapshot** | 500 | 64,583 | **65,083** | $0.000195 |
| **web_fetch** | 500 | 6,825 | **7,325** | $0.000022 |
| **Pinchtab** | 500 | 900 | **1,400** | **$0.000004** |

**Pinchtab cost advantage:**
- vs. Snapshot: **98% cheaper** ($0.000191 saved per task)
- vs. web_fetch: **82% cheaper** ($0.000018 saved per task)

### At Scale (1,000 tasks/day)

| Method | Daily Cost | Monthly Cost | Annual Cost |
|--------|-----------|--------------|-------------|
| **Snapshot** | $0.195 | $5.85 | $70.20 |
| **web_fetch** | $0.022 | $0.66 | $7.92 |
| **Pinchtab** | **$0.004** | **$0.12** | **$1.44** |

**Monthly savings with Pinchtab:**
- vs. Snapshot: **$5.73/month** = **$68.76/year**
- vs. web_fetch: **$0.54/month** = **$6.48/year**

---

## Key Insight: Agent Overhead

When agents spawn fresh (cold start):
- **Agent cost is fixed:** ~500 tokens per spawn
- **Service call determines total:** Pinchtab's 900 tokens > Agent's 500 tokens
- **Service efficiency wins:** Choosing Pinchtab saves ~$0.000191/task vs. snapshot

**At 1,000 spawns/day:**
- Snapshot: 500 (agent) + 64,583 (snapshot) = 65,083 tokens
- **Pinchtab: 500 (agent) + 900 (pinchtab) = 1,400 tokens** ← 46x lighter

---

## Real-World Pattern

### Distributed Agent Architecture

```
Master scheduler (long-lived)
  ↓
  ├─→ Spawn agent 1 (cold start, ~500 tokens)
  │   └─→ Call Pinchtab BBC (~900 tokens)
  │   └─→ Return results, die
  │
  ├─→ Spawn agent 2 (cold start, ~500 tokens)
  │   └─→ Call Pinchtab Corriere (~700 tokens)
  │   └─→ Return results, die
  │
  └─→ Spawn agent 3 (cold start, ~500 tokens)
      └─→ Call Pinchtab Daily Mail (~1,150 tokens)
      └─→ Return results, die

Total: 3 × (500 + ~920) = ~4,260 tokens
Compare:
- 3 × Snapshot: 3 × 65,083 = 195,249 tokens (46x heavier)
- 3 × web_fetch: 3 × 7,325 = 21,975 tokens (5x heavier)
```

---

## vs. Other Methods

### When Agent is Long-Lived (Conversation)

If agent already running (context already paid):
- Snapshot call cost: **64,583 tokens**
- web_fetch call cost: **6,825 tokens**
- **Pinchtab call cost: ~900 tokens** (no agent spawn)

**Savings diminish** because agent cost is sunk.

### When Agent Spawns Fresh (Per-Task)

If agent spawns for each task:
- Snapshot: 500 (agent) + 64,583 = **65,083 tokens**
- web_fetch: 500 (agent) + 6,825 = **7,325 tokens**
- **Pinchtab: 500 (agent) + 900 = 1,400 tokens** ← **Winner**

**Pinchtab wins hardest** when agents are ephemeral.

---

## Scenarios Where Pinchtab Dominates

1. **Task-based agents** (spawn, extract, die)
2. **Parallel crawling** (1000s of agents running simultaneously)
3. **Cost-sensitive workflows** (every token matters)
4. **Real-time extraction** (needs Chrome rendering, not just text)
5. **High-volume API services** (1000+ requests/day)

---

## Limitations

1. **Pinchtab startup isn't measured here** — Running in background already
2. **Agent overhead assumed constant** — Varies by model (Haiku vs. Opus)
3. **Network latency not included** — Assumes localhost
4. **CSS selectors not tested** — Could reduce tokens further with targeting

---

## The ROI: Pinchtab for Distributed Agents

**Scenario:** 10,000 web extraction tasks/day, each spawns fresh agent

| Method | Daily Cost | Monthly Cost | Annual Cost |
|--------|-----------|--------------|-------------|
| Snapshot | $1,950 | **$58.50** | **$702** |
| web_fetch | $220 | **$6.60** | **$79.20** |
| **Pinchtab** | **$40** | **$1.20** | **$14.40** |

**Pinchtab savings:**
- vs. Snapshot: **$57.30/month** = **$687.60/year**
- vs. web_fetch: **$5.40/month** = **$64.80/year**

---

## Test Data

**Tested sites:**
- BBC.com (portal, low density)
- Corriere.it (news, high density)
- Daily Mail.co.uk (news + features, very high density)

**Methodology:**
- Pinchtab /snapshot with `format=compact&maxTokens=2000`
- Response size measured in bytes
- Tokens: 4 chars ≈ 1 token
- Agent OH: ~500 tokens (system prompt + task)

**Test date:** February 27, 2026
**OpenClaw version:** 2026.2.23

---

## Comparison with Previous Methods

| Aspect | Snapshot | web_fetch | Pinchtab (Clean Slate) |
|--------|----------|-----------|----------------------|
| **Total tokens (w/ agent)** | 65,083 | 7,325 | **1,400** |
| **Pure service tokens** | 64,583 | 6,825 | 900 |
| **Agent overhead** | 500 | 500 | 500 |
| **Cost/task** | $0.000195 | $0.000022 | **$0.000004** |
| **Monthly (1K tasks)** | $5.85 | $0.66 | **$0.12** |
| **Best for** | Interaction | Text-only | Distributed agents |

---

## Recommendation

**Use Pinchtab for:**
- ✅ Spawning fresh agents per task
- ✅ High-volume distributed extraction
- ✅ Real Chrome rendering required
- ✅ Token efficiency critical

**Use web_fetch if:**
- ✅ Agent already running (context paid)
- ✅ Text-only, no JS rendering
- ✅ Simplicity preferred

**Use Snapshot if:**
- ✅ Interactive workflows (clicking, forms)
- ✅ Full page structure needed
- ✅ Long-lived agent (context sunk)

---

## For pinchtab.com Copy (Distributed Agent Angle)

> **46x lighter than snapshots when agents spawn fresh.**
>
> Running 1,000 extraction tasks/day?
> - Snapshot: **$5.85/month**
> - **Pinchtab: $0.12/month** = **$69/year savings**
>
> Plus: Real Chrome rendering. Agent-optimized. Built for scale.
