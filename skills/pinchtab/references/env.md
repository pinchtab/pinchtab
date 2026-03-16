# PinchTab Environment Variables

This reference is intentionally narrow.

For agent workflows, most runtime behavior should be configured through `config.json` or the `pinchtab config` commands, not environment variables.

## Agent-relevant variables

| Var | Typical use | Notes |
|---|---|---|
| `PINCHTAB_TOKEN` | Authenticate CLI or MCP requests to a protected server | Sent as `Authorization: Bearer ...` |
| `PINCHTAB_CONFIG` | Override the config file path | Prefer this over ad hoc env overrides when automating |

## Targeting remote servers

Use the `--server` CLI flag instead of environment variables:

```bash
pinchtab --server http://192.168.1.50:9867 snap
pinchtab --server https://pinchtab.example.com snap
```

## What is intentionally not listed

- Browser tuning should generally live in `config.json`, not in ad hoc env vars.
- Internal process wiring and inherited env passthrough are implementation details, not part of the skill contract.

## Recommended default

For most agent tasks, the only variable you need is:

```bash
PINCHTAB_TOKEN=...
```

Everything else should be handled through config, profiles, instances, and the `--server` flag.
