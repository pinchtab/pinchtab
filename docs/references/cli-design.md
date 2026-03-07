# CLI Design Guide

This page describes the CLI that exists today in `cmd/pinchtab/cmd_cli.go` and `cmd/pinchtab/main.go`.

## Process Modes

Pinchtab has three startup modes:

```bash
pinchtab          # Start full server (default)
pinchtab server   # Start full server explicitly
pinchtab bridge   # Start bridge-only runtime
```

The CLI commands below are client commands. They expect a running full server unless noted otherwise.

## Core Rule

The current CLI is split into three layers:

1. startup and utility commands
2. top-level browser control commands
3. explicit instance and tab management commands

The important constraint is that the CLI does **not** support a global `--instance` flag today.

If you need to target a specific instance:
- create or inspect it with `pinchtab instance ...`
- open a tab inside it with `pinchtab instance navigate <instance-id> <url>` or the HTTP API
- then operate on the returned tab ID with `pinchtab tab ...`

## Startup And Utility Commands

These commands are handled outside the main HTTP CLI dispatcher:

```bash
pinchtab --version
pinchtab help

pinchtab config init
pinchtab config show
pinchtab config path
pinchtab config validate

pinchtab connect <profile>
pinchtab connect <profile> --json
pinchtab connect <profile> --dashboard http://localhost:9867
```

Notes:
- `connect` resolves a running profile to its bridge URL via `/profiles/{id}/instance`
- there is no CLI command for `attach` yet; attach is API-only for now

## Top-Level Browser Control

These commands talk to the server shorthand endpoints:

```bash
pinchtab nav <url>
pinchtab snap
pinchtab click <ref>
pinchtab type <ref> <text>
pinchtab fill <ref|selector> <text>
pinchtab press <key>
pinchtab hover <ref>
pinchtab scroll <ref|pixels>
pinchtab select <ref> <value>
pinchtab focus <ref>
pinchtab text
pinchtab ss
pinchtab eval <expression>
pinchtab pdf --tab <tabId>
pinchtab health
pinchtab quick <url>
```

These commands operate against the default routed tab context on the server.

Supported flags:

```bash
pinchtab nav <url> [--new-tab] [--block-images] [--block-ads]

pinchtab snap [-i|--interactive] [-c|--compact] [-d|--diff]
              [-s|--selector <css>] [--max-tokens N] [--depth N] [--tab <tabId>]

pinchtab text [--raw] [--tab <tabId>]

pinchtab ss [-o|--output <file>] [-q|--quality N] [--tab <tabId>]

pinchtab pdf --tab <tabId> [-o|--output <file>] [--landscape] [--scale N]
             [--paper-width N] [--paper-height N]
             [--margin-top N] [--margin-bottom N]
             [--margin-left N] [--margin-right N]
             [--page-ranges RANGE]
             [--prefer-css-page-size]
             [--display-header-footer]
             [--header-template HTML]
             [--footer-template HTML]
             [--generate-tagged-pdf]
             [--generate-document-outline]
             [--file-output]
             [--path PATH]
```

## Instance Commands

The current instance CLI shape is:

```bash
pinchtab instances

pinchtab instance start [--profileId <id>] [--mode headed|headless] [--port <port>]
pinchtab instance launch [--profileId <id>] [--mode headed|headless] [--port <port>]

pinchtab instance navigate <instance-id> <url>

pinchtab instance logs <instance-id>
pinchtab instance logs --id <instance-id>

pinchtab instance stop <instance-id>
pinchtab instance stop --id <instance-id>
```

Important details:
- `launch` is a CLI alias for `start`
- both `start` and `launch` call `POST /instances/start` today
- there is no `pinchtab instance <id> logs` grammar
- there is no CLI subcommand for `attach`

HTTP mapping:

| CLI | HTTP |
|---|---|
| `pinchtab instances` | `GET /instances` |
| `pinchtab instance start ...` | `POST /instances/start` |
| `pinchtab instance launch ...` | `POST /instances/start` |
| `pinchtab instance navigate <id> <url>` | `POST /instances/{id}/tabs/open` then `POST /tabs/{tabId}/navigate` |
| `pinchtab instance logs <id>` | `GET /instances/{id}/logs` |
| `pinchtab instance stop <id>` | `POST /instances/{id}/stop` |

## Tab Commands

The current tab CLI has two shapes:

1. list or legacy lifecycle shortcuts
2. explicit tab operations

### List And Legacy Shortcuts

```bash
pinchtab tabs
pinchtab tab

pinchtab tabs new [url]
pinchtab tab new [url]

pinchtab tabs close <tabId>
pinchtab tab close <tabId>
```

Notes:
- `tabs` and `tab` are both accepted
- `new` and `close` use the legacy `/tab` endpoint
- `tab new` will auto-launch a default instance if none is running

### Explicit Tab Operations

The implemented grammar is:

```bash
pinchtab tab navigate <tabId> <url>
pinchtab tab snapshot <tabId> [-i] [-c] [-d]
pinchtab tab screenshot <tabId> [-o file] [-q N]
pinchtab tab click <tabId> <ref>
pinchtab tab type <tabId> <ref> <text>
pinchtab tab fill <tabId> <ref> <text>
pinchtab tab press <tabId> <key>
pinchtab tab hover <tabId> <ref>
pinchtab tab scroll <tabId> <direction|pixels>
pinchtab tab select <tabId> <ref> <value>
pinchtab tab focus <tabId> <ref>
pinchtab tab text <tabId> [--raw]
pinchtab tab eval <tabId> <expression>
pinchtab tab pdf <tabId> [-o file] [--landscape] [--scale N]
pinchtab tab cookies <tabId>
pinchtab tab lock <tabId> [--owner name] [--ttl seconds]
pinchtab tab unlock <tabId> [--owner name]
pinchtab tab locks <tabId>
pinchtab tab info <tabId>
```

Important detail:
- the operation comes before the tab ID
- the implemented syntax is `pinchtab tab screenshot <tabId>`
- it is **not** `pinchtab tab <tabId> screenshot`

## Profiles Command

The implemented profile command is:

```bash
pinchtab profiles
```

Current behavior:
- calls `GET /profiles`
- prints a simple human-friendly list of names
- does not currently expose create/update/delete operations through the CLI

## Environment

The CLI uses these environment variables:

```bash
PINCHTAB_URL    # server base URL, default http://127.0.0.1:9867
PINCHTAB_TOKEN  # bearer token for API requests
PINCHTAB_PORT   # startup/server port
CHROME_BIN      # startup Chrome binary path
```

Use `PINCHTAB_TOKEN`, not `BRIDGE_TOKEN`.

## Recommended Workflows

### Basic Top-Level Flow

```bash
pinchtab

pinchtab nav https://pinchtab.com
pinchtab snap -i -c
pinchtab click e5
pinchtab text
```

### Explicit Instance Flow

```bash
INST=$(pinchtab instance start --mode headed | jq -r '.id')

pinchtab instance navigate "$INST" https://pinchtab.com
pinchtab tabs
```

### Tab-Targeted Flow

```bash
TAB=$(pinchtab tabs | jq -r '.[0].id')

pinchtab tab snapshot "$TAB" -i -c
pinchtab tab click "$TAB" e5
pinchtab tab text "$TAB" --raw
pinchtab tab pdf "$TAB" -o page.pdf
```

## Non-Goals For This Doc

This page intentionally does not document commands that are not implemented yet, including:
- a global `--instance` flag
- `pinchtab instance attach`
- full profile CRUD through the CLI
- `pinchtab tab <tabId> <action>` grammar
