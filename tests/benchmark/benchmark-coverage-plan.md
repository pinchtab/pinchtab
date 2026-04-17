# Benchmark Coverage Plan — Weaknesses & Gaps

Living inventory of PinchTab weaknesses that fall outside the iframe roadmap
(see that separately). Compiled from continuous optimization loops of agent +
baseline feedback. Rank is by *agent pain per loop* × *tractability*.

Update each entry as loops land fixes. Delete entries once they are closed —
this file is a TODO, not an archive.

---

## Section A — Known bugs (reproducible, every run)

| ID | Weakness | Severity | Effort | Current status |
|----|----------|:--------:|:------:|----------------|
| A3 | **First `fill` after `nav` race** — the first `fill` action after a navigation occasionally times out; the retry works. Suggests the readiness wait in `action_pointer.go` isn't quite aligned with when the DOM is actually interactive. | 🟡 mid | 🟡 medium | open |
| A4 | **Ref recovery picks wrong target on repeated-label siblings** — a row of `✕` delete buttons causes the recovery path to silently bind to the wrong row. **Data-loss risk.** Loop #6 caught it deleting the wrong SPA task. | 🔴 high | 🟡 medium | open |
| A5 | **`scroll into view` fails on overlay/sticky elements** — returns `-32000 no layout object` even when the element is visible. Forces `eval`-based click workaround. | 🟡 mid | 🟡 medium | open |

---

## Section B — Missing CLI capabilities

| ID | Missing | Impact | Effort | Status |
|----|---------|:------:|:------:|--------|
| B2 | **Screenshot / PDF `-o` paths write inside container, not host.** Benchmark-wrapper issue; production installs are fine. Needs wrapper `docker cp` or `--host-path` flag. | 🟡 mid | 🟢 easy | open |
| B3 | **`<select multiple>` not supported.** `SelectByNodeID` sets `this.value = v` which is single-select only. Real bug. | 🟡 mid | 🟡 medium | open |
| B4 | **No viewport resize / mobile emulation.** Can't test responsive layouts or mobile-only UI. | 🟡 mid | 🟡 medium | open |
| B6 | **No request interception** — `--block-images` / `--block-ads` work but no arbitrary URL pattern blocking or response mocking. | 🟡 mid | 🔴 hard | open |
| B7 | **No mobile-gesture primitives** — pinch, long-press, two-finger scroll. Niche but will matter. | 🟢 low | 🟡 medium | open |

---

## Section C — Test-coverage holes (ship without verification)

Features PinchTab ships but the benchmark never exercises. No evidence
they actually work for agents in practice.

| ID | Untested feature | Severity | Action |
|----|-----------------|:--------:|--------|
| C1 | **`pinchtab clipboard`** read/write | 🔴 high | Add `clipboard.html` fixture + 2 tasks |
| C2 | **`pinchtab cookies`** get/set | 🔴 high | Add `cookies.html` fixture (auth-cookie flow) + 2 tasks |
| C3 | **`pinchtab storage`** localStorage, sessionStorage, IndexedDB | 🟡 mid | Add `storage.html` fixture + 2 tasks |
| C4 | **`pinchtab network`** request logging/filtering | 🟡 mid | Reuse existing fixture + add tasks that inspect network log |
| C5 | **`pinchtab console`** JS error/warning capture | 🟡 mid | Add `console.html` fixture that logs + throws |
| C6 | **`pinchtab upload`** file upload via sandboxed path | 🔴 high | Add `upload.html` fixture + sandbox-path task |
| C7 | **`pinchtab find`** natural-language element discovery — agents don't know it exists | 🟡 mid | Add 1 task per existing fixture that uses `find` instead of snap+ref |

---

## Section D — Real-world web patterns we don't simulate

Common patterns that will break agents in production but aren't in the
fixture suite. Ranked by frequency in real sites.

