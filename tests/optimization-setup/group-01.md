# Group 1: First-Use Reading & Interaction

A minimal but real walkthrough of the agent commands the README features in "AI Agent Automation": navigate, snap, click, text, fill, press. Reuse the tab ID captured in 0.1 for every step.

### 1.1 Read the wiki index
Navigate to `http://localhost:$FIXTURE_PORT/wiki.html` and list the categories with their article counts.

**Verify**: Output names at least two categories with their counts (e.g. `Programming Languages: 12 articles`, `Network Protocols: 8 articles`). The marker `VERIFY_WIKI_INDEX_55555` appears somewhere in the page text.

### 1.2 Click a link by accessibility ref
From the wiki index, take a snapshot, find the `Go (programming language)` link, and click it via its ref (e.g. `e8`). Use `--wait-nav` so the click resolves the new page before returning.

**Verify**: The current tab is now on `wiki-go.html` with title `Go Language — Encyclopedia`. No `evaluate` workaround was needed — `snap` + `click <ref>` is the expected pattern.

### 1.3 Extract structured data from a table
On the Go article page, read the infobox and answer: who designed Go, and in what year did it first appear?

**Verify**: Answer contains at least one of `Robert Griesemer`, `Rob Pike`, `Ken Thompson`, AND the year `2009`.

### 1.4 List content with `--full`
Navigate to `http://localhost:$FIXTURE_PORT/articles.html` and list every article headline. The default `text` command applies a Readability filter and will drop most of the list — use `text --full` for list/grid pages.

**Verify**: Found all three headlines:
- `The Future of Artificial Intelligence`
- `Climate Action in 2026: Progress and Challenges`
- `Mars Colony: First Steps`

The marker `VERIFY_ARTICLES_PAGE_67890` appears somewhere in the page text.

### 1.5 Fill a form and submit with `press`
Navigate to `http://localhost:$FIXTURE_PORT/login.html`, fill the username and password fields, and submit by pressing Enter on the password input. This exercises `fill` and `press` — the two interaction commands featured in the README's "AI Agent Automation" example that aren't covered by 1.2.

Steps:
1. Nav to `login.html` and verify the marker `VERIFY_LOGIN_PAGE_77777` is present.
2. `snap` to get refs for the username and password inputs.
3. `fill <userRef> admin` and `fill <passRef> password123` (these are valid credentials baked into the fixture).
4. `press <passRef> Enter` (submit by keystroke, not by clicking the button — covers a different code path).
5. Read the page after submit.

**Verify**: After submit, the page text contains `VERIFY_LOGIN_SUCCESS_DASHBOARD`. If it instead shows the login form again, the fill/press flow regressed — flag the failure.

---
