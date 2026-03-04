# Labeling Guide

This document describes the labeling system used for issues and PRs in pinchtab. Use this guide when triaging tickets or asking agents to review open issues.

---

## Quick Reference

Every issue should have:
1. **Exactly one Type label** (bug, enhancement, documentation, question, etc.)
2. **One Status label** (ready, in-progress, blocked, fixed-unreleased, needs-investigation)
3. **One Priority label** (high, medium, low) — optional for documentation or questions

---

## Tier 1: Issue Type (Mandatory)

Choose **exactly one**:

| Label | Color | Usage |
|-------|-------|-------|
| `bug` | 🔴 Red | Something isn't working correctly |
| `enhancement` | 🔵 Cyan | New feature or capability request |
| `documentation` | 🔵 Blue | Improvements/additions to README, docs, or code comments |
| `question` | 🟣 Purple | Request for clarification or information |
| `good first issue` | 🟣 Purple | Good for newcomers (in addition to type label) |
| `help wanted` | 🟢 Green | Needs extra attention or expertise |
| `dependencies` | 🔵 Blue | Dependency updates (PR label) |
| `invalid` | 🟡 Yellow | Doesn't seem right; requires clarification |
| `duplicate` | ⚪ Gray | Already exists (close and reference original) |
| `wontfix` | ⚪ White | Deliberate decision not to fix |

### Examples

- **Bug:** "fill action silently no-ops when using refs" → `bug`
- **Enhancement:** "Add CHROME_EXTENSION_PATHS support" → `enhancement`
- **Documentation:** "Update README with new CLI commands" → `documentation`
- **Question:** "How do I configure stealth mode?" → `question`

---

## Tier 2: Status (Recommended)

Choose **one** to show current state:

| Label | Color | Meaning | When to Use |
|-------|-------|---------|------------|
| `status: ready` | 🔵 Blue | Ready to start work | Issue is fully understood, no blockers, waiting for someone to pick it up |
| `status: in-progress` | 🟡 Yellow | Actively being worked on | Someone has claimed the issue and is working on a fix/feature |
| `status: blocked` | 🔴 Red | Blocked by something | Waiting on external dependency, another issue, or decision |
| `status: fixed-unreleased` | 🟢 Green | Fix merged, not in release | PR is merged but feature/fix isn't in a released version yet |
| `status: needs-investigation` | 🔴 Red | Needs debugging/research | Not enough info; requires investigation before work can start |

### Workflow

```
New Issue
    ↓
[Triage] Add Type + Status
    ↓
status: needs-investigation (if unclear) or status: ready (if clear)
    ↓
Developer picks it up
    ↓
status: in-progress
    ↓
PR submitted
    ↓
PR merged
    ↓
status: fixed-unreleased (for bugs) or remove status (for features)
    ↓
Release cut
    ↓
Remove status label (feature is released)
```

---

## Tier 3: Priority (Optional, for bugs/enhancements)

Choose **one** to indicate urgency:

| Label | Color | Level | When to Use |
|-------|-------|-------|------------|
| `priority: high` | 🔴 Red | Critical; blocks users | Security vulnerability, critical bug, high-demand feature |
| `priority: medium` | 🟡 Yellow | Normal; important for roadmap | Important bug or feature; should be done soon |
| `priority: low` | 🟢 Green | Nice to have; can defer | Minor improvement, polish, or edge case |

### Examples

- **High:** "SafePath() fails to block path traversal" (security) → `bug` + `priority: high`
- **Medium:** "Snapshot doesn't work inside iframes" (affects users) → `bug` + `priority: medium`
- **Low:** "Consider migrating to PINCHTAB_* env vars" (nice refactor) → `enhancement` + `priority: low`

---

## Special Labels

### `good first issue`
Add this **in addition to the type label** for issues that are good entry points for new contributors.

**Criteria:**
- Clear problem statement
- Solution is straightforward
- Doesn't require deep knowledge of codebase
- Estimated effort: <4 hours

**Example:** "documentation: Add /find endpoint example to README" → `documentation` + `good first issue`

### `help wanted`
Indicates the issue needs expertise or has been stalled.

