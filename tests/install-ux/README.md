# PinchTab onboarding / install-UX persona lane

Agent-driven tests of the **new-user onboarding experience**. Each scenario pairs a user
persona (skill level + intent) with a starting condition and a first-step goal, then a
**blind agent** simulates that user reaching the goal using ONLY the user-facing docs, in a
fresh container. We collect friction and turn it into fixes.

This is NOT a CI unit test — it needs Docker plus an LLM agent runtime (e.g. Claude
subagents) to play the personas. The scenario definitions, container provisioning, and agent
brief here make it repeatable.

## How to run

```bash
# 1. Provision one clean container per persona (each with its starting condition)
tests/install-ux/setup.sh

# 2. For each persona, launch a blind agent with:
#      - tests/install-ux/subagent-context.md  (shared rules + the persona's brief)
#      - the persona's container name (e.g. ptux-sa)
#    The agent operates the container via `docker exec <name> bash -lc '...'`,
#    reads ONLY user-facing docs (README.md, skills/pinchtab/SKILL.md, docs/**),
#    and returns a friction log.

# 3. Tear down
docker rm -f ptux-sa ptux-sb ptux-sc ptux-sd ptux-se ptux-sf
```

## Files
- `PERSONAS.md` — the persona × condition × goal matrix.
- `setup.sh` — provisions the per-persona containers with their starting conditions.
- `subagent-context.md` — shared blind-agent rules + the per-persona briefs to hand each agent.

## What good looks like
Every persona should reach its goal, and — critically — recover from each obstacle using the
tool's OWN output/docs (no guessing, no source-reading). The friction log scores each step
`yes/partial/no` on "could proceed from the messaging alone." See the repo's commit history
for fixes that came out of earlier runs (no-browser guidance, IDPI remediation, session
auto-start, `config --help` discoverability, etc.).
