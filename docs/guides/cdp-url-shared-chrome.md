# Remote Chrome via CDP_URL

By default, Pinchtab launches its own isolated Chrome instance. But sometimes you want to **connect to an existing Chrome** instead — to share one browser across multiple agents, save memory in container environments, or integrate with external Chrome setups.

That's what `CDP_URL` does.

## What is CDP_URL?

`CDP_URL` is a WebSocket URL that points to Chrome's DevTools Protocol server:

```bash
CDP_URL=ws://localhost:9222/devtools/browser/b041f900-...
./pinchtab
# Instead of launching Chrome, Pinchtab connects to the existing instance at port 9222
```

When you set `CDP_URL`, Pinchtab:
- ✅ Skips launching its own Chrome
- ✅ Connects to the browser at that URL
- ✅ Creates tabs in the existing browser
- ✅ Shares that browser's session (cookies, localStorage, etc.)

## Use Cases

### 1. Multi-Agent Resource Sharing

**Problem:** Multiple agents on the same machine each launching their own Chrome eats memory:

```
Agent 1 → launches Chrome (1.3GB)
Agent 2 → launches Chrome (1.3GB)
Agent 3 → launches Chrome (1.3GB)
──────────────────────────────────
Total: 3.9GB+ just for browser instances 😬
```

**Solution:** All agents share one Chrome via CDP_URL:

```bash
# Start shared Chrome once
/Applications/Google\ Chrome.app/Contents/MacOS/Google\ Chrome \
  --remote-debugging-port=9222 &

# Get the CDP URL
CDP_WS=$(curl -s http://localhost:9222/json/version | jq -r '.webSocketDebuggerUrl')

# All agents connect to it
export CDP_URL="$CDP_WS"
./agent-1
./agent-2
./agent-3
```

**Benefit:** 1.3GB for Chrome + lightweight agent processes. **Save 2.6GB per agent.**

