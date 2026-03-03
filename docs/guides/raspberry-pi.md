# Raspberry Pi Setup Guide

Pinchtab works great on Raspberry Pi (3, 4, 5, Zero 2 W) for browser automation, web scraping, testing, and headless browsing. This guide covers installation, optimization, and common issues.

## Requirements

- **Raspberry Pi**: Pi 3, 4, 5, or Zero 2 W (ARM64/aarch64)
- **OS**: Raspberry Pi OS (64-bit recommended) or Ubuntu
- **RAM**: 2GB+ recommended (1GB works for headless with low tab counts)
- **Node.js**: 18+ (install via [nvm](https://github.com/nvm-sh/nvm) recommended)
- **Chrome/Chromium**: Required (not bundled with pinchtab)

## Installation

### Step 1: Install Chrome/Chromium

Pinchtab requires Chrome or Chromium. Install it first:

```bash
sudo apt update
sudo apt install -y chromium-browser
```

Verify installation:
```bash
which chromium-browser
# Should output: /usr/bin/chromium-browser
```

### Step 2: Install Node.js

**Option A: Via nvm (recommended)**
```bash
curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/master/install.sh | bash
source ~/.bashrc  # or ~/.zshrc
nvm install 22
nvm use 22
```

**Option B: Via apt** (usually outdated, not recommended)
```bash
sudo apt install -y nodejs npm
```

Verify:
```bash
node -v  # Should be 18+
npm -v
```

### Step 3: Install Pinchtab

```bash
curl -fsSL https://pinchtab.com/install.sh | bash
```

Or manually via npm:
```bash
npm install -g pinchtab
```

### Step 4: Start Pinchtab

```bash
pinchtab
```

First startup takes ~5-10 seconds as Chrome initializes. You should see:
```
INFO starting chrome initialization headless=true
INFO chrome initialized successfully
INFO Bridge server listening addr=127.0.0.1:9867
```

Test it:
```bash
curl http://localhost:9867/health
```

## Configuration for Raspberry Pi

### Headless Mode (Default)

By default, pinchtab runs Chrome in headless mode (no GUI), which is perfect for Pi:

```bash
pinchtab  # Headless by default
```

### Headed Mode (with Desktop)

If you're running Raspberry Pi OS with desktop and want to see the browser:

```bash
BRIDGE_HEADLESS=false pinchtab
```

### Memory Optimization

Raspberry Pi has limited RAM. Optimize pinchtab for lower memory usage:

**Limit max tabs:**
```bash
BRIDGE_MAX_TABS=5 pinchtab
```

**Disable images (saves bandwidth + memory):**
```bash
BRIDGE_BLOCK_IMAGES=true pinchtab
```

**Block ads (saves bandwidth):**
```bash
BRIDGE_BLOCK_ADS=true pinchtab
```

**Combined example:**
```bash
BRIDGE_MAX_TABS=5 BRIDGE_BLOCK_IMAGES=true BRIDGE_BLOCK_ADS=true pinchtab
```

### Storage Location

On Pi, you might want to use a USB drive or external storage for the Chrome profile:

```bash
BRIDGE_PROFILE=/mnt/usb/pinchtab-profile pinchtab
```

Or create a persistent config:
```bash
mkdir -p ~/.config/pinchtab
cat > ~/.config/pinchtab/config.json <<EOF
{
  "port": "9867",
  "headless": true,
  "maxTabs": 5,
  "profileDir": "/mnt/usb/pinchtab-profile"
}
EOF
```

## Performance Tips

### 1. Use Headless Mode

Headless mode uses significantly less RAM than headed:
```bash
# Default is headless=true, but you can be explicit:
BRIDGE_HEADLESS=true pinchtab
```

### 2. Reduce Chrome Flags

Add lighter-weight Chrome flags:
```bash
CHROME_FLAGS="--disable-gpu --disable-software-rasterizer --disable-dev-shm-usage" pinchtab
```

### 3. Swap Space (for 1GB Pi models)

If you have only 1GB RAM, increase swap:
```bash
sudo dphys-swapfile swapoff
sudo nano /etc/dphys-swapfile
# Change CONF_SWAPSIZE=100 to CONF_SWAPSIZE=1024
sudo dphys-swapfile setup
sudo dphys-swapfile swapon
```

### 4. Overclock (Carefully)

For Pi 4/5, mild overclocking can improve performance:
```bash
sudo raspi-config
# Performance Options → Overclock → Moderate
```

⚠️ **Ensure adequate cooling** — use a heatsink or fan.

### 5. Use Lightweight OS

Consider **Raspberry Pi OS Lite** (no desktop) for dedicated automation:
```bash
# Install minimal OS, then:
sudo apt install -y chromium-browser nodejs npm
npm install -g pinchtab
```

## Running as a Service

Run pinchtab automatically on boot using systemd:

### Create service file:
```bash
sudo nano /etc/systemd/system/pinchtab.service
```

**Content:**
```ini
[Unit]
Description=Pinchtab Browser Automation
After=network.target

[Service]
Type=simple
User=pi
WorkingDirectory=/home/pi
ExecStart=/home/pi/.nvm/versions/node/v22.13.0/bin/pinchtab
Restart=always
RestartSec=10
Environment="BRIDGE_HEADLESS=true"
Environment="BRIDGE_MAX_TABS=5"
Environment="BRIDGE_BLOCK_IMAGES=true"

[Install]
WantedBy=multi-user.target
```

**Note:** Adjust `ExecStart` path to match your Node.js installation:
```bash
which pinchtab  # Use this path in ExecStart
```

### Enable and start:
```bash
sudo systemctl daemon-reload
sudo systemctl enable pinchtab
sudo systemctl start pinchtab
```

### Check status:
```bash
sudo systemctl status pinchtab
```

### View logs:
```bash
sudo journalctl -u pinchtab -f
```

## Common Issues

### Issue: "Chrome binary not found"

**Cause:** Chromium not installed.

**Solution:**
```bash
sudo apt install -y chromium-browser
```

If chromium is installed but pinchtab can't find it:
```bash
export CHROME_BIN=/usr/bin/chromium-browser
pinchtab
```

Make it permanent:
```bash
echo 'export CHROME_BIN=/usr/bin/chromium-browser' >> ~/.bashrc
source ~/.bashrc
```

### Issue: Chrome crashes or hangs

**Cause:** Out of memory.

**Solutions:**
1. Reduce max tabs: `BRIDGE_MAX_TABS=3 pinchtab`
2. Block images: `BRIDGE_BLOCK_IMAGES=true pinchtab`
3. Increase swap (see Performance Tips)
4. Close other applications

### Issue: "Address already in use"

**Cause:** Port 9867 is occupied.

**Solution:** Use a different port:
```bash
BRIDGE_PORT=9868 pinchtab
```

### Issue: Slow page loads

**Cause:** Limited bandwidth or CPU.

**Solutions:**
1. Block images/ads: `BRIDGE_BLOCK_IMAGES=true BRIDGE_BLOCK_ADS=true pinchtab`
2. Increase navigation timeout:
   ```json
   {
     "navigateSec": 90
   }
   ```
3. Use wired Ethernet instead of Wi-Fi

### Issue: npm install fails with EACCES

**Cause:** Permission error.

**Solution:** Use nvm (recommended) or fix npm permissions:
```bash
# Option 1: Use nvm (best)
curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/master/install.sh | bash
nvm install 22
npm install -g pinchtab

# Option 2: Fix npm permissions
mkdir ~/.npm-global
npm config set prefix '~/.npm-global'
echo 'export PATH=~/.npm-global/bin:$PATH' >> ~/.bashrc
source ~/.bashrc
npm install -g pinchtab
```

## Example Use Cases

### Web Scraping Cron Job

```bash
#!/bin/bash
# scrape-daily.sh

curl -X POST http://localhost:9867/tabs \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://example.com",
    "actions": [
      {"kind": "wait", "selector": ".content"}
    ]
  }' | jq -r '.snapshot' > /home/pi/data/snapshot-$(date +%Y%m%d).json
```

Schedule with cron:
```bash
crontab -e
# Add:
0 2 * * * /home/pi/scrape-daily.sh
```

### Home Automation Dashboard

Render a web dashboard on an HDMI screen:
```bash
BRIDGE_HEADLESS=false pinchtab
curl -X POST http://localhost:9867/tabs \
  -H "Content-Type: application/json" \
  -d '{
    "url": "http://homeassistant.local:8123",
    "kiosk": true
  }'
```

### Automated Testing

Run Playwright/Puppeteer tests against pinchtab:
```javascript
// test.js
const { chromium } = require('playwright');

(async () => {
  const browser = await chromium.connectOverCDP('http://localhost:9867');
  const page = await browser.newPage();
  await page.goto('https://example.com');
  console.log(await page.title());
  await browser.close();
})();
```

## Monitoring

Check pinchtab health and resource usage:

```bash
# Health check
curl http://localhost:9867/health

# Metrics (if enabled)
curl http://localhost:9867/metrics

# System resources
htop  # Install: sudo apt install htop
```

## Upgrading

```bash
npm update -g pinchtab
pinchtab --version
```

## Uninstallation

```bash
# Remove pinchtab
npm uninstall -g pinchtab

# Remove Chrome profile data
rm -rf ~/.config/pinchtab
```

## Further Reading

- [Data Storage Guide](data-storage.md) — Where pinchtab stores files
- [Configuration Reference](../references/configuration.md) — All config options
- [API Reference](../references/api-reference.json) — REST API documentation
- [Memory Monitoring](memory-monitoring.md) — Track memory usage

## Community

- **GitHub**: https://github.com/pinchtab/pinchtab
- **Issues**: https://github.com/pinchtab/pinchtab/issues
- **Discussions**: https://github.com/pinchtab/pinchtab/discussions

---

**Have a cool Raspberry Pi + Pinchtab project?** Share it in GitHub Discussions!
