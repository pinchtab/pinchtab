# Token Efficiency Test: Corriere.it News Extraction

**Goal:** Extract news titles from corriere.it homepage and measure token cost efficiency.

**Test Date:** [Run date]
**Tester:** [Agent name]
**Environment:** Pinchtab running on localhost:9867

---

## Prerequisites

1. Pinchtab server running:
   ```bash
   pinchtab &
   ```

2. `jq` installed (for JSON parsing + token counting):
   ```bash
   which jq || echo "Install jq: brew install jq"
   ```

3. Token counter script (estimate OpenAI tokens):
   ```bash
   # Rough estimation: ~1 token per 4 chars or 1 token per ~0.75 words
   # For exact counts, use tiktoken (Python) or check OpenAI tokenizer
   ```

---

## Test Steps

### Step 1: Navigate to Corriere.it
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Content-Type: application/json" \
  -d '{"url": "https://www.corriere.it"}'
```

Expected: `{"tabId": "...", "status": "ok"}`

### Step 2: Get News Titles — Method A (Full Snapshot)
```bash
curl http://localhost:9867/snapshot | jq '.nodes | map(select(.role == "heading")) | .[0:5]' > /tmp/corriere-snapshot-full.json
```

**Capture output:**
```bash
curl http://localhost:9867/snapshot > /tmp/corriere-snapshot-full.json
echo "Full snapshot size:"
wc -c < /tmp/corriere-snapshot-full.json
echo "Estimated tokens:"
stat -f "%.0f" /tmp/corriere-snapshot-full.json | awk '{printf("~%.0f tokens (4 chars per token)\n", $1/4)}'
```

### Step 3: Get News Titles — Method B (Compact Snapshot)
```bash
curl "http://localhost:9867/snapshot?format=compact&filter=interactive" > /tmp/corriere-snapshot-compact.json
echo "Compact snapshot size:"
wc -c < /tmp/corriere-snapshot-compact.json
echo "Estimated tokens:"
stat -f "%.0f" /tmp/corriere-snapshot-compact.json | awk '{printf("~%.0f tokens\n", $1/4)}'
```

### Step 4: Get News Titles — Method C (Text Only)
```bash
curl http://localhost:9867/text | jq '.text' > /tmp/corriere-text.txt
echo "Text extraction size:"
wc -c < /tmp/corriere-text.txt
echo "Estimated tokens:"
stat -f "%.0f" /tmp/corriere-text.txt | awk '{printf("~%.0f tokens\n", $1/4)}'
```

### Step 5: Get Specific Headlines with CSS Selector
```bash
curl "http://localhost:9867/snapshot?selector=.article-headline" | jq '.nodes[] | select(.name | length > 10) | {ref: .ref, title: .name}' > /tmp/corriere-headlines.json
wc -c < /tmp/corriere-headlines.json
```

### Step 6: Extract via CLI
```bash
pinchtab nav https://www.corriere.it
sleep 3  # Wait for page load
pinchtab snap --format=compact --filter=interactive | tee /tmp/cli-snapshot.json | wc -c
```

---

## Analysis Template

| Method | Bytes | Est. Tokens | Time | Notes |
|--------|-------|-------------|------|-------|
| Full snapshot | ? | ? | ? | Baseline (all nodes) |
| Compact + filter | ? | ? | ? | ~60% fewer tokens |
| Text only | ? | ? | ? | Cheapest option (~800 tokens) |
| CSS selector | ? | ? | ? | Most efficient (targeted) |
| CLI snapshot | ? | ? | ? | Native pinchtab tool |

---

## Expected Results

Based on typical news site structure:

| Method | Token Cost | Token Savings |
|--------|------------|---------------|
| Full `/snapshot` | ~3,500–5,000 | Baseline |
| `/snapshot?format=compact&filter=interactive` | ~1,500–2,000 | 60–70% savings |
| `/text` (readability) | ~800–1,200 | 80–90% savings |
| `/text?mode=raw` | ~1,000–1,500 | ~70% savings |

---

## Replication for Other Agents

**To run this test as another agent:**

1. **Clone or copy this file** to your workspace
2. **Ensure Pinchtab is running:** `pinchtab &` (or ask host to start it)
3. **Run each step sequentially:**
   ```bash
   bash -x token-efficiency-corriere.md  # Won't work, but shows intent
   ```
   Or manually run each curl command

4. **Compare results:**
   - Save all outputs to `/tmp/corriere-*`
   - Compare file sizes
   - Calculate token savings

5. **Report format:**
   ```
   **Token Efficiency Test: corriere.it**
   - Full snapshot: 4,250 bytes (~1,062 tokens)
   - Compact snapshot: 1,850 bytes (~462 tokens)
   - Text only: 925 bytes (~231 tokens)
   - Savings with compact: 56% fewer tokens
   - Recommended method: /text for reading, /snapshot?format=compact for interaction
   ```

---

## Docker / Remote Setup

If pinchtab isn't local:

```bash
# Replace localhost:9867 with actual host
PINCHTAB_URL="http://pinchtab-server:9867"

curl -X POST $PINCHTAB_URL/navigate \
  -H "Content-Type: application/json" \
  -d '{"url": "https://www.corriere.it"}'
```

---

## Notes

- **Token estimation:** `bytes / 4` is rough. Use tiktoken (Python) for exact counts:
  ```python
  import tiktoken
  enc = tiktoken.encoding_for_model("gpt-4")
  tokens = enc.encode(text)
  print(len(tokens))
  ```

- **Page load time:** Always wait 3+ seconds after navigate before snapshot
- **Caching:** Snapshot refs are stable within a session; re-use them for actions
- **Stealth:** Test with/without `?displayHeaderFooter=false` for cleaner output

---

## Success Criteria

✅ Agent can extract headlines from corriere.it
✅ Agent can measure token cost difference between methods
✅ Agent demonstrates 50%+ token savings with optimized approach
✅ Results reproducible across multiple runs
