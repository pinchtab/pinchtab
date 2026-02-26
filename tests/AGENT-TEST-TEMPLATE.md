# Agent Test Replication Template

**For agents without Pinchtab to replicate token efficiency testing.**

> **Reference:** Based on [docs/agent-optimization.md](../../docs/agent-optimization.md) — validated Feb 2026 with Corriere.it, BBC News.

## Quick Start

Copy these steps into your session and run sequentially.

---

## Test: Extract News Headlines + Measure Tokens

### Prerequisites
```bash
# 1. Verify pinchtab is running
curl http://localhost:9867/health
# Expected: {"status": "ok"}

# 2. Have jq installed (for JSON parsing)
which jq
```

### Test Steps (Copy + Paste)

**Step 1: Navigate**
```bash
BASE_URL="http://localhost:9867"
curl -s -X POST $BASE_URL/navigate \
  -H "Content-Type: application/json" \
  -d '{"url": "https://www.corriere.it"}' | jq '.tabId'
# Save the tabId for next steps
```

**Step 2: Wait for page load**
```bash
sleep 3
```

**Step 3: Test Method A — Full snapshot (baseline)**
```bash
echo "=== FULL SNAPSHOT ==="
curl -s $BASE_URL/snapshot > /tmp/snap-full.json
SIZE=$(wc -c < /tmp/snap-full.json)
TOKENS=$((SIZE / 4))
echo "Size: $SIZE bytes"
echo "Est. tokens: ~$TOKENS"
```

**Step 4: Test Method B — Compact snapshot (optimized)**
```bash
echo "=== COMPACT + FILTER SNAPSHOT ==="
curl -s "$BASE_URL/snapshot?format=compact&filter=interactive" > /tmp/snap-compact.json
SIZE=$(wc -c < /tmp/snap-compact.json)
TOKENS=$((SIZE / 4))
echo "Size: $SIZE bytes"
echo "Est. tokens: ~$TOKENS"
```

**Step 5: Test Method C — Text only (cheapest)**
```bash
echo "=== TEXT ONLY ==="
curl -s $BASE_URL/text > /tmp/text.json
SIZE=$(wc -c < /tmp/text.json)
TOKENS=$((SIZE / 4))
echo "Size: $SIZE bytes"
echo "Est. tokens: ~$TOKENS"
```

**Step 6: Compare results**
```bash
echo ""
echo "=== COMPARISON ==="
FULL=$(wc -c < /tmp/snap-full.json)
COMPACT=$(wc -c < /tmp/snap-compact.json)
TEXT=$(wc -c < /tmp/text.json)

SAVINGS=$((100 - (COMPACT * 100 / FULL)))
echo "Full snapshot: ~$((FULL / 4)) tokens (baseline)"
echo "Compact snapshot: ~$((COMPACT / 4)) tokens (savings: $SAVINGS%)"
echo "Text only: ~$((TEXT / 4)) tokens (savings: $((100 - (TEXT * 100 / FULL)))%)"
```

---

## Sample Output

```
=== FULL SNAPSHOT ===
Size: 4250 bytes
Est. tokens: ~1062

=== COMPACT + FILTER SNAPSHOT ===
Size: 1850 bytes
Est. tokens: ~462

=== TEXT ONLY ===
Size: 925 bytes
Est. tokens: ~231

=== COMPARISON ===
Full snapshot: ~1062 tokens (baseline)
Compact snapshot: ~462 tokens (savings: 56%)
Text only: ~231 tokens (savings: 78%)
```

---

## Instructions for Another Agent

**To replicate this test without any prior knowledge:**

1. **Get the test file:**
   ```bash
   curl -s https://raw.githubusercontent.com/pinchtab/pinchtab/main/tests/AGENT-TEST-TEMPLATE.md
   ```

2. **Run each "Test Steps" command one by one** (they're independent)

3. **Record the token counts** in a table like above

4. **Compare:** 
   - Compact should be ~50-70% smaller than full
   - Text should be ~75-85% smaller than full

5. **Report result:**
   ```
   ✅ Token efficiency verified
   - Full: 1062 tokens
   - Compact: 462 tokens (56% savings)
   - Text: 231 tokens (78% savings)
   - Recommended: Use /text for reading, /snapshot?format=compact&filter=interactive for interaction
   ```

---

## Troubleshooting

| Issue | Solution |
|-------|----------|
| `curl: (7) Failed to connect` | Pinchtab not running. Run `pinchtab &` first |
| `jq not found` | Install: `brew install jq` (macOS) or `apt-get install jq` (Linux) |
| `404 not found` | Wrong endpoint. Check: `curl http://localhost:9867/health` |
| Token counts seem off | Use `echo $TEXT \| wc -c` for exact byte count; divide by 4 for rough token estimate |

---

## For Exact Token Counting

Use Python + tiktoken (OpenAI's official tokenizer):

```python
import tiktoken
import json

with open('/tmp/snap-full.json') as f:
    data = json.load(f)
    text = json.dumps(data)

enc = tiktoken.encoding_for_model("gpt-4")
tokens = enc.encode(text)
print(f"Exact tokens: {len(tokens)}")
```

Or online: https://platform.openai.com/tokenizer

---

## Next Steps

After confirming token efficiency:

1. **Extract specific headlines:**
   ```bash
   curl -s "$BASE_URL/snapshot?format=compact&filter=interactive" | \
     jq '.nodes[] | select(.role == "heading") | {ref, title: .name}' | \
     head -10
   ```

2. **Use diff mode for repeated snapshots:**
   ```bash
   curl -s "$BASE_URL/snapshot?diff=true&format=compact" | wc -c
   # Much smaller (only changes shown)
   ```

3. **Test on other news sites:** Replace corriere.it with any site and rerun

---

## BONUS: Pattern-Driven Approach (93% Savings) ✅

**This is the VALIDATED optimal method from docs/agent-optimization.md**

Instead of exploring different approaches, use this exact pattern:

```bash
curl -X POST http://localhost:9867/navigate \
  -H "Content-Type: application/json" \
  -d '{"url": "https://www.corriere.it"}' && \
sleep 3 && \
curl http://localhost:9867/snapshot | \
jq '.nodes[] | select(.name | length > 15) | .name' | \
head -30
```

**Why it works:**
- Navigate + 3-second wait = full accessibility tree (2,645 nodes on Corriere.it)
- jq filter = extract only headline-length text
- `head -30` = limit output (saves tokens)

**Token cost:** ~272 tokens (vs 3,842 exploratory)

**Lesson:** Clear instructions beat exploration by 14.2x ✅
