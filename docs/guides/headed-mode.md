# Headed Mode (manual local only)

PinchTab supports headed Chrome (a visible browser window) for local
development, debugging, and human-in-the-loop workflows. See
[Headless vs Headed](../headless-vs-headed.md) for the per-mode tradeoffs.

This page is the operational guide for *running* headed mode. Headed mode is
**manual-local-only**: PinchTab does not ship a supported path for headed mode
in containers or CI.

## Why the bundled Docker image is headless-only

The standard PinchTab Docker image (built from the repo `Dockerfile`) installs
Chromium without any X11 / Wayland / Xvfb packages. This is deliberate:

- The image targets headless automation, agents, and CI.
- Keeping the runtime free of a display stack keeps the image small, the
  attack surface narrow, and the startup cost low.
- Driving an X server from inside a container is fragile, host-specific, and
  not something PinchTab will support as a first-class deployment target.

If you try to launch a headed instance against the bundled image it will fail
(no display, no X libraries). That is expected.

If you need headed Chrome you must run PinchTab outside the image, or build
your own image with an X stack and a display server — both treated as manual
local workflows, not officially supported configurations.

## Running headed mode

### Linux native (recommended)

Run PinchTab directly on a Linux desktop session.

```bash
export DISPLAY=:0
pinchtab server &
pinchtab instance start --mode headed
```

This is the simplest and most reliable headed setup. Wayland sessions usually
work too because Chrome falls back to XWayland.

### Linux over SSH with X forwarding

```bash
ssh -X user@workstation 'DISPLAY=:0 pinchtab instance start --mode headed'
```

Forwarding works but is laggy for interactive use; prefer running PinchTab on
the workstation itself.

### Docker with X11 forwarding (manual, Linux host only)

Not supported out of the box, but achievable on a Linux host that already runs
an X server. You must build your own image that adds the X client libraries
Chromium needs, then forward the host's X socket:

```bash
# On the host, allow local containers to use the X server
xhost +local:

docker run --rm \
  -e DISPLAY="$DISPLAY" \
  -v /tmp/.X11-unix:/tmp/.X11-unix \
  -p 9867:9867 \
  your-pinchtab-image-with-x11
```

Caveats:

- Linux hosts only — `xhost` and `/tmp/.X11-unix` do not exist as-is on
  macOS or Windows hosts.
- `xhost +local:` weakens X access control; restrict it to trusted users.
- GPU acceleration, fonts, input devices, and clipboard integration are all
  host-specific and may behave differently than a native session.
- This is **not** the bundled image. You are responsible for the X stack you
  add on top.

### macOS via XQuartz

XQuartz can host X clients on macOS, including Chromium running in a Linux
container, but the experience is fragile:

```bash
# One-time: install XQuartz from https://www.xquartz.org/
# In XQuartz preferences → Security: enable "Allow connections from network clients"
open -a XQuartz
xhost + 127.0.0.1

docker run --rm \
  -e DISPLAY=host.docker.internal:0 \
  -p 9867:9867 \
  your-pinchtab-image-with-x11
```

Caveats:

- XQuartz must be running before you start the container.
- Performance is noticeably worse than native macOS apps.
- Some Chrome features (audio, GPU, certain input events) may not work.
- Prefer running PinchTab natively on macOS for headed work — `pinchtab server`
  on the host plus headed instances is the supported path.

### Windows via WSLg or VcXsrv

- **WSLg** (Windows 11 / recent Windows 10): WSL2 ships with a built-in
  Wayland/X compositor. Run PinchTab inside your WSL distro and headed Chrome
  appears on the Windows desktop with no extra configuration.
- **VcXsrv / Xming**: install an X server on Windows, run it with access
  control disabled for the loopback interface, and point `DISPLAY` from WSL or
  a container at the Windows host IP. Fragile; prefer WSLg.

Native Windows builds of `pinchtab` are best-effort — see
[Headless vs Headed](../headless-vs-headed.md#windows) for the broader Windows
support story.

## Headed mode in CI

Not supported. The headless image is the only configuration PinchTab tests in
CI, and the bundled Docker image will not run headed Chrome. If you need
headed-only behavior validated, run those tests on a developer workstation
with a real display.

## Fingerprint differences (CloakBrowser users)

Headed and headless Chrome have observably different fingerprints — different
`navigator.webdriver` handling, different GPU strings, different feature
flags, and different default window/viewport sizes. CloakBrowser narrows the
gap but cannot make headless mode pixel-identical to a headed session on the
same machine. If a target site treats your headed-on-laptop traffic as human
and your headless-in-container traffic as a bot, that is the fingerprint
delta, not a CloakBrowser bug — validate your flows in the mode you intend to
deploy in.

## Related guides

- [cloakbrowser.md](cloakbrowser.md) — CloakBrowser provider configuration
  and troubleshooting
- [docker.md](docker.md) — bundled headless image and CloakBrowser smoke
  image (both headless-only)
- [attach-chrome.md](attach-chrome.md) — attach to a separately launched
  headed Chrome or CloakBrowser via CDP
- [security.md](security.md) — security model for local and remote setups
