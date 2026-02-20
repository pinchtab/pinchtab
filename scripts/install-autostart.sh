#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PINCHTAB_BIN="$(cd "$SCRIPT_DIR/.." && pwd)/pinchtab"

echo "🦀 Pinchtab Auto-Start Installer"
echo "================================"

if [[ "$OSTYPE" == "darwin"* ]]; then
    echo "Detected macOS - using launchd"
    if [ ! -f "$PINCHTAB_BIN" ]; then
        echo "Building pinchtab..."
        (cd "$SCRIPT_DIR/.." && go build -o pinchtab ./cmd/pinchtab)
    fi
    echo "Installing pinchtab to /usr/local/bin..."
    sudo cp "$PINCHTAB_BIN" /usr/local/bin/pinchtab
    PLIST_SRC="$SCRIPT_DIR/launchd/com.pinchtab.bridge.plist"
    PLIST_DST="$HOME/Library/LaunchAgents/com.pinchtab.bridge.plist"
    mkdir -p "$HOME/Library/LaunchAgents"
    cp "$PLIST_SRC" "$PLIST_DST"
    echo "Loading LaunchAgent..."
    launchctl load -w "$PLIST_DST" 2>/dev/null || true
    
    echo ""
    echo "✅ Pinchtab installed and set to auto-start!"
    echo ""
    echo "Commands:"
    echo "  Start:   launchctl start com.pinchtab.bridge"
    echo "  Stop:    launchctl stop com.pinchtab.bridge"
    echo "  Status:  launchctl list | grep pinchtab"
    echo "  Logs:    tail -f /tmp/pinchtab.*.log"
    echo "  Disable: launchctl unload ~/Library/LaunchAgents/com.pinchtab.bridge.plist"
    
elif [[ "$OSTYPE" == "linux-gnu"* ]]; then
    echo "Detected Linux - using systemd"
    if [ ! -f "$PINCHTAB_BIN" ]; then
        echo "Building pinchtab..."
        (cd "$SCRIPT_DIR/.." && go build -o pinchtab ./cmd/pinchtab)
    fi
    echo "Installing pinchtab to /usr/local/bin..."
    sudo cp "$PINCHTAB_BIN" /usr/local/bin/pinchtab
    SERVICE_SRC="$SCRIPT_DIR/systemd/pinchtab.service"
    SERVICE_DST="/etc/systemd/system/pinchtab@.service"
    sudo cp "$SERVICE_SRC" "$SERVICE_DST"
    echo "Enabling service for user $USER..."
    sudo systemctl daemon-reload
    sudo systemctl enable "pinchtab@$USER.service"
    sudo systemctl start "pinchtab@$USER.service"
    
    echo ""
    echo "✅ Pinchtab installed and set to auto-start!"
    echo ""
    echo "Commands:"
    echo "  Start:   sudo systemctl start pinchtab@$USER"
    echo "  Stop:    sudo systemctl stop pinchtab@$USER"
    echo "  Status:  sudo systemctl status pinchtab@$USER"
    echo "  Logs:    sudo journalctl -u pinchtab@$USER -f"
    echo "  Disable: sudo systemctl disable pinchtab@$USER"
    
else
    echo "❌ Unsupported OS: $OSTYPE"
    echo "Please manually configure auto-start for your system."
    exit 1
fi

echo ""
echo "Test with: curl http://localhost:9867/health"
