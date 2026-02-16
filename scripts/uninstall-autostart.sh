#!/bin/bash
set -e

echo "🦀 Pinchtab Auto-Start Uninstaller"
echo "==================================="

# Detect OS
if [[ "$OSTYPE" == "darwin"* ]]; then
    # macOS
    echo "Detected macOS - removing launchd configuration"
    
    PLIST="$HOME/Library/LaunchAgents/com.pinchtab.bridge.plist"
    
    # Unload if running
    echo "Stopping Pinchtab..."
    launchctl unload "$PLIST" 2>/dev/null || true
    
    # Remove plist
    if [ -f "$PLIST" ]; then
        echo "Removing LaunchAgent..."
        rm "$PLIST"
    fi
    
    # Optionally remove binary
    read -p "Remove /usr/local/bin/pinchtab? (y/N): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        sudo rm -f /usr/local/bin/pinchtab
    fi
    
elif [[ "$OSTYPE" == "linux-gnu"* ]]; then
    # Linux
    echo "Detected Linux - removing systemd configuration"
    
    # Stop and disable service
    echo "Stopping Pinchtab..."
    sudo systemctl stop "pinchtab@$USER" 2>/dev/null || true
    sudo systemctl disable "pinchtab@$USER" 2>/dev/null || true
    
    # Remove service file
    if [ -f "/etc/systemd/system/pinchtab@.service" ]; then
        echo "Removing systemd service..."
        sudo rm "/etc/systemd/system/pinchtab@.service"
        sudo systemctl daemon-reload
    fi
    
    # Optionally remove binary
    read -p "Remove /usr/local/bin/pinchtab? (y/N): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        sudo rm -f /usr/local/bin/pinchtab
    fi
    
else
    echo "❌ Unsupported OS: $OSTYPE"
    exit 1
fi

echo ""
echo "✅ Pinchtab auto-start has been removed!"