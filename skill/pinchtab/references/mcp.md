# MCP / SMCP Reference

Pinchtab exposes browser control tools via an [SMCP](https://github.com/sanctumos/smcp) plugin. Agents using MCP-compatible clients can call Pinchtab tools directly.

## Available Tools

Tool names follow the `pinchtab__<command>` convention (double underscore):

| Tool | Description |
|------|-------------|
| `pinchtab__health` | Health check |
| `pinchtab__instances` | List running instances |
| `pinchtab__instance-start` | Start an instance (optional: profile-id, mode, port) |
| `pinchtab__instance-stop` | Stop an instance |
| `pinchtab__tabs` | List tabs |
| `pinchtab__navigate` | Navigate to URL |
| `pinchtab__snapshot` | Get accessibility tree (filter, format, selector, max-tokens, diff) |
| `pinchtab__action` | Single action: click, type, press, focus, fill, hover, select, scroll |
| `pinchtab__actions` | Batch actions (JSON array) |
| `pinchtab__text` | Extract page text |
| `pinchtab__screenshot` | Take screenshot |
| `pinchtab__pdf` | Export tab to PDF |
| `pinchtab__evaluate` | Run JavaScript |
| `pinchtab__cookies-get` | Get cookies |
| `pinchtab__stealth-status` | Stealth/fingerprint status |

## Configuration

| Setting | Description |
|---------|-------------|
| `MCP_PLUGINS_DIR` | Set to the `plugins/` directory containing the `pinchtab` folder |
| `--base-url` | Pinchtab server URL (default: `http://localhost:9867`) |
| `--token` | Auth token (if `BRIDGE_TOKEN` is set on the server) |
| `--instance-id` | Target instance ID (required for instance-scoped calls via orchestrator) |

### Setup

```bash
# 1. Point SMCP at the plugins directory
export MCP_PLUGINS_DIR=/path/to/pinchtab/plugins

# 2. Ensure cli.py is executable
chmod +x plugins/pinchtab/cli.py

# 3. Restart SMCP — no pip install required (stdlib only)
```

## Example Tool Call

Agent calls `pinchtab__navigate`:

```json
{
  "base_url": "http://localhost:9867",
  "instance_id": "inst_0a89a5bb",
  "url": "https://example.com"
}
```

SMCP invokes:

```bash
python cli.py navigate --base-url http://localhost:9867 --instance-id inst_0a89a5bb --url https://example.com
```

Returns JSON to stdout:

```json
{ "status": "success", "data": { ... } }
```

## What MCP CAN'T Do

The SMCP plugin covers browser control only. These features are **not available** via MCP:

| Not Available | Alternative |
|---------------|-------------|
| Profile CRUD (create, update, delete) | Use `curl` against `/profiles` endpoints |
| Scheduler / cron jobs | External scheduler + MCP tool calls |
| Stealth configuration changes | Set `BRIDGE_STEALTH` env var before launch |
| Dashboard UI control | Use the web dashboard directly |
| Download / upload files | Use `curl` against `/download` and `/upload` endpoints |
| Cookie setting | Use `curl` against `POST /cookies` |
| Fingerprint rotation | Use `curl` against `POST /fingerprint/rotate` |

## Requirements

- Python 3.9+
- A running Pinchtab server
- No extra Python dependencies (stdlib only)
