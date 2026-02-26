# Pinchtab — TODO

**Philosophy**: 12MB binary. HTTP API. Minimal deps. Internal tool, not a product.

---

## Quality Improvements

### Code Quality
- [ ] **proxy_ws.go proper HTTP** — Replace raw `backend.Write` of HTTP headers with proper `http.Request` construction for better standards compliance.
- [ ] **humanType global rand** — Accept `*rand.Rand` parameter instead of global variable for better testability and concurrency safety.

### Feature Enhancements
- [ ] **Ad blocking** — Basic tracker blocking for cleaner snapshots (block common analytics/ad domains).
- [ ] **API Naming Consistency** — Clarify profile vs instance distinction:
  - Profile = Chrome profile directory (stable 12-char hex ID)
  - Instance = running Pinchtab process (composite ID like "name-port")
  - Option: Standardize on profile IDs throughout API

---

## Known Limitations

- **Canvas noise in headless** — `toDataURL()` returns identical data in headless Chrome. This is a Chrome limitation that only affects `full` stealth mode.

---

## Not Doing
Desktop app, plugin system, proxy rotation, SaaS, Selenium compat, MCP protocol,
cloud anything, distributed clusters, workflow orchestration.