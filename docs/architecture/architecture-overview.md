## Architecture

PinchTab sits between your tools/agents and Chrome:

```text
┌─────────────────────────────────────────┐
│         Your Tool/Agent                 │
│   (CLI, curl, Python, Node.js, etc.)    │
└──────────────┬──────────────────────────┘
               │
               │ HTTP
               ↓
┌─────────────────────────────────────────┐
│    PinchTab HTTP Server (Go)            │
│  ┌─────────────────────────────────┐    │
│  │  Tab Manager                    │    │
│  │  (tracks tabs + sessions)       │    │
│  └─────────────────────────────────┘    │
│  ┌─────────────────────────────────┐    │
│  │  Chrome DevTools Protocol (CDP) │    │
│  └─────────────────────────────────┘    │
└──────────────┬──────────────────────────┘
               │
               │ CDP WebSocket
               ↓
┌─────────────────────────────────────────┐
│        Chrome Browser                   │
│  (Headless, headed, or external)        │
└─────────────────────────────────────────┘
```
