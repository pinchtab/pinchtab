# Headless vs Headed

PinchTab can run Chrome in two modes: **Headless** (no visible UI) and **Headed** (visible window). Understanding the tradeoffs helps you choose the right mode for your workflow.

---

## Headless Mode (Default)

Chrome runs **without a visible window**. All interactions happen via the API.

```bash
# Start headless (default)
pinchtab
# or explicitly
pinchtab --headless
```

### Characteristics

- ✅ **No UI overhead** — No window rendering, faster operations
- ✅ **Scriptable** — Perfect for automation, CI/CD, unattended workflows
- ✅ **Lightweight** — Lower CPU/memory than headed mode
- ✅ **Remote-friendly** — Works over SSH, Docker, cloud servers
- ❌ **Can't see what's happening** — Debugging requires screenshots or logs

### Use Cases

- **AI agents** — Automated tasks, form filling, data extraction
- **CI/CD pipelines** — Testing, scraping, report generation
- **Cloud servers** — VPS, Lambda, container orchestration
- **Production workflows** — Long-running tasks, batch processing

### Performance

Headless is **faster** for most operations:

```text
Navigate:          200-500ms (no rendering)
Snapshot:          100-300ms (no painting)
Click + verify:    300-700ms (no visual feedback)
```

---

## Headed Mode

Chrome runs **with a visible window** that you can see and interact with.

```bash
# Start headed
pinchtab --headed
```

### Characteristics

- ✅ **Visual feedback** — See exactly what's happening in real-time
- ✅ **Debuggable** — Watch the browser, inspect elements, debug flows
- ✅ **Interactive** — You can click, type, scroll in the window manually
- ✅ **Development-friendly** — Great for testing, debugging, prototyping
- ❌ **Slower** — Window rendering adds latency
- ❌ **Requires a display** — Needs X11/Wayland on Linux, native desktop on macOS/Windows
- ❌ **Resource-heavy** — More CPU/memory for rendering

### Use Cases

- **Development & debugging** — Build and test automation scripts
- **Local testing** — Verify workflows before production
- **Live demonstrations** — Show what your automation is doing
- **Interactive debugging** — Watch and modify behavior in real-time
- **Manual collaboration** — A human watches and guides the automation

### Performance

Headed is **slower** due to rendering:

```text
Navigate:          400-900ms (rendering overhead)
Snapshot:          300-800ms (painting + encoding)
Click + verify:    800-1500ms (visual confirmation)
```

Expect **2-3x latency increase** compared to headless.

---

## Side-by-Side Comparison

| Aspect | Headless | Headed |
|---|---|---|
| **Visibility** | ❌ Invisible | ✅ Visible window |
| **Speed** | ✅ Fast | ❌ Slower (2-3x) |
| **Resource usage** | ✅ Light | ❌ Heavy |
| **Debugging** | ❌ Hard | ✅ Easy |
| **Display required** | ❌ No | ✅ Yes (X11/Wayland/native) |
| **Automation** | ✅ Perfect | ⚠️ Can interact manually |
| **CI/CD** | ✅ Ideal | ❌ Not practical |
| **Development** | ⚠️ Possible | ✅ Recommended |

---

## When to Use Headless

**Use headless for:**
- Production automation (scripts, agents, workflows)
- CI/CD pipelines (GitHub Actions, GitLab CI, Jenkins)
- Unattended execution (servers, containers, cloud functions)
- High-throughput tasks (scraping 1000s of pages)
- Cost-sensitive environments (minimize CPU/memory)
- Long-running processes (24/7 automation)

```bash
# Production setup: headless instance
pinchtab --port 9867
```

---

## When to Use Headed

**Use headed for:**
- Local development (debugging scripts)
- Testing automation behavior
- Demonstrating workflows to humans
- Prototyping and experimentation
- Interactive debugging (pause and inspect)
- Manual verification before production

