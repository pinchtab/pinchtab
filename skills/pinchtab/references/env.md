# PinchTab Environment Variables

This reference is intentionally narrow.

For agent workflows, most runtime behavior should be configured through `config.json` or the `pinchtab config` commands, not environment variables.

## Agent-relevant variables

| Var | Typical use | Notes |
|---|---|---|
| `PINCHTAB_URL` | Point CLI commands at a specific server or instance | Example: `PINCHTAB_URL=http://localhost:9868 pinchtab snap -i -c` |
| `PINCHTAB_TOKEN` | Authenticate CLI or MCP requests to a protected server | Sent as `Authorization: Bearer ...` |
| `PINCHTAB_CONFIG` | Override the config file path | Prefer this over ad hoc env overrides when automating |
| `PINCHTAB_BIND` | Override bind address for the main server | Operator-level knob; avoid exposing beyond localhost without a token |
| `PINCHTAB_PORT` | Override the main server port | Main server only, not per-instance targeting |

## What is intentionally not listed

- Browser tuning should generally live in `config.json`, not in ad hoc env vars.
- Internal process wiring and inherited env passthrough are implementation details, not part of the skill contract.

## Recommended default

For most agent tasks, the only variables you need are:

```bash
PINCHTAB_URL=http://localhost:9867
PINCHTAB_TOKEN=...
```

Everything else should be handled through profiles, instances, and config.
