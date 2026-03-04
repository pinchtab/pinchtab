# GitHub Governance Documents

This directory contains pinchtab's governance and workflow documents referenced by developers and automated agents.

## Documents

### [DEFINITION_OF_DONE.md](./DEFINITION_OF_DONE.md)
**Purpose:** PR checklist for code quality, testing, and documentation before merging.

**For:** All contributors submitting PRs
**How to use:** Check this list before submitting a PR, or ask agents to verify PRs against it.

**Key sections:**
- Automated checks (CI enforces)
- Manual code quality requirements
- Testing requirements
- Documentation requirements
- Quick checklist for copy/paste

---

### [LABELING_GUIDE.md](./LABELING_GUIDE.md)
**Purpose:** Reference guide for issue and PR labeling to maintain consistent triage.

**For:** Maintainers triaging issues, agents reviewing tickets
**How to use:** Point agents to this guide when asking them to review and label open issues.

**Key sections:**
- 3-tier labeling system (Type → Status → Priority)
- Decision tree for triage
- Guidelines for agents
- Examples of well-labeled issues
- Common mistakes to avoid

---

## Quick Reference

| Document | Checklist For | Used By |
|----------|---------------|---------|
| DEFINITION_OF_DONE.md | PR quality before merge | Developers, agents, reviewers |
| LABELING_GUIDE.md | Consistent issue triage | Maintainers, agents |

---

## How Agents Use These

When asking an agent to help with PRs or issues:

**For PR review:**
> "Review this PR against `.github/DEFINITION_OF_DONE.md` and ensure it meets all requirements."

**For issue triage:**
> "Review open issues and apply labels according to `.github/LABELING_GUIDE.md`. Ensure Type + Status + Priority are consistent."

---

## Maintenance

- **DEFINITION_OF_DONE.md** — Update when code quality standards change
- **LABELING_GUIDE.md** — Update when new labels are added or workflow changes
- This **README.md** — Update when new governance documents are added

---

Last updated: 2026-03-04