```bash
# Development setup: headed instance with profile
BRIDGE_PROFILE=dev pinchtab --headed --port 9867
```

---

## Switching Modes

You can switch between modes by restarting:

```bash
# Running headless
pinchtab --port 9867
# Ctrl+C to stop

# Switch to headed
pinchtab --headed --port 9867
```

**No state is lost** if using a profile:

```bash
# Headless session
BRIDGE_PROFILE=work pinchtab --port 9867
# ... login, do work ...
# Ctrl+C

# Switch to headed with same profile
BRIDGE_PROFILE=work pinchtab --headed --port 9867
# ... profile, cookies, tabs are restored ...
```

---

## Display Requirements

### On macOS
- Native window system — headed works out of the box
- ```bash
  pinchtab --headed
  ```

### On Linux
- Requires X11 or Wayland display server
- In a Docker container: pass `DISPLAY` environment variable
- In a headless server: headless mode only
- ```bash
  # X11 forwarding over SSH
  ssh -X user@server
  pinchtab --headed
  ```

### On Windows
- Native window system — headed works out of the box
- ```bash
  pinchtab --headed
  ```

### In Docker (Headless)
```dockerfile
# Headless works everywhere
FROM pinchtab/pinchtab:latest
CMD pinchtab
```

### In Docker (Headed)
```dockerfile
# Headed requires display forwarding
FROM pinchtab/pinchtab:latest
ENV DISPLAY=:0
CMD pinchtab --headed
```

Then run with:
```bash
docker run -e DISPLAY=$DISPLAY -v /tmp/.X11-unix:/tmp/.X11-unix:rw pinchtab/pinchtab
```

---

## Best Practices

### Development Workflow

```bash
# 1. Start headed for debugging
BRIDGE_PROFILE=dev pinchtab --headed

# 2. Build and test your automation
# (in another terminal)
curl ... # test API calls

# 3. Verify behavior in the visible window
# 4. Once stable, switch to headless

# Terminal 1: Ctrl+C to stop headed
# Terminal 1: Start headless (production)
pinchtab --port 9867
```

### CI/CD Pipeline

```yaml
# Always headless in CI
test:
  script:
    - pinchtab --port 9867 &
    - npm test  # Runs against headless instance
    - pkill pinchtab
```

### Multi-Instance Setup

```bash
# Production: Multiple headless instances for scale
pinchtab --port 9867 &  # Instance 1
pinchtab --port 9868 &  # Instance 2
pinchtab --port 9869 &  # Instance 3

# Development: One headed instance for debugging
BRIDGE_PROFILE=dev pinchtab --headed --port 9870
```

---

## Performance Tips

### For Headless (Already Optimized)
- Default is fast
- Use `--headless` explicitly if needed

### For Headed (Optimize for Dev)
- Close unused tabs to reduce rendering load
- Use `?filter=interactive` for smaller snapshots
- Take screenshots sparingly (they're rendered + encoded)
- Minimize window size (less to render)

---

## Troubleshooting

### Headed Mode Not Opening a Window

**On Linux:**
- Check if `DISPLAY` is set: `echo $DISPLAY`
- If empty, you need X11 or Wayland
- Fallback to headless: `pinchtab --headless`

**On macOS/Windows:**
- Check if Chrome/Chromium is installed
- Ensure no display is required on your system

### Headed Too Slow for Automation

- Switch to headless for production
- Use headed only for development/debugging

### Headless But Want to Debug

- Take screenshots: `curl http://localhost:9867/screenshot?tabId=abc123 > page.jpg`
- Extract text: `curl http://localhost:9867/text?tabId=abc123`
- Use CLI tools: `pinchtab snap -i` to see structure

---

## Summary

- **Headless** = Fast, scriptable, production-ready
- **Headed** = Visible, debuggable, development-friendly

Choose headless for automation, headed for debugging. You can switch modes anytime without losing session state (if using profiles).
