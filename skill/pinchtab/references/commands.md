# CLI Commands Reference

> **Canonical names only.** Aliases like `ss` (for `screenshot`) work but use full names in automation.

## Instance Management

| Command | Description |
|---------|-------------|
| `pinchtab instances` | List running instances |
| `pinchtab instance launch [--mode headed] [--port N]` | Launch a new instance |
| `pinchtab instance <id> logs` | View instance logs |
| `pinchtab instance <id> stop` | Stop an instance |

## Browser Control

All browser commands accept `--instance <id>` (or set `PINCHTAB_INSTANCE` env var).

| Command | Description |
|---------|-------------|
| `pinchtab nav <url> [--new-tab] [--block-images]` | Navigate to URL |
| `pinchtab snap [-i] [-c] [-d] [-s <selector>] [--max-tokens N]` | Snapshot accessibility tree |
| `pinchtab click <ref>` | Click element by ref |
| `pinchtab type <ref> "text"` | Type into element |
| `pinchtab fill <ref> "value"` | Set value directly (no keystrokes) |
| `pinchtab press <key>` | Press key (Enter, Tab, Escape, etc.) |
| `pinchtab scroll <down\|up\|N>` | Scroll page |
| `pinchtab text [--raw]` | Extract readable page text |
| `pinchtab screenshot [-o file] [-q 0-100]` | Take screenshot |
| `pinchtab pdf [-o file] [--landscape] [--page-ranges "1-3"]` | Export page as PDF |
| `pinchtab eval "<expression>"` | Run JavaScript |
| `pinchtab cookies` | Get cookies for current page |
| `pinchtab console [--clear] [--limit N]` | View browser console logs |
| `pinchtab errors [--clear] [--limit N]` | View uncaught JS errors |
| `pinchtab health` | Server health check |

## Tab Management

| Command | Description |
|---------|-------------|
| `pinchtab tabs` | List tabs |
| `pinchtab tab create <url>` | Create new tab |
| `pinchtab tab <id> navigate <url>` | Navigate specific tab |
| `pinchtab tab <id> close` | Close tab |
| `pinchtab tab <id> lock --owner <name> --ttl <sec>` | Lock tab (multi-agent) |
| `pinchtab tab <id> unlock --owner <name>` | Unlock tab |

## Snapshot Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--interactive` | `-i` | Interactive elements only (buttons, links, inputs) |
| `--compact` | `-c` | One-line-per-node format (56–64% fewer tokens) |
| `--diff` | `-d` | Only changes since last snapshot |
| `--selector` | `-s` | Scope to CSS selector (e.g. `main`) |
| `--max-tokens` | — | Truncate output to ~N tokens |
| `--depth` | — | Max tree depth |

## Common Pitfalls

| ❌ Wrong | ✅ Correct | Notes |
|----------|-----------|-------|
| `ss` | `screenshot` | `ss` is a shortcut alias — use full name |
| `--profileId` | `--profile` | Flag is `--profile` |
| `tabs` (for single) | `tab <id> <action>` | `tabs` lists; `tab` manages |
| `find` | `snap -i` | `find` doesn't exist; use filtered snapshot |

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PINCHTAB_URL` | `http://localhost:9867` | Server URL |
| `PINCHTAB_INSTANCE` | (auto) | Default instance ID |
| `PINCHTAB_TOKEN` | (none) | Auth token |
| `PINCHTAB_TIMEOUT` | `30` | Request timeout (seconds) |

## Exit Codes

| Code | Meaning |
|------|---------|
| `0` | Success |
| `1` | User error (bad args, missing file) |
| `2` | Server error (500, connection refused) |
| `3` | Timeout |
| `4` | Resource not found |
