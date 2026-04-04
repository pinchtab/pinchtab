# Initialization Benchmark Tasks

Measures how efficiently an agent initializes PinchTab and completes real tasks
using only the skill. Record tokens per phase separately.

## Recording

```bash
./scripts/record-phase.sh <phase> <step> <pass|fail> <tokens> "notes"
```

## Environment

- PinchTab: `http://localhost:9867`, token: `benchmark-token`
- Fixtures: `http://fixtures/` (Docker nginx)
- Real sites: GitHub, Wikipedia, Hacker News (stable, predictable content)

---

## Phase 0: Skill Loading

### 0.1 Read the skill once
Read `skills/pinchtab/SKILL.md` once. Stop when you can answer:
1. How do you authenticate?
2. What CLI command navigates to a URL?
3. How do you read page content? (When to use text vs snapshot?)
4. How do you fill a form field?
5. How do you click a submit button on a JS-heavy form?

**Pass if**: All 5 answered correctly without re-reading.
Record tokens used to load + answer.

---

## Phase 1: Server Setup Verification

### 1.1 Health check
Confirm server is running and authenticated in 1 call.

**Pass if**: Health response on first attempt, no retries.

### 1.2 Get token
Find the auth token without guessing.

**Pass if**: Correct token used on first API call.

---

## Phase 2: Fixture Navigation

### 2.1 Navigate and verify home
Navigate to `http://fixtures/` and confirm page loaded.

**Pass if**: Success on first call.

### 2.2 Choose right extraction method
Get the article headlines from `http://fixtures/articles.html`.

**Pass if**: Used `/snapshot` (not `/text`) and got all 3 headlines.
Note: `/text` strips headings — skill should make this clear.

### 2.3 Read structured data
Get the Total Users and Revenue from `http://fixtures/dashboard.html`.

**Pass if**: Correct values extracted on first snapshot call.

---

## Phase 3: Real Site Navigation

### 3.1 Navigate to GitHub repo
Navigate to `https://github.com/pinchtab/pinchtab`.

**Pass if**: Loaded successfully, no IDPI errors.

### 3.2 Extract repo metadata
Read the repo's description, primary language, and star count.

**Pass if**: All 3 facts extracted in ≤ 2 calls (nav + text/snapshot).

### 3.3 Navigate to Wikipedia
Navigate to `https://en.wikipedia.org/wiki/Go_(programming_language)`.

**Pass if**: Loaded successfully.

### 3.4 Extract specific fact
What year was Go first released? (Answer: 2009)

**Pass if**: Correct answer, ≤ 2 calls.

### 3.5 Navigate to Hacker News
Navigate to `https://news.ycombinator.com`.

**Pass if**: Loaded successfully.

### 3.6 List top 3 headlines
Extract the top 3 story titles from the front page.

**Pass if**: 3 valid titles returned, ≤ 2 calls.

---

## Phase 4: Form Interaction (Fixtures)

### 4.1 Fill and submit form
Navigate to `http://fixtures/form.html`, complete and submit:
- Full Name: `Benchmark Agent`
- Email: `bench@test.com`
- Country: United Kingdom
- Subject: Technical Support
- Submit

**Pass if**: Completed in ≤ 6 API calls, confirmed `SUBMISSION_DATA_NAME_BENCHMARK_AGENT`.

### 4.2 Search flow
On `http://fixtures/search.html`, search for "artificial intelligence",
verify result appears.

**Pass if**: ≤ 4 calls (nav, fill, click, verify). No press-Enter mistakes.

---

## Phase 5: Multi-Site Task (End-to-End)

Given only this natural language request, complete it:

> "Find out what the current top story on Hacker News is, then search
> for that topic on the fixtures search page and tell me if any results
> appear."

No re-reading the skill. Use what you learned in Phase 0.

**Pass if**:
- HN top story title extracted
- Fixtures search attempted with that topic
- Result (found/not found) reported
- Total API calls ≤ 8
- Total tokens ≤ 1500

---

## Scoring

| Phase | Description | Token budget | Ideal calls |
|-------|-------------|-------------|-------------|
| 0 | Skill loading | 400 | — |
| 1 | Server setup | 100 | 1 |
| 2 | Fixture nav | 400 | 5 |
| 3 | Real sites | 600 | 6 |
| 4 | Form interaction | 400 | 8 |
| 5 | Multi-site E2E | 1500 | 8 |
| **Total** | | **3400** | **28** |

Score = budget / actual (higher = more efficient skill).

## What "improvement" looks like

After each run, the loop finds the highest-cost phase and asks:
"What in SKILL.md caused extra tokens or wrong turns here?"

| Phase high cost | Likely cause | Fix |
|----------------|-------------|-----|
| 0 | Skill too long to scan | Compress, move key info up |
| 1 | Auth discovery unclear | Add token location to Quick Start |
| 2 | text vs snapshot unclear | Strengthen extraction guidance |
| 3 | IDPI errors on real sites | Document domain allowlist |
| 4 | Press Enter instead of click | Reinforce submit button rule |
| 5 | Re-reading skill for multi-step | Better cross-reference within skill |