**When to use:**
- Needs specific expertise (e.g., "needs Windows testing")
- Issue has been open >2 weeks without progress
- Complex problem that benefits from external input

---

## Decision Tree for Triage

```
New issue arrives
    ↓
1. Is it a valid issue?
   NO  → `invalid` + close
   YES ↓
2. What is it?
   → Bug? `bug` + assess severity → `priority: high|medium|low`
   → New feature? `enhancement` + assess importance → `priority: high|medium|low`
   → Docs missing? `documentation` + no priority needed
   → Unclear? `question` + no priority needed
   ↓
3. Can we start work immediately?
   YES → `status: ready`
   NO  ↓
4. Why not?
   → Need info → `status: needs-investigation`
   → Waiting on something → `status: blocked`
   → Already fixed → `status: fixed-unreleased`
```

---

## Guidelines for Agents

When reviewing open issues, follow this checklist:

- [ ] **Each issue has exactly one Type label** (bug, enhancement, documentation, question)
- [ ] **Bugs have Priority labels** (high, medium, low)
- [ ] **Enhancements have Priority labels** (high, medium, low)
- [ ] **Each issue has a Status label** (ready, in-progress, blocked, fixed-unreleased, needs-investigation)
- [ ] **No duplicate labels** (e.g., two type labels on one issue)
- [ ] **Status matches reality** (e.g., `status: in-progress` has an active PR)
- [ ] **Stale issues reviewed** (e.g., `status: blocked` for >2 weeks should note why)

---

## Current Label Inventory

**Type labels (10):**
- bug, enhancement, documentation, question, good first issue, help wanted, dependencies, invalid, duplicate, wontfix

**Status labels (5):**
- status: ready, status: in-progress, status: blocked, status: fixed-unreleased, status: needs-investigation

**Priority labels (3):**
- priority: high, priority: medium, priority: low

**Code labels (1):**
- javascript

---

## Examples of Well-Labeled Issues

### Example 1: Security Bug
```
Title: SafePath() fails to block path traversal on Windows
Labels: bug, priority: high, status: ready
```
**Why:** Critical security issue, ready to work on, high priority.

### Example 2: Feature Request (Ready)
```
Title: feat: Resource Pool with pluggable allocation strategies
Labels: enhancement, priority: medium, status: ready
```
**Why:** Enhancement, medium priority (important for roadmap), ready to start.

### Example 3: Feature (Blocked)
```
Title: feat: Semantic Element Selection via NLP (/find endpoint)
Labels: enhancement, priority: medium, status: blocked
```
**Why:** Enhancement, medium priority, but blocked (perhaps waiting on design decision).

### Example 4: Bug (Not Ready)
```
Title: click and humanClick fail to trigger Bootstrap dropdown
Labels: bug, priority: medium, status: needs-investigation
```
**Why:** Bug, medium priority, but needs investigation (reproduction steps unclear).

### Example 5: Already Fixed
```
Title: Installation issue
Labels: bug, priority: high, status: fixed-unreleased
```
**Why:** Critical bug, but fix is already merged and waiting for release.

---

## Common Mistakes

❌ **Don't do this:**

| Mistake | Why | Fix |
|---------|-----|-----|
| Two Type labels on one issue | Ambiguous; what is it really? | Choose one type only |
| Status label on closed issue | Status should reflect open work | Remove status label when closing |
| No Priority on bugs | Can't triage effectively | Add priority: high/medium/low to all bugs |
| `status: in-progress` with no PR | Misleading; is someone actually working on it? | Only use when work is actively underway |
| Forgetting Status labels | No visibility into progress | Always add a status label during triage |

---

## Related Documents

- [DEFINITION_OF_DONE.md](./DEFINITION_OF_DONE.md) — Checklist for PRs before merge
- [CONTRIBUTING.md](../CONTRIBUTING.md) — Contribution guidelines (if present)

---

## Last Updated

2026-03-04

---

## Summary

**For agents:** Use this guide to understand and apply labels when triaging issues. Ensure every issue has Type + Status + Priority (if applicable).

**For maintainers:** Review labels during triage and ensure consistency. Use the Decision Tree above as a quick reference.
