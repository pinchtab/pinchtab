# Speed Performance Testing

**Goal:** Measure and track endpoint latency so we know where Pinchtab is fast, where it's slow, and whether regressions creep in.

---

## Methodology

### How to Measure

Use `curl` timing or the benchmark script below. All times in milliseconds.

```bash
# Single endpoint timing
curl -o /dev/null -s -w "%{time_total}" localhost:9867/snapshot | awk '{printf "%.0fms\n", $1*1000}'

# With breakdown (DNS, connect, TTFB, total)
curl -o /dev/null -s -w "connect:%{time_connect}s ttfb:%{time_starttransfer}s total:%{time_total}s\n" localhost:9867/snapshot
```

### What to Measure

| Endpoint | Notes |
|----------|-------|
| `GET /health` | Baseline — should be < 1ms |
| `GET /snapshot` | Primary interface — most latency-sensitive |
| `GET /snapshot?filter=interactive` | Should be similar to full snapshot |
| `GET /snapshot?format=text` | Formatting overhead |
| `GET /text` | Includes readability extraction time |
| `GET /screenshot` | Render + encode time |
| `GET /screenshot?raw=true` | Without base64 |
| `POST /action {"kind":"click","ref":"eN"}` | Action execution time |
| `POST /navigate {"url":"..."}` | Includes page load wait |
| `GET /tabs` | Should be instant |
| `GET /cookies` | CDP call overhead |

### Test Conditions

Always document:
- **Headless vs headed** (headless is typically faster)
- **Page state** (what's loaded when measuring snapshot/text)
- **Machine specs** (CPU, RAM, OS)
- **Chrome version**
- **Number of open tabs** (affects memory pressure)

---

## Benchmark Script

```bash
#!/bin/bash
# speed-benchmark.sh — Measure endpoint latency
# Usage: ./speed-benchmark.sh [port] [iterations]

PORT=${1:-9867}
ITERS=${2:-10}
BASE="http://localhost:$PORT"

echo "Pinchtab Speed Benchmark — $ITERS iterations per endpoint"
echo "Port: $PORT"
echo ""

# Navigate to a standard page first
curl -s -X POST "$BASE/navigate" -d '{"url":"https://example.com"}' > /dev/null
sleep 2

endpoints=(
  "GET /health"
  "GET /tabs"
  "GET /snapshot"
  "GET /snapshot?filter=interactive"
  "GET /snapshot?format=text"
  "GET /text"
  "GET /screenshot?raw=true"
)

echo "| Endpoint | Min | Avg | Max | P95 |"
echo "|----------|-----|-----|-----|-----|"

for ep in "${endpoints[@]}"; do
  method=$(echo "$ep" | cut -d' ' -f1)
  path=$(echo "$ep" | cut -d' ' -f2)
  
  times=()
  for i in $(seq 1 $ITERS); do
    ms=$(curl -o /dev/null -s -w "%{time_total}" "$BASE$path" | awk '{printf "%.0f", $1*1000}')
    times+=("$ms")
  done
  
  # Sort and compute stats
  sorted=($(printf '%s\n' "${times[@]}" | sort -n))
  min=${sorted[0]}
  max=${sorted[-1]}
  sum=0; for t in "${times[@]}"; do sum=$((sum + t)); done
  avg=$((sum / ITERS))
  p95_idx=$(( (ITERS * 95 / 100) - 1 ))
  [ $p95_idx -lt 0 ] && p95_idx=0
  p95=${sorted[$p95_idx]}
  
  echo "| $path | ${min}ms | ${avg}ms | ${max}ms | ${p95}ms |"
done

# Action timing (needs a ref from snapshot)
echo ""
echo "Action timing (click on example.com link):"
ref=$(curl -s "$BASE/snapshot?filter=interactive" | python3 -c "import sys,json; nodes=json.load(sys.stdin).get('nodes',[]); print(nodes[0]['ref'] if nodes else 'e0')" 2>/dev/null || echo "e0")
for i in $(seq 1 $ITERS); do
  ms=$(curl -o /dev/null -s -w "%{time_total}" -X POST "$BASE/action" -d "{\"kind\":\"click\",\"ref\":\"$ref\"}" | awk '{printf "%.0f", $1*1000}')
  echo "  click: ${ms}ms"
done
```

---

## Baseline Expectations

From previous benchmarks (v0.2.0, headless, MacBook Pro M-series):

| Endpoint | Expected Latency |
|----------|-----------------|
| `/health` | < 1ms |
| `/tabs` | < 5ms |
| `/snapshot` | 20-30ms |
| `/snapshot?filter=interactive` | 20-30ms |
| `/text` | 5-10ms |
| `/screenshot` | 150-250ms |
| `/action` (click) | 5-10ms |
| `/navigate` | 1-5s (page load dependent) |

### Regression Thresholds

Flag a regression if any endpoint is **> 2x** the baseline for the same page/conditions.

---

## Performance Targets

| Metric | Target |
|--------|--------|
| Snapshot latency (simple page) | < 50ms |
| Snapshot latency (complex page) | < 200ms |
| Text extraction | < 50ms |
| Screenshot | < 300ms |
| Action (click/type) | < 20ms |
| Startup to first request | < 5s |
| Memory (idle, 1 tab) | < 200MB |
| Memory (3 tabs, active) | < 500MB |
