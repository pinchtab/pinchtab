# Token Performance Testing

**Goal:** Measure and track how many LLM tokens Pinchtab endpoints consume per page, so we can optimize for cost.

---

## Methodology

### How to Measure Tokens

Tokens ≈ bytes / 4 (rough estimate for English text). For precise counts, pipe output through a tokenizer (e.g. `tiktoken` for OpenAI, or just use byte count / 4).

```bash
# Measure snapshot tokens (approximate)
curl -s localhost:9867/snapshot | wc -c | awk '{print int($1/4)" tokens"}'

# Measure text tokens
curl -s localhost:9867/text | wc -c | awk '{print int($1/4)" tokens"}'

# Measure interactive-only snapshot
curl -s "localhost:9867/snapshot?filter=interactive" | wc -c | awk '{print int($1/4)" tokens"}'

# Measure text format snapshot
curl -s "localhost:9867/snapshot?format=text" | wc -c | awk '{print int($1/4)" tokens"}'
```

### Test Sites (standard set)

Use these consistently across versions for comparable results:

| Site | Type | Why |
|------|------|-----|
| `https://example.com` | Minimal static | Baseline / sanity check |
| `https://www.google.com` | Search engine | Common use case, forms |
| `https://github.com` | Web app | Auth, dynamic content |
| `https://www.bbc.co.uk` | News site | Heavy content, lots of nav |
| `https://en.wikipedia.org/wiki/Go_(programming_language)` | Reference | Long-form content |
| `https://news.ycombinator.com` | Link list | Repetitive structure |
| `https://x.com` | SPA | Client-rendered, auth-gated |

### Endpoints to Measure

For each site, record tokens for:

| Endpoint | What it measures |
|----------|-----------------|
| `GET /snapshot` | Full accessibility tree (JSON) |
| `GET /snapshot?filter=interactive` | Interactive elements only |
| `GET /snapshot?format=text` | Text format (indented tree) |
| `GET /text` | Readability extraction |
| `GET /text?mode=raw` | Raw innerText |

---

## Test Script

```bash
#!/bin/bash
# token-benchmark.sh — Run against a live Pinchtab instance
# Usage: ./token-benchmark.sh [port]

PORT=${1:-9867}
BASE="http://localhost:$PORT"

SITES=(
  "https://example.com"
  "https://www.google.com"
  "https://github.com"
  "https://www.bbc.co.uk"
  "https://en.wikipedia.org/wiki/Go_(programming_language)"
  "https://news.ycombinator.com"
)

echo "| Site | /snapshot | ?filter=interactive | ?format=text | /text | /text?mode=raw |"
echo "|------|----------|---------------------|-------------|-------|----------------|"

for site in "${SITES[@]}"; do
  # Navigate
  curl -s -X POST "$BASE/navigate" -d "{\"url\":\"$site\"}" > /dev/null
  sleep 3

  # Measure each endpoint
  snap=$(curl -s "$BASE/snapshot" | wc -c | awk '{print int($1/4)}')
  interactive=$(curl -s "$BASE/snapshot?filter=interactive" | wc -c | awk '{print int($1/4)}')
  text_fmt=$(curl -s "$BASE/snapshot?format=text" | wc -c | awk '{print int($1/4)}')
  text=$(curl -s "$BASE/text" | wc -c | awk '{print int($1/4)}')
  raw=$(curl -s "$BASE/text?mode=raw" | wc -c | awk '{print int($1/4)}')

  name=$(echo "$site" | sed 's|https://||' | sed 's|www.||' | cut -d'/' -f1)
  echo "| $name | ~${snap} | ~${interactive} | ~${text_fmt} | ~${text} | ~${raw} |"
done
```

---

## What to Track Per Version

For each release, record:

1. **Token counts per endpoint per site** (the table above)
2. **Comparison vs previous version** (regression or improvement)
3. **Snapshot JSON node count** — `curl -s /snapshot | jq '.nodes | length'`
4. **Text format vs JSON ratio** — how much cheaper is `?format=text`

---

## Optimization Targets

| Metric | Current (v0.3.0) | Target |
|--------|-------------------|--------|
| `/text` on news site | ~3,500 tokens | < 3,000 |
| `/snapshot` vs OpenClaw aria | 3-4x larger | < 2x larger |
| `?filter=interactive` vs full | ~60-75% reduction | maintain |
| `?format=text` vs JSON | ~40-60% reduction | maintain |

### Known Optimization Opportunities

- **Snapshot JSON is verbose** — each node has full property objects. A compact format could halve tokens.
- **`/text` on content-heavy sites** — readability extraction is good but still includes some boilerplate.
- **`?format=text` indentation** — uses spaces; could use minimal indentation to save bytes.
