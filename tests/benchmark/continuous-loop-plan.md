# Continuous Optimization Loop — Plan

## Directive

Execute 250 consecutive optimization loops (Runs #9 through #258) against the
PinchTab benchmark harness. Each loop:

1. Picks the easiest most-consistent optimization available from the backlog.
2. Adds **one** new fixture test case to the benchmark (diverse across loops).
3. Implements, tests, and verifies the change.
4. Runs the full baseline + agent benchmark cycle.
5. Logs the outcome rigorously so work is resumable across sessions.

Ultimate goal: make PinchTab the best possible tool for AI agents that automate
the web. Stay inside the existing code conventions; don't introduce new idioms
or refactor adjacent code unless the optimization requires it.

The user has delegated autonomous execution. Don't ask for approval. Keep
going until completion or a true blocker.

---

## One-loop protocol

Each iteration must follow this sequence. Skipping a step risks silent
regressions that poison later loops.

### 1. Plan the loop (in conversation)

- Pick **one optimization** from the backlog below (or a new friction point
  surfaced by the last agent run). Priority order:
  1. Real bug fix (incorrect behavior in CLI or server)
  2. Doc gap that causes agent confusion (SKILL.md, help text)
  3. CLI ergonomic (new flag, better error message)
  4. Fixture test case that exercises under-covered behavior
- Pick **one fixture test case** from the fixture-coverage backlog. Track
  which categories have been covered to maintain diversity.
- Briefly state the decision in the per-loop decision entry (see §5).

### 2. Implement

- Code changes must stay within existing style (naming, error shape, test
  patterns, flag conventions).
- Add or update unit tests alongside any code change.
- For CLI changes: register flag in `cmd_cli_register.go`, plumb through
  `internal/cli/actions/...`, document in `skills/pinchtab/SKILL.md`.
- For server changes: update action in `internal/bridge/...`, update
  `internal/handlers/...` if HTTP contract changes, add tests.
- For fixture cases: add the HTML/JS fixture in
  `tests/benchmark/fixtures/<name>.html`, add the baseline step in
  `BASELINE_TASKS.md`, add the matching agent task in `AGENT_TASKS.md`,
  update `TEST_CASES.md` summary, update the verification-strings table.
- Run `go test ./internal/...` — must pass.
- Rebuild Docker: `cd tests/benchmark && docker compose up -d --build`.

### 3. Baseline run (must be green)

- Initialize reports: `bash scripts/run-optimization.sh`.
- Run all 58 + new-task baseline curls. Record each step via
  `./scripts/record-step.sh --type baseline ...`.
- Target: **100%**. Any failure blocks the loop — diagnose and fix before
  proceeding.

### 4. Agent run (measure impact)

- Dispatch the Explore-style general-purpose subagent with:
  - A terse prompt that lists current agent-facing capabilities.
  - The hard constraint: use `./scripts/pt <args>` only, no helper files.
  - Tell it what changed in this loop so it can exercise it.
  - Ask for totals + tool-use estimate + new friction points (< 400 words).
- Record pass/fail/skip per step via `record-step.sh --type agent`.

### 5. Log the outcome

Append to `results/decisions.md`:

```markdown
## Loop #N — YYYY-MM-DD HH:MM

**Optimization chosen:** <one line>
**Reason:** <why this won the priority sort>
**Fixture case added:** <one line>

**Changes (files):**
- <path> — <one-line summary>

**Baseline:** X/Y (green/red per step ID if any red)
**Agent:** X/Y

**Agent metrics:** tool_uses=N, duration=Ns, tokens=N

**Delta vs previous successful run:**
<one line>

**Friction surfaced for future loops:**
- <one line each>

**Status:** landed | reverted | deferred
```

Update the summary table in `results/optimization_log.md` with the new row.

If this loop's agent metrics improved on the best score, also update
`results/successful_changes.md` and bump `results/best_score.txt`.

### 6. Resumability

At the end of each loop:
- Git status should be the cumulative uncommitted set from the session.
- No `/tmp` state the next loop depends on (except ephemeral Docker container
  state which is fine).
- The plan, decisions log, summary table, and `best_score.txt` are
  self-contained — a fresh Claude session can read them and pick up.

If context is getting tight, finish the current loop fully (don't leave a
half-broken state), then use `ScheduleWakeup` with the autonomous-loop-dynamic
sentinel to continue from the next loop after a pause.

---

## Optimization backlog (seeded from Runs #1–#8 friction reports)

Ranked by (value × ease). Revise each loop.

### Easy / doc-only
- [ ] SKILL.md note: `eval` shares runtime realm across calls — wrap in IIFE
  to avoid `const` collisions.
- [ ] SKILL.md note: `getBoundingClientRect()` returns a DOMRect whose fields
  don't JSON-serialize — project to `{x, y, w, h}` explicitly.
- [ ] SKILL.md note: `/text` skips modal/overlay portals; use `eval
  document.body.innerText` or `snap --selector` when modal verification is
  needed.
- [ ] SKILL.md note: screenshot/PDF `-o` paths land inside the Docker
  container for the benchmark harness.
- [ ] `pinchtab select --help` example showing text fallback.

### Easy / CLI flag additions
- [ ] `pinchtab download --output <path>` — decode base64 and write to a host
  path so benchmark callers don't have to post-process.
- [ ] `pinchtab snap --selector <css>` surfaced in `--help` Long description.

### Moderate / CLI behavior
- [ ] `pinchtab text` — optional `--max-chars N` flag (HTTP already supports
  `maxChars` query param).
- [ ] `pinchtab wait --text "..." --timeout ...` — CLI surface for
  `wait` action (if missing).
- [ ] `pinchtab console --tail` — stream console output.

### Moderate / server
- [ ] Enrich `<option>` nodes in compact snapshot with the `value` attribute
  via a single `DOM.getAttributes` batch when a `<select>` ancestor is
  present. (Partially obviated by the select fallback, but still useful for
  agents inspecting dropdown contents.)
- [ ] Investigate `text:` selector intermittent `context deadline exceeded`
  — add per-call timeout and better retry policy in
  `bridge/action_resolve.go::ResolveTextToNodeID`.
- [ ] Ref recovery confidence — lower confidence when multiple DOM nodes
  share identical text (e.g. repeated "✕" delete buttons).

### Hard / deferred
- [ ] Frame-aware ref resolver for iframes.
- [ ] First-fill-after-nav race — `action_pointer.go` readiness wait audit.

---

## Fixture coverage backlog

Aim for **diverse** test categories across loops. Categories already covered
(Runs #1–#8): Reading, Search, Forms (basic), SPA, Login, E-commerce,
Content+Interaction, Error handling, Export, Modals, Persistence, Multi-page
Nav, Form Validation, Dynamic Content, Data Aggregation, Hover, Scrolling,
Download, iFrame, Dialogs, Async/awaitPromise, Drag.

Uncovered categories to prioritize:

- **Input variants**: file upload, date picker, time picker, color picker,
  range slider, autocomplete/datalist.
- **Form state**: radio groups, checkbox groups, multi-select, fieldset,
  disabled/readonly, required, pattern validation with messages.
- **Dynamic UI**: loading spinners, toast notifications, pagination, tabs,
  accordion, stepper/wizard.
- **Keyboard**: keyboard shortcuts, tab focus order, focus traps.
- **Visual/CSS**: RTL text, very long text, Unicode/emoji, elements with
  CSS transforms / position:fixed / sticky.
- **Accessibility**: ARIA landmarks, ARIA live regions, ARIA expanded/pressed
  state, role=grid, aria-controls relationships.
- **State stores**: localStorage, sessionStorage, cookies, IndexedDB.
- **Browser APIs**: clipboard read/write, geolocation (mocked),
  notifications.
- **Networking**: fetch polling, SSE stream, WebSocket echo, delayed
  response, 5xx retry.
- **Web components**: Shadow DOM (open and closed), slot content, custom
  elements, light DOM fallback.
- **Gestures**: drag-to-reorder list, resize handle, pinch-zoom, double-click
  distinct from two clicks.
- **Observers**: IntersectionObserver trigger, ResizeObserver trigger,
  MutationObserver-driven DOM.
- **Edge cases**: very deep DOM, many siblings, element moved during
  interaction, stale ref recovery.

---

## Success list

`results/successful_changes.md` ranks interventions by measured impact
(primarily tool-use reduction per run, secondarily token reduction, then
qualitative agent feedback). Update after every loop that lands a new
low-water mark.

---

## Stop conditions

- Loop #258 completed and logged.
- User explicitly tells the agent to stop.
- Two consecutive loops fail the baseline and cannot be recovered without
  human intervention.
- Backlog exhausted AND no new friction surfaced for 3 consecutive loops —
  in that case, exit gracefully and post a completion note to the log.
