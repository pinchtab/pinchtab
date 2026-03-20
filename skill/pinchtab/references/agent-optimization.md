# Agent Optimization Reference

Practical playbook for token-efficient, reliable browser automation with Pinchtab.

## Cheapest-Path Decision Table

Pick the cheapest method that gives you what you need:

| Need | Method | Typical Tokens | When to Use |
|------|--------|---------------|-------------|
| Read page content | `text` | ~800 | Extracting articles, checking text on page |
| Find interactive elements | `snap -i -c` | ~1,800 | Locating buttons, links, inputs to act on |
| See what changed | `snap -d` | varies | Multi-step workflows (only diffs returned) |
| Full page understanding | `snap` | ~10,500 | First visit, complex layouts |
| Visual verification | `screenshot` | ~2K (vision) | Confirming visual state, debugging |
| Export page | `pdf` | 0 (binary) | Saving page — no token cost |

**Strategy:** Start with `snap -i -c`. Use `snap -d` on subsequent snapshots. Use `text` when you only need readable content. Full `snap` only when the cheaper options don't give you enough context.

## The 3-Second Wait Pattern

**Critical:** Always wait 3+ seconds after `nav` before taking a snapshot.

```bash
pinchtab nav https://example.com
sleep 3
pinchtab snap -i -c
```

**Why:** Chrome's accessibility tree lags behind DOM rendering. Without the wait, you get only 1 node (`RootWebArea`) instead of 2,000+.

**Dynamic/SPA sites:** Increase to 5–10 seconds, or take multiple snapshots with waits between state changes.

## Diff Snapshots for Multi-Step Workflows

After the first full snapshot, use `-d` (diff) to see only what changed:

```bash
pinchtab snap -i -c          # Full snapshot (first time)
pinchtab click e5             # Act on an element
sleep 1
pinchtab snap -d              # Only changes since last snapshot
```

This reduces token usage dramatically in multi-step flows (login → navigate → fill form → submit).

## Recovery Patterns

| Error | Meaning | Recovery |
|-------|---------|----------|
| **Connection refused** | Pinchtab server not running | Start with `pinchtab &` or check `PINCHTAB_URL` |
| **403 Forbidden** | Auth token required or invalid | Set `PINCHTAB_TOKEN` or check `BRIDGE_TOKEN` |
| **401 Unauthorized** | Token mismatch | Verify token matches the server's `BRIDGE_TOKEN` |
| **Stale refs** | Page changed since last snapshot | Take a new snapshot before acting |
| **Bot detection** | Site blocked automated browser | Try `BRIDGE_STEALTH=full`, rotate fingerprint, add waits |
| **Only 1 node** | Snapshot taken too early | Wait 3+ seconds after navigation |
| **Timeout** | Action or navigation took too long | Increase `BRIDGE_TIMEOUT` or `BRIDGE_NAV_TIMEOUT` |

## Token Savings: Real Numbers

| Approach | Tokens | Notes |
|----------|--------|-------|
| Exploratory (agent discovers pattern) | ~3,842 | Multiple retries, full tree parsing |
| Pattern-driven (prescriptive instructions) | ~272 | **14× better** — direct execution |

**Key insight:** Give agents exact commands. Exploratory agent loops waste 93% of tokens on reasoning, failed filters, and narration.

## Prescriptive Scraping Template

For agents extracting headlines/data:

```bash
pinchtab nav TARGET_URL
sleep 3
pinchtab snap -i -c | jq '.nodes[] | select(.name | length > 15) | .name' | head -30
```

Replace `TARGET_URL`. Limit output with `head -N`. Filter threshold (`length > 15`) skips buttons and labels.

## Lite Engine Boundaries

`--engine lite` uses a minimal rendering pipeline:

| ✅ Works | ❌ Doesn't work |
|----------|-----------------|
| Page navigation | Complex JS-heavy SPAs |
| Text extraction | Sites requiring full JS execution |
| Simple form fills | Dynamic content after initial load |
| Screenshot capture | Sites with anti-bot JS challenges |

Use lite engine for read-heavy, simple pages. Switch to default engine for anything interactive or JS-dependent.

## Tips

- **Block images** for read-heavy tasks: `pinchtab nav URL --block-images` or `BRIDGE_BLOCK_IMAGES=true`
- **Scope snapshots** to reduce tokens: `snap -s main` limits to `<main>` element
- **Cap output**: `snap --max-tokens 2000` truncates to ~2K tokens
- **Batch actions**: Use `pinchtab action -f actions.json` to send multiple steps in one call
- **Always pass `--instance`** explicitly in multi-instance setups
