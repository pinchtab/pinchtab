# Headless vs Headed

PinchTab instances can run Chrome in two modes: **Headless** (no visible UI) and **Headed** (visible window). Understanding the tradeoffs helps you choose the right mode for your workflow.

**Note:** You run a single orchestrator (`pinchtab`), then create instances with different modes via the API.

---

## Headless Mode (Default)

Chrome runs **without a visible window**. All interactions happen via the API.

```bash
# Start orchestrator (once)
pinchtab

# Terminal 2: Create headless instance (default)
pinchtab instance launch

# Or via curl
curl -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"mode": "headless"}'

# Response
{
  "id": "inst_0a89a5bb",
  "port": "9868",
  "headless": true,
  "status": "starting"
}
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

---

## Headed Mode

Chrome runs **with a visible window** that you can see and interact with.

```bash
# Start orchestrator (once)
pinchtab

# Terminal 2: Create headed instance
pinchtab instance launch --mode headed

# Or via curl
curl -X POST http://localhost:9867/instances/launch \
  -H "Content-Type: application/json" \
  -d '{"mode": "headed"}'

# Response
{
  "id": "inst_1b9a5dcc",
  "port": "9869",
  "headless": false,
  "status": "starting"
}
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
# Production: Create headless instance
pinchtab instance launch

# Or multiple headless instances for scale
for i in 1 2 3; do
  pinchtab instance launch --mode headless
done

# List all instances
curl http://localhost:9867/instances | jq .
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
# Development: Create headed instance with profile
pinchtab profile create dev

# Get profile ID
DEV_ID=$(pinchtab profiles | jq -r '.[] | select(.name=="dev") | .id')

# Start headed instance with profile
curl -X POST http://localhost:9867/instances/start \
  -H "Content-Type: application/json" \
  -d '{"profileId":"'$DEV_ID'","mode":"headed"}'

# Or simpler: start without persistent profile
pinchtab instance launch --mode headed
```

---

## Display Requirements

Display requirements apply to **headed instances**, not the orchestrator. The orchestrator can run anywhere and create instances in any mode.

### On macOS
- Native window system — headed instances work out of the box
- ```bash
  pinchtab  # Orchestrator
  # Terminal 2:
  pinchtab instance launch --mode headed
  ```

### On Linux
- Headless instances: Work anywhere (no display needed)
- Headed instances: Require X11 or Wayland display server
- In a Docker container: Forward `DISPLAY` environment variable
- In a headless server: Use headless mode only
- ```bash
  # Orchestrator on remote server (SSH)
  ssh user@server 'pinchtab &'
  
  # Create headed instance via X11 forwarding
  ssh -X user@server 'pinchtab instance launch --mode headed'
  ```

### On Windows
- Native window system — headed instances work out of the box
- ```bash
  pinchtab  # Orchestrator
  # Terminal 2:
  pinchtab instance launch --mode headed
  ```

### In Docker (Headless - Recommended)
```dockerfile
# Headless works everywhere, no display needed
FROM pinchtab/pinchtab:latest
CMD ["pinchtab"]
```

Run with:
```bash
docker run -d -p 9867:9867 pinchtab/pinchtab

# Create headless instances in the container
curl -X POST http://localhost:9867/instances/launch \
  -d '{"mode":"headless"}'
```

### In Docker (Headed - Advanced)
```dockerfile
# Headed requires display forwarding
FROM pinchtab/pinchtab:latest
ENV DISPLAY=:0
CMD ["pinchtab"]
```

Run with X11 forwarding:
```bash
docker run \
  -e DISPLAY=$DISPLAY \
  -v /tmp/.X11-unix:/tmp/.X11-unix:rw \
  -p 9867:9867 \
  pinchtab/pinchtab

# Create headed instances (display forwarded to host)
curl -X POST http://localhost:9867/instances/launch \
  -d '{"mode":"headed"}'
```

---

## Best Practices

### Development Workflow

```bash
# Terminal 1: Start orchestrator (once)
pinchtab

# Terminal 2: Create headed instance for debugging
DEV=$(pinchtab instance launch --mode headed | jq -r '.id')

# Terminal 3: Build and test your automation
curl -X POST http://localhost:9867/instances/$DEV/navigate \
  -d '{"url":"https://example.com"}'

# Verify behavior in the visible window while you develop
# ... iterate on your script ...

# When stable, test in headless for production
PROD=$(pinchtab instance launch --mode headless)

# Run full test suite against headless instance
# ... verify all tests pass ...

