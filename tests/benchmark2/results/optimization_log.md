## Run #3 — 2026-05-16 16:19

| Phase | Budget | Actual | Pass | Root cause |
|-------|--------|--------|------|------------|
| 0 Skill | 400 | 2622 | ✅ | Quick Start still required a long initial scan. |
| 1 Server | 100 | 52 | ✅ | |
| 2 Fixtures | 400 | 1375 | ✅ | `fixtures` host did not resolve from the host-run server, so fallback to `localhost:8080` was needed. |
| 3 Real sites | 600 | 27930 | ❌ | Large real-page `/text` and `/snapshot` reads exploded token usage; content-scan tuning was also required. |
| 4 Forms | 400 | 814 | ✅ | |
| 5 Multi-site | 1500 | 1342 | ✅ | |
| **Total** | **3400** | **34135** | | |

**Highest cost phase**: Phase 3 (27930 tokens, budget 600)
**Root cause type**: missing
**Fix**: Added Quick Start guidance to cap `/text` with `maxChars` on large real pages before requesting more content.
**Commit**: [pending]
