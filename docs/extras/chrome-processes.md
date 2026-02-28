# Chrome Processes & Activity Monitor

Pinchtab launches its own Chrome instance. This doc explains how to identify Pinchtab's Chrome processes vs your personal Chrome, and what to do if things go wrong.

## How Pinchtab Uses Chrome

When Pinchtab starts, it spawns a Chrome process tree:

| Process | Role |
|---------|------|
| **Google Chrome** | Main browser process (parent) |
| **Google Chrome Helper** (gpu) | GPU rendering |
| **Google Chrome Helper** (network) | Network requests |
| **Google Chrome Helper** (storage) | Cookies, localStorage, etc. |
| **Google Chrome Helper (Renderer)** | One per tab/extension |

A typical Pinchtab instance creates **6-10 processes** depending on the number of open tabs.

## Identifying Pinchtab's Chrome

Pinchtab's Chrome is launched with a custom `--user-data-dir` flag pointing to the Pinchtab profile directory. Your personal Chrome uses `~/Library/Application Support/Google/Chrome` instead.

### From the terminal

```bash
# List Pinchtab's Chrome processes
ps ax | grep "user-data-dir=.*pinchtab" | grep -v grep

# List your personal Chrome processes (no --user-data-dir flag)
ps ax | grep "Google Chrome" | grep -v "user-data-dir" | grep -v grep
```

### From Activity Monitor

1. Open **Activity Monitor** (Spotlight → "Activity Monitor")
2. Search for **"Google Chrome"** in the search bar
3. Select a Chrome process → click the **ⓘ** (info) button in the toolbar
4. Go to the **Open Files and Ports** tab
5. Look for paths containing your Pinchtab state directory:
   - **Pinchtab Chrome**: references like `~/.pinchtab/chrome-profile/` or your custom `BRIDGE_STATE_DIR`
   - **Your Chrome**: references like `~/Library/Application Support/Google/Chrome/`

Alternatively, double-click any Chrome process → **Sample** or **Open Files** to inspect it.

### Quick check: is it headless?

Pinchtab's Chrome (in headless mode) won't appear in the Dock or have a visible window. If you see Chrome in Activity Monitor but not in the Dock, it's likely Pinchtab's.

## Normal Behavior

- Pinchtab Chrome processes appear when Pinchtab is running and disappear on graceful shutdown (SIGTERM/SIGINT)
- More tabs = more Renderer processes
- Memory usage: ~100-300MB base + ~50-150MB per tab depending on page complexity

## Orphaned Processes

If Pinchtab is force-killed (`kill -9`) or crashes, Chrome processes may be left behind. These are **orphaned processes** — they consume memory but do nothing useful.

### Automatic cleanup

Pinchtab automatically detects and kills orphaned Chrome processes on startup. You'll see this in the logs:

```
WARN killed orphaned Chrome processes count=8 profileDir=/path/to/profile
```

### Manual cleanup

If you need to clean up manually:

```bash
# Find orphaned Pinchtab Chrome processes
ps ax | grep "user-data-dir=.*pinchtab" | grep -v grep

# Kill them (graceful)
ps ax | grep "user-data-dir=.*pinchtab" | grep -v grep | awk '{print $1}' | xargs kill

# Or kill by specific profile path
pkill -f "user-data-dir=/Users/you/.pinchtab/chrome-profile"
```

In Activity Monitor, you can right-click any orphaned process → **Quit** (or **Force Quit**).

### How to tell if Chrome is orphaned

- Pinchtab is not running (`curl localhost:9867/health` fails)
- But Chrome processes with `--user-data-dir=<pinchtab-path>` still exist
- These are safe to kill

## Docker

Inside Docker containers, Chrome runs as `chromium-browser` under the `pinchtab` user. The container handles process lifecycle — when the container stops, all processes inside are cleaned up automatically. Orphan issues only apply to native (non-Docker) installations.