# Clean up
pinchtab instance stop $DEV
pinchtab instance stop $PROD
```

### CI/CD Pipeline

```yaml
# Always headless in CI
test:
  script:
    # Start orchestrator
    - pinchtab &
    - sleep 1  # Wait for orchestrator to be ready
    
    # Create headless instance
    - INST=$(curl -X POST http://localhost:9867/instances/launch | jq -r '.id')
    - sleep 2  # Wait for instance to initialize
    
    # Run tests against headless instance
    - npm test
    
    # Cleanup
    - curl -X POST http://localhost:9867/instances/$INST/stop
    - pkill pinchtab
```

### Multi-Instance Setup (Scale)

```bash
# Terminal 1: Start orchestrator (once)
pinchtab

# Terminal 2: Create multiple headless instances for scale
for i in 1 2 3; do
  INST=$(pinchtab instance launch --mode headless | jq -r '.id')
  echo "Created headless instance: $INST"
done

# List all instances
curl http://localhost:9867/instances | jq .

# Response: 3 independent headless instances
# [
#   {"id": "inst_xxx", "port": "9868", "headless": true, "status": "running"},
#   {"id": "inst_yyy", "port": "9869", "headless": true, "status": "running"},
#   {"id": "inst_zzz", "port": "9870", "headless": true, "status": "running"}
# ]

# Terminal 3: Route requests to instances via orchestrator
# All operations go through: http://localhost:9867/instances/{id}/...
```

### Multi-Instance with Mixed Modes

```bash
# Terminal 1: Start orchestrator
pinchtab

# Create persistent profiles
pinchtab profile create alice
pinchtab profile create bob
pinchtab profile create dev

# Get profile IDs
ALICE_ID=$(pinchtab profiles | jq -r '.[] | select(.name=="alice") | .id')
BOB_ID=$(pinchtab profiles | jq -r '.[] | select(.name=="bob") | .id')
DEV_ID=$(pinchtab profiles | jq -r '.[] | select(.name=="dev") | .id')

# Production: Multiple headless instances
ALICE_INST=$(curl -X POST http://localhost:9867/instances/start \
  -d '{"profileId":"'$ALICE_ID'","mode":"headless"}' | jq -r '.id')

BOB_INST=$(curl -X POST http://localhost:9867/instances/start \
  -d '{"profileId":"'$BOB_ID'","mode":"headless"}' | jq -r '.id')

# Development: One headed instance for debugging
DEV_INST=$(curl -X POST http://localhost:9867/instances/start \
  -d '{"profileId":"'$DEV_ID'","mode":"headed"}' | jq -r '.id')

# Each instance is isolated with its own profile/cookies
```

---

## Performance Tips

### For Headless Instances (Already Optimized)
- Headless is the default and already optimized
- Most operations are fast
- For high-throughput: Create multiple headless instances and load-balance

### For Headed Instances (Optimize for Dev)
- Close unused tabs to reduce rendering load
- Use snapshots instead of screenshots when possible (faster)
- Take screenshots sparingly (they're rendered + encoded, slower)
- Minimize window size (less to render)
- For faster debugging: Use headless for bulk work, headed only for visual debugging

### For Scale
- Use headless instances (no display overhead)
- Create multiple instances (one per worker/agent)
- Load-balance via `/instances` list and round-robin routing
- Monitor instance health via `GET /instances/{id}`

---

## Troubleshooting

### Headed Instance Not Opening a Window

**Root cause:** Display server not available

**On Linux:**
```bash
# Check if DISPLAY is set
echo $DISPLAY

# If empty or :0 unavailable, you need X11 or Wayland
# Fallback to headless: Use --mode headless instead
pinchtab instance launch --mode headless
```

**On macOS/Windows:**
- Verify Chrome/Chromium is installed
- Verify OS has native display server

### Headed Instance Too Slow

**Solution:** Use headless for production, headed only for development

```bash
# Switch from headed to headless
INST=$(pinchtab instance launch --mode headed)
# ... do some debugging ...
pinchtab instance stop $INST

# Create headless for production testing
INST=$(pinchtab instance launch --mode headless)
```

### Headless Instance But Need to Debug

**Solution:** Use API operations to see what's happening

```bash
# Get page structure (DOM)
curl http://localhost:9867/instances/$INST/snapshot | jq .

# Extract all text
curl http://localhost:9867/instances/$INST/text

# Take screenshot
curl http://localhost:9867/instances/$INST/screenshot > page.png

# Get page title and URL
curl http://localhost:9867/instances/$INST/tabs | jq '.[] | {title, url}'
```

### Instance Initialization Slow

**Symptom:** Instance stuck in "starting" state

**Solution:** Wait for instance to reach "running" state before making requests

```bash
# Check instance status
curl http://localhost:9867/instances/$INST | jq '.status'

# Poll until running (first request initializes Chrome: 5-20 seconds)
while [ "$(curl -s http://localhost:9867/instances/$INST | jq -r '.status')" != "running" ]; do
  sleep 0.5
done

# Now safe to use
curl -X POST http://localhost:9867/instances/$INST/navigate \
  -d '{"url":"https://example.com"}'
```

### Can't Connect to Instance

**Symptom:** 503 error when trying to navigate/snapshot

**Causes:**
1. Instance still initializing (check status)
2. Chrome crashed (check logs)
3. Port already in use (specify different port)

**Debug:**
```bash
# Get instance status and error details
curl http://localhost:9867/instances/$INST | jq .

# Get instance logs
curl http://localhost:9867/instances/$INST/logs

# Check if port is available
lsof -i :9868  # Shows what's using the port
```

---

## Summary

- **Headless instances** = Fast, scriptable, production-ready (no display overhead)
- **Headed instances** = Visible, debuggable, development-friendly (requires display)

### Key Points
- Start orchestrator once: `pinchtab`
- Create instances with mode via API: `instance launch --mode headless|headed`
- Profiles are optional but preserve state across mode switches
- You can have both headless and headed instances running simultaneously
- Switch modes by stopping one instance and creating another
- If using profiles, login state and cookies persist across mode switches

### Quick Reference
```bash
# Orchestrator (runs once)
pinchtab

# Create headless instance (production)
pinchtab instance launch --mode headless

# Create headed instance (development)
pinchtab instance launch --mode headed

# Create with persistent profile (state preserved on mode switch)
pinchtab profile create work
curl -X POST http://localhost:9867/instances/start \
  -d '{"profileId":"work-id","mode":"headed"}'
```

**Next:** See [Instance API Reference](references/instance-api.md) for complete instance management details.
