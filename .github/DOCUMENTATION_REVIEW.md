# Documentation Review Guide

## Purpose

Code is the source of truth. This guide helps agents audit, validate, and maintain documentation to ensure it stays in sync with the codebase.

Use this document when asking agents to review documentation for accuracy, completeness, and consistency.

---

## Quick Summary

When asked to review documentation, an agent should:

1. **Verify all examples match current code behavior**
2. **Check doc structure against actual codebase**
3. **Update or remove outdated content**
4. **Report findings and improvements**

---

## Detailed Review Process

### Phase 1: Code-to-Docs Validation

#### 1.1 Check All Examples

For **every code example** in `/docs`:

- [ ] **Run the example** (if it's executable)
  - Does it produce the expected output?
  - Does it work with current API/CLI?
  - Are environment variables correct?

- [ ] **Verify API endpoints**
  - Do endpoints still exist? (`GET /snapshot`, `POST /action`, etc.)
  - Do request/response formats match?
  - Are all parameters documented?
  - Are deprecated endpoints removed?

- [ ] **Verify CLI commands**
  - Do commands still exist? (`pinchtab config set`, `pinchtab health`, etc.)
  - Do they work as documented?
  - Are flags/options correct?
  - Are deprecated commands removed?

- [ ] **Verify configuration values**
  - Do env vars still exist? (`CHROME_EXTENSION_PATHS`, `BRIDGE_PORT`, etc.)
  - Are default values correct?
  - Are deprecated config options removed?

#### 1.2 Check Content Accuracy

For **every paragraph, section, and claim** in `/docs`:

- [ ] **API behavior** — Does it accurately describe current behavior?
- [ ] **Performance notes** — Are benchmarks/timings still valid?
- [ ] **Stealth/security claims** — Do they match current implementation?
- [ ] **Feature availability** — Is feature still implemented?
- [ ] **Limitations** — Are noted limitations still present?
- [ ] **Requirements** — Are tool versions, Chrome versions, etc. correct?

#### 1.3 Cross-Check Against Code

Use the code as reference:

```
For each doc section:
1. Find the corresponding code file(s)
2. Compare doc description with actual implementation
3. If different → docs are WRONG
4. If missing → docs are INCOMPLETE
5. If outdated → docs are STALE
```

**Example:**
- Doc says: "fill action with refs sets the input value"
- Code says: `FillByNodeID()` exists → docs are CORRECT ✅
- Code says: no `FillByNodeID()` → docs are WRONG ❌

---

### Phase 2: Structure Review

#### 2.1 Check Documentation Structure

**File structure should match codebase organization:**

```
docs/
├── api/
│   ├── endpoints/
│   │   ├── navigation.md
│   │   ├── snapshot.md
│   │   ├── actions.md
│   │   ├── find.md
│   │   ├── evaluate.md
│   │   └── ...
│   └── ...
├── cli/
│   ├── management.md
│   ├── config.md
│   └── ...
├── architecture/
│   └── ...
└── index.json
```

**Verify:**
- [ ] All major API endpoints documented?
- [ ] All CLI commands documented?
- [ ] Organization makes sense?
- [ ] Groups reflect actual functionality?
- [ ] No duplicate content?
- [ ] No orphaned docs (not linked from index)?

#### 2.2 Check index.json

**Verify index.json structure:**

```json
{
  "sections": [
    {
      "title": "API",
      "path": "api/",
      "items": [
        {
          "title": "Endpoints",
          "path": "api/endpoints/",
          "items": [
            { "title": "Navigation", "path": "api/endpoints/navigation.md" },
            { "title": "Snapshot", "path": "api/endpoints/snapshot.md" },
            ...
          ]
        }
      ]
    }
  ]
}
```

**Checks:**
- [ ] All sections map to actual directories?
- [ ] All items map to actual files?
- [ ] No broken paths?
- [ ] Nesting depth reasonable (not too deep)?
- [ ] Titles are accurate and clear?
- [ ] Order makes sense (API before examples)?

---

### Phase 3: Report Findings

#### If Changes Were Made

**Create a PR with:**

1. **Summary of fixes**
   ```markdown
   ## Documentation Fixes

   - Updated /docs/api/endpoints/fill.md to document FillByNodeID support (fixes #114)
   - Removed deprecated /docs/api/endpoints/old-nav.md (no longer exists in code)
   - Fixed /docs/cli/config.md examples to use new config set syntax
   - Updated Chrome requirement from 144 to 145+ for extension support
   ```

2. **List of possible improvements** (as comments in PR)
   ```markdown
   ## Suggested Improvements

   - [ ] /docs/architecture/ could use a "bridge lifecycle" diagram
   - [ ] /docs/api/endpoints/actions.md could add more examples
   - [ ] /docs/cli/config.md could show YAML format examples
   ```

3. **PR description**
   ```markdown
   ## Documentation Review — Code Alignment

   Verified all examples, API endpoints, CLI commands, and config values against current code.

   ### Changes Made
   - X examples updated to match current API
   - X endpoints documented/updated
   - X CLI commands verified
   - X deprecated content removed

   ### No Changes Needed
   - API behavior accurate
   - Structure organized
   - Examples working

   ### Improvements Suggested
   See comments in PR for enhancement requests.
   ```

#### If No Changes Needed but Improvements Found

**Create a GitHub issue:**

```markdown
Title: docs: enhancement - add examples for [feature]

Body:
## Suggestion

The documentation could be improved by:

1. Adding example for feature X (current docs only show basic usage)
2. Adding "Common Patterns" section to endpoint Y
3. Adding troubleshooting section to CLI guide

## Reasoning

Users asking "how do I..." suggests these are common use cases.

## References

- See /docs/api/endpoints/[endpoint].md
- See /docs/cli/[command].md
```

---

## Checklist for Agents

When given the task "Review documentation", verify:

### Code Accuracy
- [ ] All examples are tested and working
- [ ] All API endpoints documented correctly
- [ ] All CLI commands documented correctly
- [ ] All config options documented correctly
- [ ] No deprecated content remains
- [ ] All current features documented

### Structure
- [ ] index.json paths are correct
- [ ] Directory structure makes sense
- [ ] No orphaned docs
- [ ] Grouping still makes sense
- [ ] Navigation depth reasonable

### Output
- [ ] If changes: Create PR with summary + improvements list
- [ ] If no changes but improvements: Create GitHub issue with enhancement request
- [ ] Always: Note which docs were spot-checked and results

---

## Common Documentation Mistakes to Fix

❌ **Outdated examples:**
```bash
# OLD (deprecated)
pinchtab nav https://example.com   # ← This command was removed

# NEW (correct)
curl -X POST http://localhost:9867/navigate \
  -d '{"url":"https://example.com"}'
```

❌ **Wrong API format:**
```javascript
// OLD (incorrect)
response = await fetch('/action', {
  body: JSON.stringify({action: 'click', ref: 'e5'})
})

// NEW (correct)
response = await fetch('/action', {
  body: JSON.stringify({kind: 'click', ref: 'e5'})
})
```

❌ **Missing new features:**
```markdown
<!-- Missing in docs -->
<!-- Code has FillByNodeID support for fills with refs -->
<!-- Docs should say: "fill supports both selectors and refs" -->
```

❌ **Incorrect config examples:**
```bash
# OLD (deprecated env var name)
export BRIDGE_NAV_TIMEOUT=30

# NEW (correct)
export PINCHTAB_NAVIGATE_TIMEOUT=30
```

---

## Documentation File Locations

**Root docs location:** `/docs`

**Key files to check:**

- `/docs/index.json` — Navigation/structure
- `/docs/README.md` — Overview
- `/docs/api/endpoints/*.md` — API documentation
- `/docs/cli/*.md` — CLI documentation
- `/docs/configuration.md` — Configuration reference
- `/docs/architecture/*.md` — Architecture/design docs
- `/docs/examples/*.md` — Usage examples

---

## When to Ask for Documentation Review

**Ask for review:**
- After major feature additions (new endpoints, CLI commands)
- After refactoring (config structure changes, API changes)
- After bug fixes affecting documented behavior
- Periodically (every quarter) for general sync-up
- Before major releases (ensure nothing is stale)

**Example requests:**

> "Review documentation against current code. Check all API examples, CLI commands, and config values. Update docs to match code or remove outdated content. Create a PR with a summary of changes."

> "Audit /docs for accuracy. Update any incorrect examples, remove deprecated features, fix broken index.json paths. Create an issue with improvement suggestions if no changes needed."

---

## Quality Gates

A documentation review is **complete** when:

✅ All examples are tested and working
✅ All API endpoints match code implementation
✅ All CLI commands match code implementation
✅ All config options match code implementation
✅ No deprecated content remains
✅ index.json structure is accurate
✅ Documentation structure reflects codebase
✅ Either: PR with fixes submitted, OR Issue with improvements created

---

## Example Review Output (PR)

```markdown
## Documentation Review — Code Alignment

All examples, endpoints, and CLI commands verified against current codebase.

### ✅ Changes Made (3 files)

1. **docs/api/endpoints/actions.md**
   - Updated fill example to show ref support (new in PR #119)
   - Added FillByNodeID code path documentation

2. **docs/cli/config.md**
   - Fixed config set examples (new syntax with PR #120)
   - Added YAML output format example

3. **docs/api/endpoints/find.md**
   - Added Find endpoint documentation (new in feat/allocation-strategies)

### ✅ Verified (No Changes Needed)

- All navigate endpoint examples working
- All snapshot filters documented correctly
- All action kinds (click, type, press, etc.) verified
- Config sections (server, chrome, orchestrator) accurate
- Environment variable names current

### 💡 Suggested Improvements

See comments below for enhancement requests (low priority):
- [ ] Add "common patterns" section to actions.md
- [ ] Add troubleshooting FAQ to cli/config.md
- [ ] Create /docs/examples/multi-step-workflow.md

### Review Stats

- Examples tested: 12/12 ✅
- Endpoints checked: 8/8 ✅
- CLI commands checked: 5/5 ✅
- Config sections checked: 4/4 ✅
- Deprecated content found: 0 ✅
- Structure issues found: 0 ✅
```

---

## Related Documents

- [DEFINITION_OF_DONE.md](./DEFINITION_OF_DONE.md) — PR checklist
- [LABELING_GUIDE.md](./LABELING_GUIDE.md) — Issue labeling guide

---

Last updated: 2026-03-04
