# PinchTab API Structure: New (Tab-centric)

## Simplified, Flat API Focused on Tabs as Primary Resource

### Resource Hierarchy

```
Profile
  ↓ (create instance with profile)
Instance (orchestrator resource)
  ↓ (create tab in instance)
Tab (primary resource - where all work happens)
  ├─ Navigate
  ├─ Snapshot
  ├─ Action / Actions
  ├─ Text
  ├─ Evaluate
  ├─ Screenshot
  ├─ PDF
  ├─ Cookies
  ├─ Lock / Unlock
  └─ Close
```

---

## Complete Endpoint List

### Profile Management
| Method | Path | Payload | Response | Purpose |
|--------|------|---------|----------|---------|
| GET | `/profiles` | - | `[{id, name, createdAt, usedAt}]` | List profiles |
| POST | `/profiles` | `{name}` | `{id, name, createdAt}` | Create profile |
| GET | `/profiles/{id}` | - | `{id, name, createdAt, usedAt}` | Get profile info |
| DELETE | `/profiles/{id}` | - | `{id, deleted: true}` | Delete profile |

### Instance Management
| Method | Path | Payload | Response | Purpose |
|--------|------|---------|----------|---------|
| GET | `/instances` | - | `[{id, profileId, port, mode, status, startTime}]` | List instances |
| POST | `/instances/start` | `{profile, mode?, port?}` | `{id, profileId, port, mode, status}` | Start instance |
| POST | `/instances/{id}/stop` | - | `{id, stopped: true}` | Stop instance |
| GET | `/instances/{id}/logs` | - | `text` | Get instance logs |

### Tab Management (NEW)
| Method | Path | Payload | Response | Purpose |
|--------|------|---------|----------|---------|
| POST | `/tabs/new` | `{instanceId, url?}` | `{id, instanceId, url?, status}` | Create tab |
| GET | `/tabs` | - | `[{id, instanceId, url, title, type}]` | List all tabs |
| GET | `/tabs?instanceId={id}` | - | `[{id, instanceId, url, title}]` | List instance tabs |
| GET | `/tabs/{id}` | - | `{id, instanceId, url, title, type, status}` | Get tab info |
| POST | `/tabs/{id}/close` | - | `{id, closed: true}` | Close tab |

### Tab Operations (NEW - Replaces /instances/{id}/...)
| Method | Path | Query/Body | Response | Purpose |
|--------|------|-----------|----------|---------|
| **Navigate** |
| POST | `/tabs/{id}/navigate` | `{url, timeout?, blockImages?, blockAds?}` | `{url, status, title}` | Navigate to URL |
| **Snapshot** |
| GET | `/tabs/{id}/snapshot` | `?interactive&compact&depth=3&maxTokens=2000` | `{elements: [], tree: ...}` | Page structure |
| **Action** |
| POST | `/tabs/{id}/action` | `{kind, ref?, text?, key?, value?, ...}` | `{kind, result?, error?}` | Single action |
| POST | `/tabs/{id}/actions` | `{actions: [{kind, ...}, ...]}` | `{results: [...]}` | Multiple actions |
| **Text & Evaluation** |
| GET | `/tabs/{id}/text` | `?raw` | `{text: "...", elements: [...]}` | Extract text |
| POST | `/tabs/{id}/evaluate` | `{expression, await?}` | `{result: any, type: string}` | Run JS |
| **Media** |
| GET | `/tabs/{id}/screenshot` | `?format=png&quality=80` | `binary (PNG/JPEG)` | Screenshot |
| GET | `/tabs/{id}/pdf` | `?landscape&margins=0.5&scale=1.0&pages=1-3` | `binary (PDF)` | PDF export |
| **Cookies** |
| GET | `/tabs/{id}/cookies` | - | `[{name, value, domain, ...}]` | Get cookies |
| POST | `/tabs/{id}/cookies` | `{action: "set"|"delete", cookies: [...]}` | `{set: [...], deleted: [...]}` | Manage cookies |
| **Locking** |
| POST | `/tabs/{id}/lock` | `{owner: string, ttl: number}` | `{locked: true, owner, expiresAt}` | Lock tab |
| POST | `/tabs/{id}/unlock` | `{owner: string}` | `{unlocked: true}` | Unlock tab |
| GET | `/tabs/{id}/locks` | - | `{locked: bool, owner?, expiresAt?}` | Check lock |
| **Fingerprinting** |
| POST | `/tabs/{id}/fingerprint/rotate` | - | `{rotated: true, userAgent, ...}` | Rotate fingerprint |
| GET | `/tabs/{id}/fingerprint/status` | - | `{stealth: "light"|"full", ua, ...}` | Get fingerprint |