| ID | Pattern | Frequency | Notes |
|----|---------|:---------:|-------|
| D1 | **Shadow DOM / web components** | very high | Google, Stripe, modern widgets. `querySelector` doesn't pierce shadow roots. |
| D2 | **Multi-select `<select multiple>`** | high | Any filter/tag picker. Ties to B3. |
| D3 | **File upload (`<input type="file">`)** | high | Gmail, GitHub, forms. Ties to C6. |
| D4 | **Infinite scroll** | high | Twitter, Instagram, news feeds. |
| D5 | **Autocomplete / combobox with suggestions** | high | Google search, Slack picker. |
| D6 | **Date picker** | high | Booking sites. Native `<input type="date">` vs custom. |
| D7 | **Draggable list reorder** | medium | Trello, Notion. Current `drag.html` is zone-drop, not list-reorder. |
| D8 | **Toast / snackbar notifications** | high | Transient content — pairs with `wait --not-text`. |
| D9 | **Tooltips on keyboard focus** | medium | Accessibility-first UIs; current `hovers.html` only tests hover. |
| D10 | **Virtual scrolling** | medium | AG Grid, React Virtualized — off-screen rows not in DOM. |
| D11 | **Inline async form validation** | high | "Checking username availability..." — wait+re-snap cycle. |
| D12 | **Modal with backdrop click-to-close** | high | React-style modals, not JS `alert`. |
| D13 | **Sticky header + table scroll** | medium | Dashboards — scroll-into-view lands under sticky header. |
| D14 | **Copy-to-clipboard button** | high | Ties to C1. |

---

## Section E — Documentation / discoverability gaps

Agents cost 1–2 extra tool calls per run because of these.

| ID | Gap | Fix |
|----|-----|-----|
| E2 | `pinchtab tab list` is wrong — it's bare `pinchtab tab`. Agents keep guessing. | Add "list" alias or a SKILL.md note. |
| E3 | `text --full` vs default Readability trap — agents still default to `text`. | Consider making default retry raw when Readability returns < N chars. |
| E4 | Container-vs-host paths for `-o` flags — no note in `--help`. | Add note to `screenshot` / `pdf` / `download` help text. |
| E6 | Error code reference — `-32000`, `409`, `412` show up without context. | Create `skills/pinchtab/references/errors.md`. |

---

## Section F — Agent-ergonomic gaps

Not bugs — friction that wrapper / docs could eliminate.

| ID | Gap | Fix idea |
|----|-----|----------|
| F1 | No "warm start" hint — agents always do `snap -i -c` first. | SKILL.md: recommend `pinchtab quick <url>` for one-shot tasks. |
| F2 | No "here's what changed" primitive beyond `snap -d`. | `pinchtab wait-and-diff` as a single call. |
| F4 | Agent re-snapshots after every action — ~half the tool calls. | A `click --then-text` / `click --snap-after` one-shot. |

---

## Priority order (recommended execution sequence)

When picking the next continuous-loop target, sort by `(agent pain × value) / effort`. Current order:

1. **C2 — cookies fixture + tasks** — critical untested capability (auth flows)
2. **B3 — multi-select support** — real bug, fixture-driven validation already designed
3. **A4 — ref recovery confidence tuning** — data-loss risk 🔴
4. **D1 — shadow DOM fixture + `>>>` piercing** — biggest real-world pattern absence
5. **A5 — scroll-into-view fallback for layout-less elements**
6. **C1 — clipboard fixture + tasks**
7. **C6 — file upload fixture + sandbox-path integration**
8. **D4 — infinite-scroll fixture**
9. **C3–C5** — storage / network / console coverage
10. **D5–D14** — remaining real-world patterns

Below that threshold: everything is incremental coverage / doc polish.

---

## Iframe coverage — separate roadmap

Covered by the `pinchtab frame` command (landed upstream in PR #471, merged
into this branch).

### Still missing

- **Cross-origin iframes** — `/frame` can't scope into them; fall back to
  `eval` against `iframe.contentDocument` (same-origin-policy permitting).
  Needs OOPIF session attachment (see coverage-plan §3 in the iframe
  functionality survey).
- **Frame navigation within iframe** — no way to `nav` the inner frame
  independently. `Page.navigate` with a `frameId` isn't exposed.
- **Frame discovery command** — no `pinchtab frames` listing; agents must
  know frame names/URLs in advance.
- **`snap -i -c` still filters iframe children** — full `snap` works but
  adds tokens. A frame-aware interactive filter would be nicer.
- **Sandboxed iframes without `allow-same-origin`** — opaque origin, out
  of reach. Same gap as cross-origin.
