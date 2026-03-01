# Default Instance Pattern: Simplified API Design

**Status:** Design Proposal  
**Date:** 2026-03-01  
**Purpose:** Support simplified single-instance workflows while maintaining full multi-instance capability  
**Related Issue:** [#65 - Single instance mode](https://github.com/pinchtab/pinchtab/issues/65)

---

## Overview

The **Default Instance Pattern** provides a simplified REST API for users who only need a single browser instance. Instead of managing multiple instances, users can directly interact with a default instance using clean, intuitive endpoints.

### Use Cases

1. **Simple automation** - Single-user browser automation scripts
2. **Testing frameworks** - Integration with test runners (Playwright-compatible)
3. **Quick scraping** - One-off web scraping tasks
4. **CLI tools** - Command-line utilities that need a browser
5. **Educational** - Learning browser automation without complexity

---

## API Structure

### Current Multi-Instance API
```
GET  /instances/{id}/status
POST /instances/{id}/navigate
POST /tabs/{tabId}/snapshot
GET  /tabs/{tabId}/screenshot
```

### Proposed Default Instance API
```
GET  /
POST /start
POST /navigate
GET  /snapshot
GET  /screenshot
```

This is NOT a replacement, but a **convenience layer** that:
- Routes to an internal default instance
- Manages lifecycle automatically
- Provides cleaner URLs for common use cases

---

## Endpoints (Default Instance Mode)

### Lifecycle

**GET /**
```bash
curl http://localhost:9867/

Response:
{
  "status": "running|stopped",
  "instanceId": "inst_default",
  "instanceName": "default",
  "browser": "chrome",
  "chromeBinary": "/path/to/chrome",
  "startTime": "2026-03-01T15:12:34Z",
  "uptime": 3600
}
```

**POST /start**
```bash
curl -X POST http://localhost:9867/start \
  -H "Content-Type: application/json" \
  -d '{"mode":"headless"}'

Response:
{
  "status": "starting",
  "instanceId": "inst_default",
  "port": 9868
}
```

**POST /stop**
```bash
curl -X POST http://localhost:9867/stop

Response:
{
  "status": "stopped",
  "instanceId": "inst_default"
}
```

### Navigation & Tabs

**GET /tabs**
```bash
curl http://localhost:9867/tabs

Response:
[
  {
    "id": "tab_abc123",
    "title": "Example Domain",
    "url": "https://example.com/",
    "active": true
  }
]
```

**POST /tabs/open**
```bash
curl -X POST http://localhost:9867/tabs/open \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com"}'

Response:
{
  "id": "tab_abc123",
  "title": "Example Domain",
  "url": "https://example.com/"
}
```

**DELETE /tabs/{targetId}**
```bash
curl -X DELETE http://localhost:9867/tabs/tab_abc123

Response:
{
  "status": "closed",
  "id": "tab_abc123"
}
```

### Navigation

**POST /navigate**
```bash
curl -X POST http://localhost:9867/navigate \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com"}'

Response:
{
  "status": "navigated",
  "url": "https://example.com",
  "title": "Example Domain"
}
```

### Browser Operations

**GET /snapshot**
```bash
curl http://localhost:9867/snapshot

Response:
{
  "nodes": [...],
  "tree": "DOM tree in ARIA format"
}
```

**GET /screenshot**
```bash
curl http://localhost:9867/screenshot \
  -o page.png
```

**GET /pdf**
```bash
curl http://localhost:9867/pdf?landscape=true \
  -o report.pdf
```

**POST /action** (Unified Action Endpoint)
```bash
curl -X POST http://localhost:9867/action \
  -H "Content-Type: application/json" \
  -d '{
    "action": "click",
    "ref": "e5"
  }'

# Supported actions:
# - click, type, fill, select, press, hover, drag, resize, focus, scroll
# - wait, evaluate, close, highlight
```

### Advanced Operations

**POST /evaluate**
```bash
curl -X POST http://localhost:9867/evaluate \
  -H "Content-Type: application/json" \
  -d '{"expression":"document.title"}'

Response:
{
  "result": "Example Domain"
}
```

**POST /highlight**
```bash
curl -X POST http://localhost:9867/highlight \
  -H "Content-Type: application/json" \
  -d '{"ref":"e5"}'
```

**GET /text**
```bash
curl http://localhost:9867/text

Response:
{
  "text": "Page text content..."
}
```

**POST /screenshot**
```bash
curl -X POST http://localhost:9867/screenshot \
  -H "Content-Type: application/json" \
  -d '{"selector":".main"}'
```

**GET /download**
```bash
# Trigger file download (returns filename and size info)
curl http://localhost:9867/download

Response:
{
  "filename": "document.pdf",
  "size": 1024000,
  "ready": true
}
```

---

## Implementation Strategy

### Phase 1: Route Handling
Add routes that map to default instance:

```go
// In orchestrator/handlers.go or dashboard/cmd_dashboard.go

// Health/status
mux.HandleFunc("GET /", handleDefaultStatus)
mux.HandleFunc("POST /start", handleDefaultStart)
mux.HandleFunc("POST /stop", handleDefaultStop)

// Tabs
mux.HandleFunc("GET /tabs", handleDefaultListTabs)
mux.HandleFunc("POST /tabs/open", handleDefaultOpenTab)
mux.HandleFunc("DELETE /tabs/{targetId}", handleDefaultCloseTab)

// Operations
mux.HandleFunc("POST /navigate", handleDefaultNavigate)
mux.HandleFunc("GET /snapshot", handleDefaultSnapshot)
mux.HandleFunc("GET /screenshot", handleDefaultScreenshot)
mux.HandleFunc("GET /pdf", handleDefaultPDF)
mux.HandleFunc("POST /action", handleDefaultAction)
mux.HandleFunc("GET /text", handleDefaultText)
// ... etc
```

### Phase 2: Default Instance Manager
Create a helper to manage the default instance:

```go
type DefaultInstanceManager struct {
    orchestrator *Orchestrator
    instanceId   string  // Always "inst_default"
    mu           sync.RWMutex
}

func (m *DefaultInstanceManager) Start(mode string) error
func (m *DefaultInstanceManager) Stop() error
func (m *DefaultInstanceManager) Status() InstanceStatus
func (m *DefaultInstanceManager) GetTabId() (string, error)  // Returns active tab
func (m *DefaultInstanceManager) EnsureRunning() error
```

### Phase 3: Handler Implementation
Each handler resolves to orchestrator calls:

```go
func handleDefaultSnapshot(w http.ResponseWriter, r *http.Request) {
    mgr := getDefaultInstanceManager()
    tabId, err := mgr.GetTabId()
    if err != nil {
        web.Error(w, 503, err)
        return
    }
    
    // Route to: GET /tabs/{tabId}/snapshot
    o.handleTabSnapshot(w, r, tabId)
}
```

### Phase 4: Backward Compatibility
- Default instance routes coexist with multi-instance routes
- No changes to existing `/instances/{id}/*` routes
- Users can still use full orchestrator API if they want

### Phase 5: Helm/Docker Configuration
Add flag to enable/disable default instance:

```bash
# Docker
docker run -e PINCHTAB_DEFAULT_INSTANCE=1 pinchtab:latest

# CLI
pinchtab dashboard --default-instance
```

---

## Architecture Diagram

```
Client (simplified API)
    ↓
GET /
POST /navigate
GET /snapshot
POST /action
    ↓
Default Instance Manager
    ↓
Orchestrator (internal routing)
    ↓
GET /tabs/{defaultTabId}/snapshot
POST /tabs/{defaultTabId}/action
    ↓
Instance Bridge (port 9868)
    ↓
Chrome
```

---

## Relationship to Issue #65

**Issue #65** likely requests a simpler, more direct API for users who don't need multi-instance complexity.

### How This Design Satisfies It

| Requirement | Solution |
|---|---|
| Single instance mode | Default instance pattern |
| Simplified URLs | No instance ID in path |
| Less configuration | Auto-manages default instance |
| Familiar API | Similar to Playwright/Puppeteer |
| Optional | Can disable, use full API instead |

### Example Workflow (Issue #65)

Before (multi-instance):
```bash
# Create instance
INST=$(curl -s -X POST http://localhost:9867/instances/launch \
  -d '{"mode":"headless"}' | jq -r '.id')
sleep 2

# Navigate
TAB=$(curl -s -X POST http://localhost:9867/instances/$INST/tabs/open \
  -d '{"url":"https://example.com"}' | jq -r '.id')

# Snapshot
curl http://localhost:9867/tabs/$TAB/snapshot
```

After (default instance):
```bash
# Start default instance
curl -X POST http://localhost:9867/start

# Navigate
curl -X POST http://localhost:9867/navigate \
  -d '{"url":"https://example.com"}'

# Snapshot
curl http://localhost:9867/snapshot
```

Much simpler! ✨

---

## Testing Strategy

### Unit Tests
- `DefaultInstanceManager` lifecycle management
- Route handlers for error conditions
- Tab ID resolution

### Integration Tests
```go
func TestDefaultInstanceWorkflow(t *testing.T) {
    // Start default instance
    // Navigate
    // Snapshot
    // Stop
    // Verify state
}
```

### Compatibility Tests
- Multi-instance and default instance can coexist
- Switching between modes works
- No cross-contamination

---

## Deployment Considerations

### Backward Compatibility
✅ No breaking changes to existing API  
✅ Opt-in feature (can be disabled)  
✅ Existing orchestrator routes unchanged

### Configuration
```yaml
# config.yaml
default_instance:
  enabled: true
  auto_start: false  # Or true if always-on
  mode: headless     # Or headed
  lazy_chrome: true
```

### Monitoring
- Track default instance usage
- Log lifecycle events
- Metrics: uptime, memory, operations/sec

---

## Future Extensions

### Multiple Named Instances
```bash
GET /instances/production
GET /instances/staging
POST /instances/testing/navigate
```

### Session Persistence
```bash
POST /save-session
GET /restore-session
```

### Headless vs Headed
```bash
POST /start?headed=true
```

---

## Migration Path

### Step 1 (Now)
- Implement default instance handlers
- Route to existing orchestrator
- Document API

### Step 2 (Next)
- Add Playwright adapter
- Support `@playwright/test`
- Test compatibility

### Step 3 (Future)
- Docker image with default instance
- CLI preset for single-instance mode
- Web UI for default instance

---

## Example Usage Patterns

### Shell Script
```bash
#!/bin/bash
curl -X POST http://localhost:9867/start
curl -X POST http://localhost:9867/navigate -d '{"url":"https://example.com"}'
curl http://localhost:9867/snapshot | jq '.nodes'
curl -X POST http://localhost:9867/stop
```

### Python
```python
import requests

BASE = "http://localhost:9867"

# Start
requests.post(f"{BASE}/start", json={"mode": "headless"})

# Navigate
requests.post(f"{BASE}/navigate", json={"url": "https://example.com"})

# Snapshot
snap = requests.get(f"{BASE}/snapshot").json()
print(snap["nodes"])

# Stop
requests.post(f"{BASE}/stop")
```

### JavaScript
```javascript
const BASE = "http://localhost:9867";

async function automate() {
  // Start
  await fetch(`${BASE}/start`, { 
    method: "POST",
    body: JSON.stringify({ mode: "headless" })
  });

  // Navigate
  await fetch(`${BASE}/navigate`, {
    method: "POST",
    body: JSON.stringify({ url: "https://example.com" })
  });

  // Snapshot
  const snap = await fetch(`${BASE}/snapshot`).then(r => r.json());
  console.log(snap.nodes);

  // Stop
  await fetch(`${BASE}/stop`, { method: "POST" });
}

automate();
```

---

## Success Criteria

- ✅ All default instance endpoints work
- ✅ Routes correctly proxy to orchestrator
- ✅ Multi-instance mode still works
- ✅ Documentation covers both patterns
- ✅ Tests for default instance workflows
- ✅ No performance degradation
- ✅ Users prefer it for single-instance tasks

---

## Questions & Decisions

**Q1: Should default instance be always running?**
- **A:** Optional. Can auto-start or lazy-init on first request.

**Q2: What happens to multi-instance during default-instance use?**
- **A:** Fully supported. Can launch other instances independently.

**Q3: Can you switch between default and multi-instance APIs?**
- **A:** Yes. Both work simultaneously on the same server.

**Q4: Is default instance a "first tab only" concept?**
- **A:** Initially yes (for simplicity). Can expand to multi-tab later.

**Q5: How does this relate to profiles?**
- **A:** Default instance uses default profile (or specified profile).

---

## Conclusion

The **Default Instance Pattern** provides:

1. **Simplicity** - Clean API for single-instance use cases
2. **Compatibility** - Doesn't break multi-instance system
3. **Flexibility** - Can opt-in or opt-out
4. **Familiarity** - Similar to Playwright/Puppeteer
5. **Migration path** - Route to full orchestrator when needed

This design **satisfies issue #65** as a particular case of the multi-instance/profile system, where the "special case" is using a single default instance without explicit instance management.

---

## Implementation Checklist

- [ ] Design review (this doc)
- [ ] Create DefaultInstanceManager
- [ ] Implement lifecycle handlers (/start, /stop, /)
- [ ] Implement tab handlers (/tabs, /tabs/open)
- [ ] Implement operation handlers (/navigate, /snapshot, /action)
- [ ] Integration tests
- [ ] Documentation
- [ ] Example scripts
- [ ] Docker image
- [ ] Release notes

---

**Next Step:** Review this design, gather feedback, then implement Phase 1 (route handling).
