# Identifying Instances

When you run PinchTab alongside your normal browser, the easiest way to distinguish its Chrome processes is to combine three signals:

- a dedicated Chrome binary name
- recognizable command-line flags
- the PinchTab dashboard and instance metadata

## 1. Use A Distinct Chrome Binary Name

If you copy Chrome or Chromium to a custom filename, that filename appears in process listings.

```bash
# macOS example
cp "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" /usr/local/bin/pinchtab-chrome
chmod +x /usr/local/bin/pinchtab-chrome

# Set in config.json
pinchtab config set browser.binary /usr/local/bin/pinchtab-chrome
pinchtab server
```

Now a process listing such as `ps -axo pid,command | rg pinchtab-chrome` gives you a quick way to spot the browser PinchTab launches.

## 2. Add Recognizable Chrome Flags

Use `instanceDefaults.userAgent` for a visible process marker, and reserve `browser.extraFlags` for safe non-security-reducing flags:

```json
{
  "instanceDefaults": {
    "userAgent": "PinchTab-Automation/1.0"
  },
  "browser": {
    "extraFlags": "--ash-no-nudges --disable-focus-on-load"
  }
}
```

Those flags appear in the Chrome command line, which makes process inspection easier:

```bash
ps -axo pid,command | rg 'PinchTab-Automation|user-data-dir'
```

Use this when you want to differentiate roles such as “scraper”, “monitor”, or “debug”.

Do not put security-reducing or PinchTab-owned flags in `browser.extraFlags`. For example, `--user-agent=...`, `--no-sandbox`, and stealth/runtime-owned flags are rejected.

## 3. Use Profile Paths As An Identifier

Each managed profile lives under the configured profile base directory. By default that is the OS-specific PinchTab config directory under `profiles/`.

PinchTab-launched Chrome processes include a `--user-data-dir=...` argument that points at that profile location. That is often the fastest way to confirm that a browser process belongs to PinchTab rather than your personal Chrome profile.

## 4. Use The Dashboard For The Most Reliable View

Open the dashboard at:

- `http://localhost:9867/`
- or `http://localhost:9867/dashboard`

The dashboard and instance APIs show:

- instance IDs
- profile IDs and profile names
- assigned ports
- headless vs headed mode
- current status

If you need an API-based view instead of the UI:

```bash
curl http://localhost:9867/instances
```

## Practical Combination

For most setups, this combination is enough:

1. point PinchTab to a renamed Chrome binary via `browser.binary` in config
2. add a recognizable `instanceDefaults.userAgent` marker or a safe `browser.extraFlags` marker in config
3. verify the profile path or instance ID in the dashboard

## Docker

The same approach works in containers:

- set `browser.binary` in config if you need to override the bundled browser path
- put only safe identifying flags in `browser.extraFlags`
- inspect the instance list from the API or dashboard rather than relying only on process names inside the container
