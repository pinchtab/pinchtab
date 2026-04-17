# PinchTab Environment Variables

This reference is intentionally narrow.

For agent workflows, most runtime behavior should be configured through `config.json` or the `pinchtab config` commands, not environment variables.

## Agent-relevant variables

| Var | Typical use | Notes |
|---|---|---|
| `PINCHTAB_TOKEN` | Authenticate CLI or MCP requests to a protected server | Sent as `Authorization: Bearer ...` |
| `PINCHTAB_CONFIG` | Override the config file path | Prefer this over ad hoc env overrides when automating |
| `PINCHTAB_TAB` | Default tab ID for tab-scoped commands (`snap`, `eval`, `click`, `fill`, `drag`, etc.) | Used when `--tab` isn't passed explicitly. Lets agents `export PINCHTAB_TAB=$(pinchtab nav URL)` once and drop `--tab "$TAB"` from every subsequent command. |

## Targeting remote servers

Use the `--server` CLI flag instead of environment variables:

```bash
pinchtab --server http://192.168.1.50:9867 snap
pinchtab --server https://pinchtab.com snap
```

## What is intentionally not listed

- Browser tuning should generally live in `config.json`, not in ad hoc env vars.
- Internal process wiring and inherited env passthrough are implementation details, not part of the skill contract.

## Recommended default

For most agent tasks, the only variable you need is:

```bash
PINCHTAB_TOKEN=...
```

For multi-step flows on a specific tab, also set `PINCHTAB_TAB` once after
navigating so you don't have to thread `--tab` through every command:

```bash
export PINCHTAB_TOKEN=...
export PINCHTAB_TAB=$(pinchtab nav http://example.com)   # pipe → tabId only
pinchtab snap -i -c                                      # auto-targets $PINCHTAB_TAB
pinchtab eval --await-promise "window.fetchPayload()"
pinchtab click "#submit"
```

Or use agent sessions for per-agent identity and revocability:

```bash
PINCHTAB_SESSION=ses_...
```

When `PINCHTAB_SESSION` is set, the CLI uses `Authorization: Session <token>` instead of bearer auth. The session maps to a specific agentId server-side and can be revoked independently.

Everything else should be handled through config, profiles, instances, and the `--server` flag.
