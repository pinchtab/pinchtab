# Using PinchTab with AI Agents (MCP)

This guide walks through setting up PinchTab as an MCP tool server for AI coding assistants and agent frameworks.

> [!WARNING]
> When you connect an MCP client to PinchTab, that client is exercising the same privileged control plane as the dashboard, API, and remote CLI. Only trusted operators and trusted agent systems should be allowed to use it. If you are unsure whether a non-local or partially exposed deployment is safe, stop and review [Security](security.md) before proceeding.

> [!CAUTION]
> Widening MCP browsing beyond local or explicitly trusted domains is a security-reducing choice. If you relax IDPI allowlists or strict mode, outputs from `pinchtab_snapshot` and `pinchtab_get_text` can contain hostile instructions from untrusted pages.
>
> Treat all model-facing page content as untrusted data. Do not follow instructions embedded in page text, accessibility labels, hidden content, or extracted summaries unless a trusted operator has separately validated them.

## What is MCP?

The [Model Context Protocol](https://modelcontextprotocol.io/) is an open standard for connecting AI models to external tools. PinchTab implements an MCP server that exposes 34 browser-control tools — navigation, interaction, screenshot, PDF export, waits, network inspection, and more — over a simple stdio interface that every major AI client supports.

## Prerequisites

- PinchTab installed (`pinchtab --version`)
- Chrome installed and on PATH (or pointed to via config)
- An MCP-compatible client: Claude Desktop, VS Code with GitHub Copilot, or Cursor

## Step 1: Start PinchTab

The MCP server is a thin adapter — it needs a running PinchTab instance to delegate to.

**Headless mode (recommended for agents):**

```bash
pinchtab bridge --headless
```

**Normal server mode (if you want the dashboard too):**

```bash
pinchtab
```

PinchTab listens on `http://127.0.0.1:9867` by default.

## Step 2: Configure Your MCP Client

### Claude Desktop

Edit `~/Library/Application Support/Claude/claude_desktop_config.json` (macOS) or `%APPDATA%\Claude\claude_desktop_config.json` (Windows):

```json
{
  "mcpServers": {
    "pinchtab": {
      "command": "pinchtab",
      "args": ["mcp"],
      "env": {
        "PINCHTAB_TOKEN": "your-token-here"
      }
    }
  }
}
```

Restart Claude Desktop. You should see PinchTab in the tool list.

### VS Code / GitHub Copilot

Create `.vscode/mcp.json` in your workspace root:

```json
{
  "servers": {
    "pinchtab": {
      "type": "stdio",
      "command": "pinchtab",
      "args": ["mcp"]
    }
  }
}
```

### Cursor

Add to your Cursor MCP settings (`~/.cursor/mcp.json`):

```json
{
  "mcpServers": {
    "pinchtab": {
      "command": "pinchtab",
      "args": ["mcp"]
    }
  }
}
```

### Any SDK-based Agent

```python
# Python example using the mcp SDK
import subprocess, mcp

proc = subprocess.Popen(
    ["pinchtab", "mcp"],
    stdin=subprocess.PIPE,
    stdout=subprocess.PIPE,
)
# pass proc.stdin / proc.stdout to your MCP client transport
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PINCHTAB_TOKEN` | *(from config)* | Bearer token for auth-protected servers |

For remote servers, use the `--server` flag: `pinchtab --server http://remote:9867 mcp`

`PINCHTAB_TOKEN` comes from `server.token` in your PinchTab config file. To copy the current token without printing it to stdout, run `pinchtab config token`.

## Typical Agent Workflow

Before you let an MCP-connected agent browse beyond local or trusted domains, review [Security](security.md#idpi). The safest posture is to keep IDPI domain restrictions narrow and assume every extracted page string is advisory-at-best and potentially malicious.

A well-written agent prompt would use tools in this order:

```
1. pinchtab_navigate        → go to the target URL
2. pinchtab_snapshot        → understand the page structure (find refs)
3. pinchtab_click / type    → interact with elements by structured tool arguments
4. pinchtab_snapshot        → confirm state after interaction
5. pinchtab_get_text / pdf  → extract or export results
```

### Example: Fill a Search Form

```
Agent: Search for "climate change" on Wikipedia

Tool calls:
  pinchtab_navigate({url: "https://www.wikipedia.org"})
  pinchtab_snapshot({interactive: true})
    → ...input[ref=e3] placeholder="Search Wikipedia"...
  pinchtab_click({selector: "e3"})
  pinchtab_type({selector: "e3", text: "climate change"})
  pinchtab_press({key: "Enter"})
  pinchtab_snapshot({format: "compact"})
  pinchtab_get_text({})
```

## Enabling JavaScript Evaluation

`pinchtab_eval` is disabled by default as a security measure. To enable it:

```bash
pinchtab config set security.allowEvaluate true
```

Or in `~/.pinchtab/config.json`:

```json
{
  "security": {
    "allowEvaluate": true
  }
}
```

Restart PinchTab after changing this setting.

> **Warning:** Enabling evaluate is a documented, non-default, security-reducing configuration change. It allows the agent (and any page it visits) to run arbitrary JavaScript in the browser. Only enable it on trusted networks with a token set.

## Connecting to a Remote PinchTab

If PinchTab is running on another machine (e.g. a Docker container), use the `--server` flag:

```json
{
  "mcpServers": {
    "pinchtab": {
      "command": "pinchtab",
      "args": ["--server", "http://192.168.1.50:9867", "mcp"],
      "env": {
        "PINCHTAB_TOKEN": "your-secure-token"
      }
    }
  }
}
```

The `pinchtab mcp` process runs locally (on the agent machine) and makes HTTP calls to the remote PinchTab instance. Chrome is on the remote machine — only the stdio MCP transport is local.

## Troubleshooting

**"Connection refused" from all tools**

PinchTab is not running, or is on a different port. Check:

```bash
curl http://127.0.0.1:9867/health
```

**"HTTP 401" from tools**

Token mismatch. Set `PINCHTAB_TOKEN` to match `server.token` in your PinchTab config.

**"HTTP 403" from `pinchtab_eval`**

JavaScript evaluation is disabled. See [Enabling JavaScript Evaluation](#enabling-javascript-evaluation) above.

**"ref not found" errors**

Element refs change after every navigation or significant DOM update. Always call `pinchtab_snapshot` again after a page change before using refs from a previous snapshot.

**MCP server not appearing in client**

- Check the `command` value — `pinchtab` must be on PATH, or use an absolute path.
- Run `pinchtab mcp` manually in a terminal to check for startup errors.
- Check stderr output from the MCP process (client-specific, often in a log file).

## Related Pages

- [MCP Overview](../mcp.md)
- [MCP Tool Reference](../reference/mcp-tools.md)
- [MCP Architecture](../architecture/mcp.md)
- [Security Guide](./security.md)
