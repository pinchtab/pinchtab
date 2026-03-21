# PinchTab SMCP Plugins

This directory holds plugins for [SMCP](https://github.com/sanctumos/smcp) (Model Context Protocol server for the Animus/Letta/Sanctum ecosystem). SMCP discovers plugins by scanning a directory for `plugins/<name>/cli.py` and running `python cli.py --describe` to get tool schemas.

> [!WARNING]
> These plugins expose PinchTab's privileged browser-control surface to MCP/SMCP clients. They are not designed for untrusted users, shared multi-tenant access, or direct public-internet exposure. If you are unsure how to secure a non-local deployment, review [../docs/guides/security.md](../docs/guides/security.md) and use the private security contact path in [../SECURITY.md](../SECURITY.md) before exposing the service.

## SMCP contract (reference)

Instructions below match the SMCP plugin contract. If your SMCP version differs, prefer [sanctumos/smcp](https://github.com/sanctumos/smcp) and its docs.

- **Discovery:** SMCP scans the directory set in **`MCP_PLUGINS_DIR`** (or its default `plugins/`). For each subdirectory `<name>` that contains a file **`cli.py`**, it runs `python cli.py --describe` and expects a single JSON object on stdout with:
  - **`plugin`:** `{ "name", "version", "description" }`
  - **`commands`:** array of `{ "name", "description", "parameters": [ { "name", "type", "description", "required", "default" } ] }`
- **Fallback:** If `--describe` is not supported, SMCP may run `python cli.py --help` and scrape an "Available commands:" section (no param schemas).
- **Tool naming:** Registered tools are **`<plugin>__<command>`** (double underscore), e.g. `pinchtab__navigate`. Legacy `plugin.command` may still be supported.
- **Execution:** For a tool call, SMCP runs `python cli.py <command> --arg1 val1 --arg2 val2 ...`. Argument names use **kebab-case** (`--base-url`, `--instance-id`). SMCP maps underscores to dashes; booleans `true` become `--flag` only; arrays become repeated `--arg item`. Default timeout is **300 seconds**. The plugin must print a **single JSON object to stdout**; SMCP returns it as the tool result.
- **Plugin layout:** Each plugin must live in a folder `<name>/` with **`cli.py`** (required, and must be executable). `__init__.py`, `README.md`, and `requirements.txt` are optional.

## Installation (PinchTab plugin)

1. **Point SMCP at this plugins directory**
   - Set **`MCP_PLUGINS_DIR`** to the path of **this** directory (the one that **contains** the `pinchtab` folder).
   - Example: if the repo is at `/home/me/pinchtab`, set `MCP_PLUGINS_DIR=/home/me/pinchtab/plugins`. SMCP will then find `pinchtab/cli.py` and discover the plugin.
   - Alternatively, copy the **`pinchtab`** folder into your existing SMCP plugins directory so you have `.../plugins/pinchtab/cli.py`.

2. **Make the CLI executable**
   ```bash
   chmod +x /path/to/plugins/pinchtab/cli.py
   ```

3. **No Python dependencies** for this plugin (stdlib only). Skip `pip install -r requirements.txt` for runtime.

4. **Restart the SMCP server** so it rescans and loads the plugin.

## Verify discovery

From the repo root:

```bash
python3 plugins/pinchtab/cli.py --describe | head -30
```

You should see JSON with `"plugin": {"name": "pinchtab", ...}` and `"commands": [...]`.

## PinchTab URL and auth

- The plugin defaults to **`--base-url http://localhost:9867`** (orchestrator). Agents pass `base_url`, and optionally `token` and `instance_id`, per tool call; no plugin-specific env vars are required.
- If PinchTab is protected with **`PINCHTAB_TOKEN`**, agents must pass `token` in the tool arguments (or configure it in your MCP server if it supports per-plugin env).

---

## pinchtab

Full SMCP plugin for the PinchTab HTTP API: health, instances, instance-start/stop, tabs, navigate, snapshot, action, actions, text, screenshot, pdf, evaluate, cookies-get, stealth-status.

- **Path:** `pinchtab/` (so SMCP sees `plugins/pinchtab/cli.py`).
- **Describe:** `python cli.py --describe` returns the JSON schema above.
- **Tests:** `cd pinchtab && python3 -m venv .venv && .venv/bin/pip install pytest && .venv/bin/pytest tests/ -v`

See [pinchtab/README.md](pinchtab/README.md) for command list and usage.
