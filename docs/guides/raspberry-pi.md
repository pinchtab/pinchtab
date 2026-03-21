# Raspberry Pi

PinchTab runs on Raspberry Pi as long as Chromium or Chrome is available. The current implementation does not need Pi-specific feature flags, but it does benefit from conservative defaults because memory is limited.

## Recommended Baseline

- Raspberry Pi OS or Ubuntu on ARM64
- 64-bit userspace if possible
- Chromium installed locally
- headless mode by default
- low tab counts on smaller boards

## Install Chromium

On Raspberry Pi OS:

```bash
sudo apt update
sudo apt install -y chromium-browser
```

Verify the binary:

```bash
which chromium-browser
which chromium
```

If auto-detection misses it, set the binary path in config:

```bash
pinchtab config set browser.binary /usr/bin/chromium-browser
pinchtab server
```

## Install PinchTab

Use your normal PinchTab install path for the platform, or build the binary from this repository:

```bash
go build -o pinchtab ./cmd/pinchtab
```

Then start it:

```bash
./pinchtab server
```

## Pi-Friendly Config

Create a config file and keep most settings there instead of relying on old environment variables.

Example:

```json
{
  "browser": {
    "binary": "/usr/bin/chromium-browser",
    "extraFlags": "--disable-gpu --disable-dev-shm-usage"
  },
  "instanceDefaults": {
    "mode": "headless",
    "maxTabs": 5,
    "blockImages": true,
    "blockAds": true
  },
  "profiles": {
    "baseDir": "/home/pi/.config/pinchtab/profiles",
    "defaultProfile": "default"
  }
}
```

Run with it:

```bash
PINCHTAB_CONFIG=/home/pi/.config/pinchtab/config.json ./pinchtab
```

## Headless Vs Headed

For most Raspberry Pi workloads, keep the default:

```json
{
  "instanceDefaults": {
    "mode": "headless"
  }
}
```

If you are using a desktop session and want a visible browser, switch to:

```json
{
  "instanceDefaults": {
    "mode": "headed"
  }
}
```

Headed mode costs more RAM and is usually best kept for debugging.

## Storage

If the SD card is small or slow, move profile storage to a larger drive:

```json
{
  "profiles": {
    "baseDir": "/mnt/usb/pinchtab-profiles",
    "defaultProfile": "default"
  },
  "server": {
    "stateDir": "/mnt/usb/pinchtab-state"
  }
}
```

This is the current supported way to relocate data. Keep using the nested config keys rather than older flat config files.

## Running As A Service

Example `systemd` unit:

```ini
[Unit]
Description=PinchTab Browser Service
After=network.target

[Service]
Type=simple
User=pi
WorkingDirectory=/home/pi
ExecStart=/home/pi/pinchtab
Environment=PINCHTAB_CONFIG=/home/pi/.config/pinchtab/config.json
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

Then:

```bash
sudo systemctl daemon-reload
sudo systemctl enable pinchtab
sudo systemctl start pinchtab
sudo systemctl status pinchtab
```

## Performance Tips

- keep `instanceDefaults.maxTabs` low on 1 GB and 2 GB boards
- prefer headless mode
- block images and ads for scraping-heavy workloads
- move profiles to faster external storage if the SD card is the bottleneck
- add swap carefully if you are hitting OOM conditions often

## Troubleshooting

### Chrome Binary Not Found

Set `browser.binary` in config:

```bash
pinchtab config set browser.binary /usr/bin/chromium-browser
```

### Out Of Memory

Reduce workload in config:

```json
{
  "instanceDefaults": {
    "maxTabs": 3,
    "blockImages": true,
    "mode": "headless"
  }
}
```

### Port Already In Use

Change the port in config:

```bash
pinchtab config set server.port 9868
./pinchtab server
```
