# Core Concepts

## Tab-Centric Design

Every operation targets a specific tab by `tabId`. Create a tab first:

```bash
curl -X POST http://localhost:9867/tab \
  -d '{"action":"new","url":"https://example.com"}' | jq '.tabId'
# Returns: "abc123"
```

Then use that `tabId` for all subsequent operations.

**Get page snapshot:**
```bash
curl "http://localhost:9867/snapshot?tabId=abc123"
```

**Extract text:**
```bash
curl "http://localhost:9867/text?tabId=abc123"
```

**Perform action (click):**
```bash
curl -X POST http://localhost:9867/action \
  -d '{"kind":"click","ref":"e5","tabId":"abc123"}'
```

## Refs Instead of Coordinates

The accessibility tree provides **stable element references** instead of pixel coordinates:

```json
{
  "elements": [
    {"ref": "e0", "role": "heading", "name": "Title"},
    {"ref": "e5", "role": "button", "name": "Submit"},
    {"ref": "e8", "role": "input", "name": "Email"}
  ]
}
```

Click or interact by ref:

```bash
curl -X POST http://localhost:9867/action \
  -d '{"kind":"click","ref":"e5","tabId":"abc123"}'
```

## Persistent Sessions

Tabs, cookies, and login state survive server restarts:

```bash
# Login
pinchtab nav https://example.com/login
pinchtab fill e3 user@example.com
pinchtab fill e5 password
pinchtab click e7

# Restart the server
pkill pinchtab
sleep 2
./pinchtab

# Tab is restored, still logged in
pinchtab nav https://example.com/dashboard
pinchtab snap  
# Works without re-login
```

---