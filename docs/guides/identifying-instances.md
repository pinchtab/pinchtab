# Chrome Instances

Running multiple Chrome instances? Here's how to tell Pinchtab's apart from your regular browser.

## 1. Rename the Chrome binary (recommended)

Copy Chrome to a custom name — changes the actual process name in `ps`, Activity Monitor, Task Manager:

```bash
# macOS
cp "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" /usr/local/bin/pinchtab-chrome

# Linux
cp $(which google-chrome) /usr/local/bin/pinchtab-chrome

chmod +x /usr/local/bin/pinchtab-chrome
```

Then point Pinchtab at it:

```bash
CHROME_BIN=/usr/local/bin/pinchtab-chrome pinchtab
```

Now `ps aux | grep pinchtab-chrome` instantly identifies the Chrome processes. Name it whatever you want — `ai-agent-chrome`, `bot-chrome`, etc.

## 2. Add custom Chrome flags (visible in process)

```bash
CHROME_FLAGS="--user-agent=Custom-Agent/1.0" pinchtab
```

Custom flags show up in the full process command line:

```bash
ps aux | grep pinchtab
# Shows Chrome processes with custom flags
```

Useful for identifying different agent roles:

```bash
# Scraper agent (with custom flag)
CHROME_FLAGS="--user-agent=Scraper/1.0" pinchtab &

# Monitor agent (different flag)
BRIDGE_PORT=9868 CHROME_FLAGS="--user-agent=Monitor/1.0" pinchtab &
```

## 3. Separate profile directory (built-in)

Each Pinchtab profile gets its own directory under `~/.pinchtab/profiles/<name>/` — completely separate from your real Chrome profile. 

Instance Chrome processes show `--user-data-dir=/.../.pinchtab/profiles/...` in their args.

When you create instances via API, they automatically use isolated profiles.

## 4. Identify via Dashboard

The easiest way: Open the dashboard at `http://localhost:9867/dashboard`

You'll see:
- Instance IDs (`inst_XXXXXXXX`)
- Which profile each instance uses
- Whether it's headed or headless
- When it was started
- Current status

## Full example

```bash
# Start orchestrator with custom Chrome binary
CHROME_BIN=/usr/local/bin/pinchtab-chrome pinchtab
```

Or for agents/instances with custom Chrome flags:

```bash
# Orchestrator
CHROME_BIN=/usr/local/bin/pinchtab-chrome pinchtab &

# Create instances with identifying info
# View in dashboard: http://localhost:9867/dashboard
curl -X POST http://localhost:9867/instances/launch \
  -d '{"mode":"headless"}'
  
# Instance appears in dashboard with its ID, port, profile info
```

## Docker

Volume-mount your renamed binary and set the env var:

```dockerfile
FROM alpine:latest

COPY pinchtab-chrome /usr/local/bin/pinchtab-chrome
RUN chmod +x /usr/local/bin/pinchtab-chrome

ENV CHROME_BIN=/usr/local/bin/pinchtab-chrome

EXPOSE 9867

CMD ["pinchtab"]
```

## Tips

- **[Chrome for Testing](https://googlechromelabs.github.io/chrome-for-testing/)** provides standalone Chrome binaries — no installer, won't conflict with system Chrome. Ideal for automation setups.
- **`chrome-headless-shell`** from the same source is even smaller — headless-only, perfect for server deployments.
- The orchestrator process is always `pinchtab`, so `pkill -f pinchtab` cleanly shuts down all instances.
- Instance IDs (`inst_XXXXXXXX`) are hash-based and stable — great for identifying instances in logs, dashboards, and monitoring.
- Combine approaches (renamed binary + custom flags + dashboard visibility) for maximum clarity in production.
- The dashboard shows all running instances with their profiles, status, and real-time activity — easiest way to identify what's running.
