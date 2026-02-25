# Pinchtab

Browser control API for AI agents.

## Installation

```bash
npm install pinchtab
```

This downloads the Pinchtab binary for your platform (macOS, Linux, Windows) on install.

## Quick Start

### Start the server

```bash
pinchtab serve --port 9867
```

### Use the SDK

```typescript
import Pinchtab from 'pinchtab';

const pinch = new Pinchtab({ port: 9867 });

// Start the server
await pinch.start();

// Take a snapshot
const snapshot = await pinch.snapshot({ refs: 'role' });
console.log(snapshot.html);

// Click on an element
await pinch.click({ ref: 'e42' });

// Lock a tab
await pinch.lock({ tabId: 'tab1', timeoutMs: 5000 });

// Stop the server
await pinch.stop();
```

## API

### `new Pinchtab(options)`

Create a Pinchtab client.

**Options:**
- `baseUrl` (string): API base URL. Default: `http://localhost:9867`
- `timeout` (number): Request timeout in ms. Default: `30000`
- `port` (number): Port to run on. Default: `9867`

### `start(binaryPath?)`

Start the Pinchtab server process.

### `stop()`

Stop the Pinchtab server process.

### `snapshot(params?)`

Take a snapshot of the current tab.

**Params:**
- `refs` ('role' | 'aria'): Reference system
- `selector` (string): CSS selector filter
- `maxTokens` (number): Token limit
- `format` ('full' | 'compact'): Response format

### `click(params)`

Click on an element.

**Params:**
- `ref` (string): Element reference
- `targetId` (string): Optional target tab ID

### `lock(params)` / `unlock(params)`

Lock/unlock a tab.

### `createTab(params)`

Create a new tab.

**Params:**
- `url` (string): Tab URL
- `stealth` ('light' | 'full'): Stealth level

## CLI

```bash
pinchtab serve [--port PORT]
pinchtab --version
pinchtab --help
```

## License

MIT
