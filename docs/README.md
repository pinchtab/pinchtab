# Pinchtab Documentation

## Browser Content Extraction Analysis

This folder contains comprehensive benchmarks comparing three web content extraction methods for AI agents: Snapshot (heavyweight), web_fetch (lightweight), and **Pinchtab** (ultra-lightweight).

### Quick Links

**Start here:**
- 📊 **[Browser Extraction Spectrum](./browser-extraction-spectrum.md)** — The complete comparison (snapshot vs. web_fetch vs. Pinchtab)

**Deep dives:**
- 🖥️ **[Default Isolated Browser](./default-isolated-browser.md)** — OpenClaw's semantic snapshot analysis + token costs
- 📝 **[web_fetch Lightweight Analysis](./web-fetch-lightweight.md)** — Text extraction via Readability parser
- 📦 **[snapshot-test-results.zip](./snapshot-test-results.zip)** — Raw test data (BBC, Corriere, Daily Mail)

---

## The Headline

**For 1,000 pages/day:**
- Snapshot: **$5.82/month** (~64K tokens/page)
- web_fetch: **$0.63/month** (~6.8K tokens/page)
- **Pinchtab: $0.11/month** (~1.2K tokens/page) ← **98% savings vs. snapshot**

**Pinchtab wins on:**
- ✅ Token efficiency (90% reduction)
- ✅ Real Chrome rendering (handles JavaScript)
- ✅ Cost at scale ($68/year savings per 1K pages/day)
- ✅ Agent-optimized output (no boilerplate)

**Use Snapshot if:** You need to interact (click, fill forms)
**Use web_fetch if:** Text-only, no JS rendering needed, want simplicity
**Use Pinchtab if:** Real rendering + minimal tokens (the Goldilocks option)

---

## Test Sites

All benchmarks use real-world news sites:
- **BBC.com** — Portal layout, low content density
- **Corriere.it** — News hub, high content density  
- **Daily Mail.co.uk** — News + features, very high density

---

## Files in This Folder

| File | Purpose | Best For |
|------|---------|----------|
| `browser-extraction-spectrum.md` | Complete comparison across all three | Decision-making |
| `default-isolated-browser.md` | Snapshot analysis + architecture | Deep technical understanding |
| `web-fetch-lightweight.md` | web_fetch analysis + use cases | Text extraction scenarios |
| `snapshot-test-results.zip` | Raw data (4.4 KB) | Verification, reproduction |
| Other docs | Architecture, Docker, Chrome lifecycle | General Pinchtab info |

---

## Key Insights

### 1. Snapshot Overhead
OpenClaw's default semantic snapshot includes full DOM + accessibility tree. This is powerful for UI agents but heavy for text extraction: **29x heavier than web_fetch on Corriere.it**.

### 2. web_fetch Sweet Spot
Readability parser removes ~70-90% of boilerplate automatically. Perfect for news articles, blogs, content extraction. **9x cheaper than snapshot**, but can't handle JavaScript-heavy sites.

### 3. Pinchtab's Advantage
Real Chrome rendering + text optimization = best of both worlds. **5.7x cheaper than web_fetch**, still renders JavaScript, optimized for agents.

### 4. Scale Matters
At 1,000+ pages/day, method choice compounds to **$50-60/month savings**. At 10,000 pages/day, you're looking at **$600+/year** difference.

---

## Citation

When referencing these benchmarks:

> Extracted from Pinchtab documentation (Feb 26, 2026). Tested against BBC.com, Corriere.it, Daily Mail.co.uk using OpenClaw v2026.2.23. Token costs based on Claude Sonnet ($3/M input tokens).

---

## Questions?

- **How do I use Pinchtab?** See [pinchtab-architecture.md](./pinchtab-architecture.md) and [docker.md](./docker.md)
- **How is Pinchtab different from browser snapshots?** See [browser-extraction-spectrum.md](./browser-extraction-spectrum.md)
- **What are Pinchtab's stealth levels?** See [agent-optimization.md](./agent-optimization.md)
- **Can I run Pinchtab in Docker?** Yes, see [docker.md](./docker.md)

---

**Last updated:** February 26, 2026 | **OpenClaw version:** 2026.2.23
