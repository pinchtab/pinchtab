# PinchTab SMCP Plugin

SMCP plugin for [PinchTab](https://github.com/pinchtab/pinchtab): browser control for AI agents via the PinchTab HTTP API. Conforms to the SMCP plugin contract (discovery via `--describe`, tool naming `plugin__command`, execution via `python cli.py <command> --arg val ...`, JSON to stdout).

## Requirements

- Python 3.9+
- A running PinchTab server (orchestrator on port 9867 or direct instance on 9868+)
- No extra Python dependencies (stdlib only)

## SMCP contract (this plugin)

- **Discovery:** SMCP runs **`python cli.py --describe`**. This plugin returns JSON with `plugin` (`name`, `version`, `description`) and `commands` (each with `name`, `description`, `parameters` with `name`, `type`, `description`, `required`, `default`).
- **Tool names:** SMCP registers tools as **`pinchtab__<command>`** (double underscore), e.g. `pinchtab__navigate`, `pinchtab__instance-start`, `pinchtab__snapshot`.
- **Execution:** SMCP runs **`python cli.py <command> --arg1 val1 --arg2 val2 ...`**. Parameter names are **kebab-case** (e.g. `--base-url`, `--instance-id`, `--tab-id`). The plugin prints a **single JSON object to stdout**; SMCP uses it as the tool result. Timeout is 300 seconds (SMCP default).
- **Layout:** This folder must contain **`cli.py`** and it must be **executable** (`chmod +x cli.py`). SMCP discovers it when **`MCP_PLUGINS_DIR`** points at the parent directory that contains this `pinchtab` folder (see [../README.md](../README.md)).

## Commands

| Command | Description |
|--------|-------------|
| `health` | Health check |
| `instances` | List instances (orchestrator) |
| `instance-start` | Start an instance (optional profile-id, mode, port) |
| `instance-stop` | Stop an instance (requires instance-id) |
| `tabs` | List tabs |
| `navigate` | Navigate to URL |
| `snapshot` | Get accessibility tree (filter, format, selector, max-tokens, diff) |
| `action` | Single action: click, type, press, focus, fill, hover, select, scroll |
| `actions` | Batch actions (JSON array) |
| `text` | Extract page text |
| `screenshot` | Take screenshot |
| `pdf` | Export tab to PDF (requires tab-id) |
| `evaluate` | Run JavaScript |
| `cookies-get` | Get cookies |
| `stealth-status` | Stealth/fingerprint status |

## Usage

- **Base URL:** `--base-url http://localhost:9867` (default). Use orchestrator URL or a direct instance URL.
- **Orchestrator + instance:** When talking to the orchestrator, pass **`--instance-id inst_xxxx`** for instance-scoped calls (navigate, snapshot, action, etc.).
- **Token:** If PinchTab uses **`PINCHTAB_TOKEN`**, pass **`--token YOUR_TOKEN`** in the tool args.

## Example (SMCP tool call)

1. Agent calls tool **`pinchtab__navigate`** with arguments (e.g. from MCP client):
   ```json
   { "base_url": "http://localhost:9867", "instance_id": "inst_0a89a5bb", "url": "https://pinchtab.com" }
   ```
2. SMCP invokes:
   ```bash
   python cli.py navigate --base-url http://localhost:9867 --instance-id inst_0a89a5bb --url https://pinchtab.com
   ```
3. Plugin prints to stdout:
   ```json
   { "status": "success", "data": { ... } }
   ```
   SMCP returns that JSON as the tool result.

## Installation (summary)

1. Set **`MCP_PLUGINS_DIR`** to the path of the **parent** `plugins/` directory (the one that contains this `pinchtab` folder), or copy this **`pinchtab`** folder into your existing SMCP plugins directory.
2. Run **`chmod +x cli.py`**.
3. Restart SMCP. No `pip install` is required for runtime.

## Tests

Optional (for development). From this directory:

```bash
python3 -m venv .venv
.venv/bin/pip install pytest
.venv/bin/pytest tests/ -v
```
