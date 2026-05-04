# Agent CLI Test Blueprint — Setup/Server/Config

Reusable test plan for validating the agent-facing UX of the setup, server,
and configuration commands. Run by spawning a fresh general-purpose agent
with only the skill (`./skills/pinchtab/SKILL.md`) and the built binary
(`./pinchtab`) — no other context.

The agent should execute each scenario, capture output, and report:
- What worked.
- What failed (exact command + error).
- What was confusing or required guesswork beyond the skill.
- Suggestions to make commands more agent-friendly.

## Preconditions

- `./pinchtab` exists and is executable.
- `./skills/pinchtab/SKILL.md` is readable.
- No pinchtab server currently running on the configured port.

## Scenarios (run in order)

### Phase 1 — Read-only inspection (no side effects)

1. `./pinchtab` (bare) — expect a hint screen showing `server stopped`,
   `allowedDomains`, `idpi`, plus "Next steps" / "Configure" hints.
2. `./pinchtab --help` — confirm command list is discoverable.
3. `./pinchtab config` — expect a Config overview + "Change config:" hints.
4. `./pinchtab security` — expect Security checks + recommended-defaults
   summary + "Change security:" hints.
5. `./pinchtab daemon` — expect Daemon overview + "Manage daemon:" hints.
6. `./pinchtab daemon --json` — expect parseable JSON with `installed`,
   `running` keys.
7. `./pinchtab config get server.port` — expect plain value on stdout.
8. `./pinchtab config show` — expect structured config output.

### Phase 2 — Background server lifecycle

9. `./pinchtab server --background` — expect JSON on stdout with
   `pid`, `url`, `token`, `logFile`, `pidFile`. Capture all four.
10. `./pinchtab` (bare again) — expect `protected listener`; bare status
    must not send the API token to identify the listener.
11. `./pinchtab health` — expect first line `ok`, then "Next steps" hints.
12. `PINCHTAB_SESSION= ./pinchtab health --json` — expect JSON; verify it
    contains a `security` block with `level`, `allowedDomains`, `idpiEnabled`
    when using the full configured API token rather than an agent session.

### Phase 3 — Sessions + browse end-to-end

13. `./pinchtab session create --agent-id testbot` — capture the session
    token from stdout (must be a single line, just the token). Stderr
    must be empty (no hints).
14. `export PINCHTAB_SESSION=<token>` then
    `./pinchtab nav https://example.com --snap` — expect snapshot.
15. `./pinchtab text` — expect page text from example.com.

### Phase 4 — Stop and verify

16. `./pinchtab server stop` — expect "Stopped background server (pid …)".
17. `./pinchtab` — expect `server stopped` again, matching the Phase 1 baseline.

### Phase 5 — Config introspection & safe mutations

Server can be stopped or running; if you need it for these checks, restart
with `./pinchtab server --background`.

18. `./pinchtab config path` — expect a single line with the config file
    path.
19. `./pinchtab config validate` — expect a "valid" message and exit 0.
20. `./pinchtab config get nonexistent.path` — expect a clear error and
    non-zero exit.
21. `printf 'n\n' | ./pinchtab config set server.port abc` — expect a
    validation warning on stderr, the message `aborted: value not saved`,
    and exit code 1. Verify port is unchanged afterwards.
22. Round-trip a safe value:
    - Read original: `./pinchtab config get instanceDefaults.maxTabs` (record value).
    - Set new: `./pinchtab config set instanceDefaults.maxTabs 25`.
    - Verify: `./pinchtab config get instanceDefaults.maxTabs`.
    - Restore: `./pinchtab config set instanceDefaults.maxTabs <original>`.
    - Verify again.
23. `./pinchtab config schema` — expect a URL.
24. `./pinchtab config schema --print` — expect JSON Schema output (truncate to first 5 lines).

### Phase 6 — Session lifecycle

Server must be running.

25. `./pinchtab session create --agent-id agentA` — capture token A.
26. `./pinchtab session create --agent-id agentB --label "second"` — capture token B.
27. `./pinchtab session list` — expect both sessions visible.
28. `PINCHTAB_SESSION=<tokenA> ./pinchtab session info` — expect agent ID `agentA`.
29. Revoke session B: extract its session ID from `session list` and
    run `./pinchtab session revoke <id>`. Verify with `session list` it's
    gone (or marked revoked).

### Phase 7 — Error cases & edge cases

30. With server stopped: `./pinchtab health` — should fail clearly, not hang.
31. With server stopped: `./pinchtab nav https://example.com` — should
    auto-start the server, wait for the browser instance to come up, and
    return success with a tab id on stdout. Exit code 0.
32. With no PID file: `./pinchtab server stop` — expect a clear
    "no PID file" error, non-zero exit.
33. Start once: `./pinchtab server --background`. Then immediately
    try `./pinchtab server --background` again — expect a "server already
    running (pid X); stop with: pinchtab server stop" error.
34. Stop the server, then run `./pinchtab daemon --json` — verify
    JSON shape and that `running` is false.

### Phase 8 — YOLO background flow

35. Stop any running server first.
36. `./pinchtab server --background -y` — expect JSON same shape as
    scenario 9; the "YOLO" message goes to stderr.
37. `./pinchtab` (bare) — expect `protected listener`; bare status must not
    send the API token to discover guards-down state.
38. `PINCHTAB_SESSION= ./pinchtab health --json | jq '.security.guardsDown'`
    (or grep) — expect `true` when using the full configured API token rather
    than an agent session.
39. `./pinchtab server stop` — clean up.

## Things to deliberately NOT do

- Do not run `security up`, `security down`, `config set`, `config patch`,
  `config init`, `daemon install`, `daemon uninstall`, or anything else
  that mutates the user's persistent config or system.
- Do not delete `~/.pinchtab/` or any of its contents.
- If a step fails, do NOT try to "fix" the user's environment — log the
  failure and continue to the next scenario.

## Report format

For each scenario, report:

```
### Scenario N — <name>
Command: <exact command>
Exit code: <int>
Stdout (verbatim, trimmed):
  <output>
Stderr (verbatim, trimmed):
  <output>
Verdict: pass | fail | confusing
Notes: <one or two sentences about what was unclear or unexpected>
```

End with a "Top issues" section listing the three biggest agent-friendliness
gaps you hit, in priority order.
