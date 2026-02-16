# Pinchtab Scripts

## Auto-Start Setup

### Quick Install

```bash
./scripts/install-autostart.sh
```

This will:
- Build and install pinchtab to `/usr/local/bin`
- Configure auto-start on boot
- Start the service immediately

### macOS (launchd)

The installer creates a LaunchAgent that:
- Runs pinchtab on login
- Restarts automatically if it crashes
- Logs to `/tmp/pinchtab.*.log`

Manual control:
```bash
# Start/stop
launchctl start com.pinchtab.bridge
launchctl stop com.pinchtab.bridge

# Check status
launchctl list | grep pinchtab

# View logs
tail -f /tmp/pinchtab.*.log
```

### Linux (systemd)

The installer creates a systemd user service that:
- Runs pinchtab on boot
- Restarts automatically if it crashes
- Logs to systemd journal

Manual control:
```bash
# Start/stop
sudo systemctl start pinchtab@$USER
sudo systemctl stop pinchtab@$USER

# Check status
sudo systemctl status pinchtab@$USER

# View logs
sudo journalctl -u pinchtab@$USER -f
```

### Uninstall

```bash
./scripts/uninstall-autostart.sh
```

### Custom Configuration

Edit environment variables in:
- **macOS**: `~/Library/LaunchAgents/com.pinchtab.bridge.plist`
- **Linux**: `/etc/systemd/system/pinchtab@.service`

Common environment variables:
- `BRIDGE_PORT` - HTTP port (default: 9867)
- `BRIDGE_TOKEN` - Auth token (optional)
- `BRIDGE_HEADLESS` - Run Chrome headless (default: true)
- `BRIDGE_PROFILE` - Chrome profile directory

## Other Scripts

- `check.sh` - Run all pre-push checks (format, vet, build, test)