---

## Comparison: Old vs New

### Example 1: Click Element

**OLD (instance-scoped):**
```bash
curl -X POST http://localhost:9867/instances/inst_abc123/action \
  -d '{"kind": "click", "ref": "e5"}'
```
- Requires instance ID
- Path has 3 segments

**NEW (tab-scoped):**
```bash
curl -X POST http://localhost:9867/tabs/tab_xyz789/action \
  -d '{"kind": "click", "ref": "e5"}'
```
- Requires tab ID (which identifies instance)
- Path has 2 segments
- Cleaner for multi-tab workflows

---

### Example 2: Navigate to URL

**OLD:**
```bash
curl -X POST http://localhost:9867/instances/inst_abc123/navigate \
  -d '{"url": "https://example.com"}'
```

**NEW:**
```bash
curl -X POST http://localhost:9867/tabs/tab_xyz789/navigate \
  -d '{"url": "https://example.com"}'
```

---

### Example 3: Create Tab

**OLD:**
```bash
curl -X POST http://localhost:9867/instances/inst_abc123/tab \
  -d '{}'
# Response: {id: "tab_xyz", ...}
```

**NEW:**
```bash
curl -X POST http://localhost:9867/tabs/new \
  -d '{"instanceId": "inst_abc123"}'
# Response: {id: "tab_xyz", ...}
```

---

### Example 4: List tabs

**OLD:**
```bash
curl http://localhost:9867/instances/inst_abc123/tabs
# Returns just tabs from that instance
```

**NEW:**
```bash
# All tabs (across instances)
curl http://localhost:9867/tabs

# Specific instance
curl http://localhost:9867/tabs?instanceId=inst_abc123
```

---

## Query Parameters vs Body

### Query Parameters (GET)
```bash
# Snapshot with options
curl 'http://localhost:9867/tabs/tab_xyz/snapshot?interactive&compact&depth=2&maxTokens=2000'

# Screenshot with format
curl 'http://localhost:9867/tabs/tab_xyz/screenshot?format=jpeg&quality=85'

# List with filter
curl 'http://localhost:9867/tabs?instanceId=inst_abc123'
```

### Body (POST)
```bash
# Navigate with options
curl -X POST http://localhost:9867/tabs/tab_xyz/navigate \
  -d '{
    "url": "https://example.com",
    "timeout": 30,
    "blockImages": true,
    "blockAds": false
  }'

# Action (already body-based)
curl -X POST http://localhost:9867/tabs/tab_xyz/action \
  -d '{
    "kind": "click",
    "ref": "e5",
    "timeout": 5
  }'
```

---

## HTTP Status Codes

| Code | Meaning | Example |
|------|---------|---------|
| **200** | Success (GET/action) | Snapshot returned, action completed |
| **201** | Created | Tab created, instance started |
| **204** | No content | Close successful |
| **400** | Bad request | Invalid action kind, malformed JSON |
| **404** | Not found | Tab ID doesn't exist, instance stopped |
| **409** | Conflict | Tab locked by another agent, port in use |
| **500** | Server error | Chrome crashed, internal error |
| **503** | Unavailable | Chrome not initialized, instance starting |

---

## Response Format

### Success Response
```json
{
  "kind": "action",
  "result": {
    "text": "Element text",
    "visible": true,
    "rect": {...}
  }
}
```

