# Identifying Pinchtab Chrome Instances

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
CHROME_BINARY=/usr/local/bin/pinchtab-chrome ./pinchtab
```

Now `ps aux | grep pinchtab-chrome` instantly identifies the automation instance. Name it whatever you want — `ai-agent-chrome`, `bot-chrome`, etc.

## 2. Add custom flags (visible in command line)

```bash
CHROME_FLAGS="--pinchtab-instance=prod-scraper-01" ./pinchtab
```

Chrome ignores unknown flags but they show up in the full command line:

```bash
ps aux | grep pinchtab-instance
```

Useful for distinguishing multiple Pinchtab instances too:

```bash
# Instance A
CHROME_FLAGS="--pinchtab-id=scraper" ./pinchtab

# Instance B
BRIDGE_PORT=9868 CHROME_FLAGS="--pinchtab-id=monitor" ./pinchtab
```

## 3. Separate profile directory (built-in)

Pinchtab already uses `~/.pinchtab/chrome-profile` by default — completely separate from your real Chrome profile. You'll see `--user-data-dir=/.../.pinchtab/...` in the process args.

For dashboard-managed instances, each profile gets its own directory under `~/.pinchtab/profiles/<name>/`.

## Full example

```bash
CHROME_BINARY=/usr/local/bin/pinchtab-chrome \
BRIDGE_PROFILE=~/.pinchtab/bot-prod \
BRIDGE_HEADLESS=true \
CHROME_FLAGS="--pinchtab-id=prod-scraper-01" \
./pinchtab
```

## Docker

Volume-mount your renamed binary and set the env var:

```dockerfile
COPY pinchtab-chrome /usr/local/bin/pinchtab-chrome
ENV CHROME_BINARY=/usr/local/bin/pinchtab-chrome
ENV CHROME_FLAGS="--pinchtab-id=docker-prod"
```

## Tips

- **[Chrome for Testing](https://googlechromelabs.github.io/chrome-for-testing/)** provides standalone Chrome binaries — no installer, won't conflict with system Chrome. Ideal for automation setups.
- **`chrome-headless-shell`** from the same source is even smaller — headless-only, perfect for server deployments.
- The parent process is always `pinchtab`, so `pkill -f pinchtab` won't touch your real Chrome.
- Combine all three approaches (renamed binary + custom flags + separate profile) for maximum clarity.
