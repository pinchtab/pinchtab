# Pinchtab OpenClaw Plugin

Browser control for AI agents via [Pinchtab](https://pinchtab.com).

## Install

```bash
openclaw plugins install @pinchtab/openclaw-plugin
```

## Configure

```json5
{
  plugins: {
    entries: {
      pinchtab: {
        enabled: true,
        config: {
          baseUrl: "http://localhost:9867",
          token: "your-bridge-token",  // optional
        },
      },
    },
  },
  agents: {
    list: [{
      id: "main",
      tools: { allow: ["pinchtab"] },
    }],
  },
}
```

## Tools

| Tool | Description |
|---|---|
| `pinchtab_navigate` | Navigate to a URL |
| `pinchtab_snapshot` | Get accessibility tree (refs for actions) |
| `pinchtab_action` | Click, type, press, hover, scroll, select |
| `pinchtab_text` | Extract readable text (~1K tokens) |
| `pinchtab_tabs` | List/open/close tabs |
| `pinchtab_screenshot` | Take screenshot (returns JPEG) |
| `pinchtab_health` | Check Pinchtab connectivity |

All tools are optional (opt-in). Enable via agent allowlist: `tools.allow: ["pinchtab"]`.

## Requirements

- Running Pinchtab instance (`pinchtab &`)
- OpenClaw Gateway