**Typical setup:**
- Agent A handles browser operations (creates/controls tabs)
- Agent B handles data processing (reads via Agent A's HTTP API)
- Agent C handles orchestration (no browser needed)

All three can point to the same Pinchtab instance instead of each running their own.

### 2. Integration Testing

**Problem:** Test scripts need to control the same browser in sequence:

```bash
./test-screenshot.sh
./test-login.sh
./test-checkout.sh
# Each script might have its own isolated Chrome
```

**Solution:** Start one Chrome, all scripts target it:

```bash
# Start test Chrome once
chrome --remote-debugging-port=9222 &

# All test scripts use the same browser
CDP_URL=ws://localhost:9222/... ./test-screenshot.sh
CDP_URL=ws://localhost:9222/... ./test-login.sh
CDP_URL=ws://localhost:9222/... ./test-checkout.sh
```

**Benefits:**
- Persistent session across tests (log in once, reuse)
- Simpler test setup (no per-test Chrome cleanup)
- Faster iteration (no Chrome startup overhead per test)

### 3. Container/Docker Deployments

**Problem:** Chrome runs in one container, your application in another:

```dockerfile
# Chrome container (separate)
FROM chromium:latest
EXPOSE 9222

# App container (separate)
FROM golang:latest
ENV CDP_URL=http://chrome-service:9222/...
RUN ./pinchtab
```

**Solution:** Pinchtab connects to the Chrome container via CDP_URL:

```bash
# Start Chrome container
docker run -d --name chrome \
  -p 9222:9222 \
  chromium:latest --remote-debugging-port=9222

# Start Pinchtab container, pointing at Chrome
docker run -d \
  -e CDP_URL=http://chrome:9222/devtools/browser/... \
  -p 9867:9867 \
  pinchtab/pinchtab
```

**Benefits:**
- Separate scaling (one Chrome for 10 Pinchtab instances)
- Simpler networking (Chrome container = stable endpoint)
- Cleaner orchestration (containers talk via service names)

## How to Use

### Find the CDP URL

Start Chrome with `--remote-debugging-port` and query the endpoint:

```bash
# macOS
/Applications/Google\ Chrome.app/Contents/MacOS/Google\ Chrome \
  --remote-debugging-port=9222 &

# Linux
google-chrome --remote-debugging-port=9222 &

# Docker
docker run -p 9222:9222 chromium:latest --remote-debugging-port=9222 &
```

Get the WebSocket URL:

```bash
curl http://localhost:9222/json/version | jq -r '.webSocketDebuggerUrl'
# Output: ws://localhost:9222/devtools/browser/b041f900-b164-4ad2-872d-359456b4e198
```

### Connect Pinchtab

```bash
export CDP_URL="ws://localhost:9222/devtools/browser/b041f900-..."
./pinchtab
```

That's it. Pinchtab will:
1. Connect to the existing Chrome
2. Discover existing tabs (if any)
3. Create new tabs as requested
4. Serve the HTTP API normally

### Multi-Agent Example

```bash
#!/bin/bash

# Start shared Chrome
chrome --remote-debugging-port=9222 &
CHROME_PID=$!

# Get the CDP URL
CDP_WS=$(curl -s http://localhost:9222/json/version | jq -r '.webSocketDebuggerUrl')

# Launch all agents pointing to the same Chrome
export CDP_URL="$CDP_WS"

# Start multiple agent instances
./agent-browser &        # Handles browser operations
./agent-processor &      # Processes data (uses HTTP)
./agent-orchestrator &   # Coordinates workflow

wait
```

## Limitations & Notes

**What works:**
- ✅ Creating tabs in existing Chrome
- ✅ Navigating to URLs
- ✅ Snapshots and actions
- ✅ Session persistence (cookies are stored in Chrome's profile)
- ✅ Multiple Pinchtab instances sharing one Chrome

**What doesn't work yet:**
- ❌ Launching Chrome from scratch (you have to start Chrome separately)
- ❌ Profile management (profiles belong to the remote Chrome, not Pinchtab)

**Performance:**
- Slightly higher latency per request (WebSocket to remote Chrome instead of local IPC)
- Memory savings: **~1.3GB per agent** when sharing

## Troubleshooting

### "Connection refused" or "Failed to open new tab"

Chrome might not have any windows open yet. Solution:

```bash
# Ensure Chrome has at least one window
chrome --remote-debugging-port=9222 --new-window &
```

### "invalid memory address"

Pinchtab failed to find a valid target. Make sure:

1. Chrome is running with `--remote-debugging-port`
2. The CDP_URL is correct (test with `curl`)
3. Chrome has at least one window/tab open

### Ports and networking

**Local machine only:**
```bash
export CDP_URL="ws://localhost:9222/..."
```

**Docker/Kubernetes (inter-container):**
```bash
export CDP_URL="ws://chrome-service:9222/..."  # Use service name
```

**Remote machine:**
```bash
# SSH tunnel (recommended for security)
ssh -L 9222:remote-chrome:9222 user@remote-host &
export CDP_URL="ws://localhost:9222/..."

# Or direct (less secure)
export CDP_URL="ws://remote-chrome.example.com:9222/..."
```

## Configuration Reference

| Env Var | Default | Effect |
|---------|---------|--------|
| `CDP_URL` | *(none)* | WebSocket URL to remote Chrome. When set, Pinchtab connects instead of launching |
| `BRIDGE_PORT` | `9867` | Local HTTP port (independent of Chrome port) |
| `BRIDGE_HEADLESS` | `true` | Ignored when using CDP_URL (remote Chrome state is independent) |
| `BRIDGE_PROFILE` | `~/.pinchtab/chrome-profile` | Session storage directory (still used even with CDP_URL) |
| `BRIDGE_NO_RESTORE` | `false` | Skip restoring tabs from previous session |

## Security ⚠️

**Important:** Chrome's DevTools Protocol has **no built-in authentication**. Anyone with access to the CDP port can fully control Chrome — steal cookies, read pages, make transactions, access any logged-in account.

### Local Machine (Safe)

If Chrome and Pinchtab run on the same local machine:

```bash
# ✅ Safe — listens on localhost only by default
chrome --remote-debugging-port=9222
export CDP_URL="ws://localhost:9222/..."
```

**What's safe:**
- Only processes on your machine can access it
- Other users on shared systems can still access it
- Docker containers on same host can access it

**Defense:** Use file permissions on Chrome's profile directory:
```bash
chmod 700 ~/.chrome-profile  # Only you can read
```

### Network Exposure (Critical Risk 🚨)

**Never expose the CDP port to the network:**

```bash
# ❌ DANGEROUS — exposes to entire network
chrome --remote-debugging-address=0.0.0.0 --remote-debugging-port=9222

# ❌ DANGEROUS — Docker exposes to host network
docker run -p 9222:9222 chromium:latest --remote-debugging-port=9222
```

**What attackers can do with network access:**
- Read every page Chrome visits
- Steal all cookies and auth tokens
- Make purchases, send emails, access bank accounts
- Execute arbitrary JavaScript
- Access sensitive data in localStorage/IndexedDB

### Safe Network Access: SSH Tunnel

If Chrome runs on a remote machine:

```bash
# On local machine, create secure tunnel
ssh -L 9222:localhost:9222 user@remote-host &

# Now Pinchtab uses the tunneled connection (encrypted via SSH)
export CDP_URL="ws://localhost:9222/..."
./pinchtab
```

This way:
- ✅ Communication is encrypted (SSH tunnel)
- ✅ Remote Chrome is not exposed to the network
- ✅ Only you can access it (requires SSH login)

### Docker/Kubernetes Best Practices

**Option 1: Internal Network Only**
```yaml
# Kubernetes Pod with Chrome + Pinchtab sidecars
spec:
  containers:
  - name: chrome
    ports:
    - containerPort: 9222  # No hostPort = not exposed to outside
    
  - name: pinchtab
    env:
    - name: CDP_URL
      value: "ws://localhost:9222/..."  # Talk within pod
```

**Option 2: Internal Service Only**
```yaml
# Don't expose Chrome service outside the cluster
apiVersion: v1
kind: Service
metadata:
  name: chrome
spec:
  type: ClusterIP  # ✅ Internal only, not LoadBalancer or NodePort
  ports:
  - port: 9222
```

**Option 3: SSH Tunnel for External Access**
```bash
# From outside cluster, use kubectl port-forward
kubectl port-forward service/chrome 9222:9222

# Then connect via tunnel
export CDP_URL="ws://localhost:9222/..."
```

### Checklist

- [ ] Chrome listens on `127.0.0.1` (localhost only) or is SSH-tunneled
- [ ] CDP port (9222) is not exposed to the network/internet
- [ ] No `--remote-debugging-address=0.0.0.0` flag
- [ ] Firewall blocks external access to the port (if on a server)
- [ ] Chrome profile directory has restricted permissions
- [ ] Only trusted agents/code have access to `CDP_URL`
- [ ] In containers: Chrome service is `ClusterIP`, not `LoadBalancer`

### Principle

Treat Chrome's CDP port like you'd treat:
- An SSH port (gives shell access to a machine)
- A database port (gives data access)
- An admin panel (gives system control)

**Restrict access ruthlessly. Use SSH tunnels for anything remote.**
