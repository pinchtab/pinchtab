# Subagent Context

You are running PinchTab optimization tasks. You will be given a set of group files, each containing numbered steps to execute using the PinchTab browser automation tool.

## What to read

1. **PinchTab skill**: `skills/pinchtab/SKILL.md` — full command reference and patterns.
2. **Group files**: `tests/optimization/group-XX.md` — task descriptions and verification markers.

## What NOT to read

- `tests/tools/scripts/baseline.sh` — deterministic baseline; reading it defeats the purpose.
- `tests/tools/scripts/run-optimization.sh` — infrastructure script, not relevant.
- `tests/benchmark/` — separate benchmark lane, not your concern.

## Environment

- PinchTab server: `http://localhost:9867`, token: `benchmark-token`
- Fixtures: `http://fixtures/` (Docker hostname)
- Available fixture pages: `/`, `/wiki.html`, `/wiki-go.html`, `/wiki-python.html`, `/wiki-rust.html`, `/articles.html`, `/search.html`, `/form.html`, `/dashboard.html`, `/ecommerce.html`, `/spa.html`, `/login.html` — plus additional fixture pages referenced in specific group steps.

## Wrapper

Always use `./scripts/pt ...` — never call `pinchtab` directly.

The wrapper executes pinchtab inside the Docker service and forwards `PINCHTAB_TOKEN` and `PINCHTAB_SERVER` automatically.

Multiple subagents run in parallel against the same browser instance. Make sure your commands don't interfere with other agents' work.

## Recording

Each agent writes to its own report file to avoid concurrent-write corruption. Your report file path will be provided as `REPORT_FILE` when you are launched. Pass it on every `step-end` call with `--report-file`.

Record every step result immediately after completion:

```bash
./scripts/runner step-end --report-file "$REPORT_FILE" <group> <step> answer "<what you observed>" <pass|fail|skip> "verification notes"
```

For failures:

```bash
./scripts/runner step-end --report-file "$REPORT_FILE" <group> <step> fail "<what went wrong>" skip ""
```

- `<group>` is the group number (e.g., `0`, `15`, `38`)
- `<step>` is the step number within the group (e.g., `1`, `2`, `3`)
- Keep answers factual — do not self-grade in the answer payload.
- **Quote actual output, don't paraphrase.** The answer field must include the literal text or marker from the tool output. For example, if the server returns `status: ok`, write `status: ok` in the answer — not "Server responded with ok". Verification patterns match against exact substrings, so paraphrasing causes false failures.
- **Always include `UPPER_CASE_MARKER` strings verbatim.** Fixture pages embed verification markers like `SUGGESTIONS_VISIBLE_COUNT_2`, `VERIFY_HOME_LOADED_12345`, `SCROLL_MIDDLE_MARKER`, etc. When you see these in the page content or tool output, copy them exactly into your answer — they are the primary tokens that automated verification matches against.

## Execution approach

1. Read the PinchTab skill to learn available commands.
2. Read the assigned group files.
3. For each step: navigate to the fixture, interact using PinchTab commands, verify the expected markers appear, and record the result.
4. Use your judgment to figure out the right commands — the group files describe WHAT to achieve, not HOW.