### Error Response
```json
{
  "error": "tab not found",
  "code": "ERR_TAB_NOT_FOUND",
  "statusCode": 404
}
```

### Snapshot Response
```json
{
  "elements": [
    {
      "ref": "e1",
      "tag": "button",
      "text": "Click me",
      "interactive": true,
      "rect": {"x": 100, "y": 50, "w": 80, "h": 30}
    }
  ],
  "tree": {...},
  "meta": {"count": 45, "interactive": 12}
}
```

---

## CLI Command Mapping

### Profile Management
```bash
# List
pinchtab profiles

# Create
pinchtab profile create my-profile

# Delete
pinchtab profile delete my-profile
```

### Instance Management
```bash
# List
pinchtab instances

# Start
pinchtab instance start --profile my-profile --mode headed --port 9868

# Stop
pinchtab instance stop inst_abc123

# Logs
pinchtab instance logs inst_abc123
```

### Tab Management
```bash
# List all tabs
pinchtab tabs

# List instance tabs
pinchtab --instance inst_abc123 tabs

# Create tab
pinchtab --instance inst_abc123 tab new https://example.com
# → tab_xyz789

# Close tab
pinchtab --tab tab_xyz789 close
```

### Tab Operations
```bash
# Navigate
pinchtab --tab tab_xyz789 nav https://example.com

# Snapshot
pinchtab --tab tab_xyz789 snap -i -c

# Click
pinchtab --tab tab_xyz789 click e5

# Get text
pinchtab --tab tab_xyz789 text

# Run action
cat << 'EOF' | pinchtab --tab tab_xyz789 action
{
  "kind": "actions",
  "actions": [
    {"kind": "click", "ref": "e1"},
    {"kind": "type", "ref": "e2", "text": "search"},
    {"kind": "press", "key": "Enter"}
  ]
}
EOF

# Lock tab (exclusive access)
pinchtab --tab tab_xyz789 lock --owner my-agent --ttl 60

# Unlock
pinchtab --tab tab_xyz789 unlock --owner my-agent
```

---

## Full Workflow Example

### Setup
```bash
# 1. Create profile
PROF=$(pinchtab profile create my-app | jq -r .id)

# 2. Start instance
INST=$(pinchtab instance start --profile my-app --mode headed | jq -r .id)

# 3. Create tab
TAB=$(pinchtab --instance $INST tab new | jq -r .id)
```

### Work
```bash
# 4. Navigate
pinchtab --tab $TAB nav https://example.com

# 5. Get page structure
pinchtab --tab $TAB snap -i -c | jq .

# 6. Interact
pinchtab --tab $TAB click e5
pinchtab --tab $TAB type e12 "search text"
pinchtab --tab $TAB press Enter

# 7. Check result
pinchtab --tab $TAB snap -d | jq .

# 8. Extract data
pinchtab --tab $TAB text
```

### Cleanup
```bash
# 9. Close tab (optional, instance cleanup closes all)
pinchtab --tab $TAB close

# 10. Stop instance
pinchtab instance stop $INST
```

---

## Data Flow

### Request Path
```
CLI Command
  ↓
HTTP Request (curl or SDK)
  ↓
Orchestrator Server (9867)
  ↓
Tab Resolver (tab ID → instance ID)
  ↓
Route to Instance (HTTP call to 9868+)
  ↓
Bridge Server
  ↓
Chrome (DevTools Protocol)
```

### Response Path
```
Chrome responds
  ↓
Bridge returns JSON
  ↓
Instance Server returns JSON
  ↓
Orchestrator aggregates/proxies
  ↓
CLI/SDK receives JSON
```

---

## Deprecation Timeline

### Phase 1 (Now - v1.0)
- New tab endpoints available
- Old instance endpoints still work
- Deprecation headers on old endpoints

### Phase 2 (v1.1)
- CLI prefers `--tab` over `--instance`
- Documentation focuses on new API
- Users migrated gradually

### Phase 3 (v2.0)
- Old instance endpoints removed
- Only tab-centric API
