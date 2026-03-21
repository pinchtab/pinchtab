# Background Service (Daemon)

PinchTab can run as a user-level background service (daemon) on macOS (`launchd`) and Linux (`systemd`). This ensures that the PinchTab server is always available to your agents without needing an open terminal window.

This workflow is not currently provided on Windows. Windows binaries are available, but Windows support is limited and best-effort; on Windows, prefer running `pinchtab server` or `pinchtab bridge` directly.

![Daemon Status & Picker](../media/daemon-status.png)

## Quick Start

The normal entrypoints are:

```bash
pinchtab
```

Then choose `Daemon` from the menu, or manage the service directly:

```bash
pinchtab daemon
```

When run without arguments in an interactive terminal, this command shows the current status and opens a picker for common actions.

## Daemon Commands

| Command | Description |
|---------|-------------|
| `pinchtab daemon` | Show status summary, recent logs, and open interactive picker. |
| `pinchtab daemon install` | Create and enable the background service file. |
| `pinchtab daemon start` | Start the background service if it is stopped. |
| `pinchtab daemon stop` | Stop the background service. |
| `pinchtab daemon restart` | Restart the service (useful after config changes). |
| `pinchtab daemon uninstall` | Disable and remove the background service file. |

## Status & Diagnostics

The `pinchtab daemon` command provides a comprehensive overview of the service:

- **Service Status**: Shows if the `.plist` (macOS) or `.service` (Linux) file is installed.
- **State**: Indicates if the process is `active (running)` or `stopped`.
- **PID**: The Process ID of the running server.
- **Path**: The exact location of the service configuration file on your system.
- **Recent Logs**: The last few lines of output from the server to help diagnose issues.

## Manual Installation

If the automated commands fail due to permission issues or system restrictions, PinchTab provides manual instructions tailored to your OS.

PinchTab now fails fast before install when the current session cannot manage a user service.

Typical cases:

- Linux shell without a working `systemctl --user` session
- macOS shell without an active GUI `launchd` domain

In those cases, use the manual steps below or run `pinchtab server` in the foreground instead.

### macOS (launchd)
Service file: `~/Library/LaunchAgents/com.pinchtab.pinchtab.plist`

1. Create the plist file (PinchTab will provide the content on error).
2. Register and start:
   ```bash
   launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.pinchtab.pinchtab.plist
   ```

### Linux (systemd)
Service file: `~/.config/systemd/user/pinchtab.service`

1. Create the unit file.
2. Reload and enable:
   ```bash
   systemctl --user daemon-reload
   systemctl --user enable --now pinchtab.service
   ```

## Conflict Detection

If you try to start a PinchTab server in the foreground (`pinchtab server`) while the daemon is already running on the same port, PinchTab will detect the conflict, warn you, and exit to prevent port binding errors.
