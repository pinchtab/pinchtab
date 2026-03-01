# Profile API Reference

Profiles are browser user data directories. Each profile is an isolated Chrome user data directory where you can log into different accounts, save preferences, and maintain browser state.

## Quick Start

### List Profiles
```bash
# CLI
pinchtab profiles

# Curl
curl http://localhost:9867/profiles | jq .

# Response
[
  {
    "id": "278be873adeb",
    "name": "Pinchtab org",
    "created": "2026-02-27T20:37:13.599055326Z",
    "diskUsage": 534952089,
    "source": "created",
    "useWhen": "For gmail related to giago org"
  }
]
```

### Create Profile
```bash
# CLI (ready to implement)
pinchtab profile create my-profile

# Curl
curl -X POST http://localhost:9867/profiles \
  -H "Content-Type: application/json" \
  -d '{
    "name": "my-profile",
    "description": "For web scraping",
    "useWhen": "When extracting data from e-commerce"
  }'

# Response
{
  "status": "created",
  "name": "my-profile"
}
```

### Delete Profile
```bash
# CLI (ready to implement)
pinchtab profile delete my-profile

# Curl
curl -X DELETE http://localhost:9867/profiles/my-profile

# Response
{
  "status": "deleted",
  "id": "my-profile",
  "name": "my-profile"
}
```

---

## Complete API Reference

### 1. List Profiles

**Endpoint:** `GET /profiles`

**CLI:**
```bash
pinchtab profiles
```

**Curl:**
```bash
# List all profiles except temporary ones
curl http://localhost:9867/profiles

# Include temporary (auto-generated) profiles
curl 'http://localhost:9867/profiles?all=true'
```

**Response:** Array of ProfileInfo objects
```json
[
  {
    "id": "278be873adeb",
    "name": "Pinchtab org",
    "created": "2026-02-27T20:37:13.599055326Z",
    "diskUsage": 534952089,
    "source": "created",
    "running": false,
    "accountEmail": "admin@gi-ago.com",
    "accountName": "Luigi Agosti",
    "chromeProfileName": "Your Chrome",
    "hasAccount": true,
    "useWhen": "For gmail related to giago org",
    "description": ""
  }
]
```

**Query Parameters:**
- `all` (boolean, optional, default: false) — Include temporary profiles created by instances

---

### 2. Get Single Profile

**Endpoint:** `GET /profiles/{id}`

**Parameters:**
- `id` (string, path) — Profile ID (hash like `278be873adeb`) or name (like `my-profile`)

**Curl:**
```bash
# By ID
curl http://localhost:9867/profiles/278be873adeb

# By name (spaces are URL-encoded)
curl 'http://localhost:9867/profiles/Pinchtab%20org'
```

**Response:** Single ProfileInfo object
```json
{
  "id": "278be873adeb",
  "name": "Pinchtab org",
  "created": "2026-02-27T20:37:13.599055326Z",
  "diskUsage": 534952089,
  "source": "created",
  "running": false,
  "accountEmail": "admin@gi-ago.com",
  "accountName": "Luigi Agosti",
  "chromeProfileName": "Your Chrome",
  "hasAccount": true,
  "useWhen": "For gmail related to giago org",
  "description": ""
}
```

---

### 3. Create Profile

**Endpoint:** `POST /profiles`

**Parameters:** JSON body

**Curl:**
```bash
# Minimal (name only)
curl -X POST http://localhost:9867/profiles \
  -H "Content-Type: application/json" \
  -d '{"name": "my-profile"}'

# Full (with metadata)
curl -X POST http://localhost:9867/profiles \
  -H "Content-Type: application/json" \
  -d '{
    "name": "scraping-profile",
    "description": "Used for production web scraping",
    "useWhen": "When extracting data from e-commerce sites"
  }'
```

**Request Body:**
```json
{
  "name": "my-profile",
  "description": "Optional description",
  "useWhen": "Helps agents pick the right profile"
}
```

**Response:**
```json
{
  "status": "created",
  "name": "my-profile"
}
```

**Validation:**
- `name` is required and must be unique
- Special characters and spaces are allowed in names
- If name already exists, returns 400 error

---

### 4. Update Profile

**Endpoint:** `PATCH /profiles/{id}`

**Parameters:**
- `id` (string, path) — Profile ID or name
- `description` (string, body, optional) — Profile description
- `useWhen` (string, body, optional) — Use case guidance for agents

**Curl:**
```bash
curl -X PATCH http://localhost:9867/profiles/my-profile \
  -H "Content-Type: application/json" \
  -d '{
    "description": "Updated description",
    "useWhen": "Updated use case"
  }'
```

**Request Body:**
```json
{
  "description": "New description for the profile",
  "useWhen": "New use case guidance"
}
```

**Response:**
```json
{
  "status": "updated",
  "id": "my-profile",
  "name": "my-profile"
}
```

**Notes:**
- Only provided fields are updated
- Can omit either field to update only one

---

### 5. Delete Profile

**Endpoint:** `DELETE /profiles/{id}`

**Parameters:**
- `id` (string, path) — Profile ID or name

**CLI:**
```bash
pinchtab profile delete my-profile
```

**Curl:**
```bash
curl -X DELETE http://localhost:9867/profiles/my-profile
```

**Response:**
```json
{
  "status": "deleted",
  "id": "my-profile",
  "name": "my-profile"
}
```

**Notes:**
- Recursively deletes entire profile directory
- Returns 404 if profile not found
- Deletion is permanent and cannot be undone

---

### 6. Reset Profile

**Endpoint:** `POST /profiles/{id}/reset`

**Parameters:**
- `id` (string, path) — Profile ID or name

**Curl:**
```bash
curl -X POST http://localhost:9867/profiles/my-profile/reset
```

**Response:**
```json
{
  "status": "reset",
  "id": "my-profile",
  "name": "my-profile"
}
```

**What Gets Cleared:**
- Sessions
- Session Storage
- Cache
- Code Cache
- GPUCache
- Service Worker
- Cookies
- Cookies-journal
- History
- Visited Links

**Notes:**
- Profile directory structure remains
- User can re-login after reset
- Useful before sharing profile or starting fresh

---

### 7. Get Profile Logs

**Endpoint:** `GET /profiles/{id}/logs`

**Parameters:**
- `id` (string, path) — Profile ID or name
- `limit` (int, query, optional, default: 100) — Max number of log entries

**Curl:**
```bash
# Last 50 actions
curl 'http://localhost:9867/profiles/my-profile/logs?limit=50'

# All logs
curl 'http://localhost:9867/profiles/my-profile/logs?limit=1000'
```

**Response:** Array of ActionRecord objects
```json
[
  {
    "timestamp": "2026-03-01T05:12:45Z",
    "action": "navigate",
    "url": "https://example.com"
  },
  {
    "timestamp": "2026-03-01T05:12:50Z",
    "action": "click",
    "selector": "e5"
  },
  {
    "timestamp": "2026-03-01T05:12:55Z",
    "action": "type",
    "text": "search query"
  }
]
```

**Actions Include:**
- navigate, click, type, press, fill, hover, scroll, select, focus
- screenshot, snapshot, pdf, evaluate
- And more (see ActivityTracker)

---

### 8. Get Profile Analytics

**Endpoint:** `GET /profiles/{id}/analytics`

**Parameters:**
- `id` (string, path) — Profile ID or name

**Curl:**
```bash
curl http://localhost:9867/profiles/my-profile/analytics
```

**Response:** AnalyticsReport object
```json
{
  "totalActions": 256,
  "averageActionDuration": 2.5,
  "topActions": {
    "navigate": 128,
    "click": 98,
    "type": 30
  },
  "lastUsed": "2026-03-01T05:12:50Z",
  "commonHosts": [
    "example.com",
    "google.com",
    "github.com"
  ],
  "last24h": {
    "actions": 50,
    "uniqueHosts": 5
  }
}
```

**Insights Provided:**
- Total action count
- Average action duration
- Top 5 most used actions
- Last used timestamp
- Common hosts visited
- 24h activity summary

---

### 9. Import Profile

**Endpoint:** `POST /profiles/import`

**Parameters:** JSON body

**Curl:**
```bash
curl -X POST http://localhost:9867/profiles/import \
  -H "Content-Type: application/json" \
  -d '{
    "name": "imported-chrome",
    "sourcePath": "/Users/you/Library/Application Support/Google/Chrome/Default",
    "description": "Imported from my Chrome browser",
    "useWhen": "For existing Chrome accounts"
  }'
```

**Request Body:**
```json
{
  "name": "chrome-work",
  "sourcePath": "/path/to/chrome/profile",
  "description": "Optional description",
  "useWhen": "Optional use case"
}
```

**Response:**
```json
{
  "status": "imported",
  "name": "chrome-work"
}
```

**Common Source Paths:**

**macOS:**
```
/Users/<username>/Library/Application Support/Google/Chrome/Default
/Users/<username>/Library/Application Support/Google/Chrome/Profile 1
```

**Linux:**
```
~/.config/google-chrome/Default
~/.config/google-chrome/Profile 1
~/.config/chromium/Default
```

**Windows:**
```
C:\Users\<username>\AppData\Local\Google\Chrome\User Data\Default
C:\Users\<username>\AppData\Local\Chromium\User Data\Default
```

**Validation:**
- Source directory must contain `Default/` subdirectory or `Preferences` file
- Profile name must be unique
- Source must be readable

---

## Complete Workflow Examples

### Example 1: Create and Use Profile

```bash
# 1. Create profile
PROF=$(curl -s -X POST http://localhost:9867/profiles \
  -H "Content-Type: application/json" \
  -d '{"name":"my-work"}' | jq -r .name)

echo "Created profile: $PROF"

# 2. Get profile info
curl -s http://localhost:9867/profiles/$PROF | jq .

# 3. Update profile metadata
curl -s -X PATCH http://localhost:9867/profiles/$PROF \
  -H "Content-Type: application/json" \
  -d '{"description":"Work account profile"}'

# 4. View profile logs (after use)
curl -s http://localhost:9867/profiles/$PROF/logs | jq length

# 5. Reset before archiving
curl -s -X POST http://localhost:9867/profiles/$PROF/reset

# 6. View final analytics
curl -s http://localhost:9867/profiles/$PROF/analytics | jq .totalActions
```

### Example 2: Import Chrome Profile

```bash
# Find your Chrome profile path
CHROME_PATH="/Users/luigi/Library/Application Support/Google/Chrome/Default"

# Import it
curl -X POST http://localhost:9867/profiles/import \
  -H "Content-Type: application/json" \
  -d "{
    \"name\": \"imported-work\",
    \"sourcePath\": \"$CHROME_PATH\",
    \"description\": \"Imported from my Chrome\",
    \"useWhen\": \"Production accounts\"
  }"

# Verify
curl -s http://localhost:9867/profiles/imported-work | jq '{id, name, source}'
```

### Example 3: Manage Multiple Profiles

```bash
#!/bin/bash
# Create profiles for different use cases

for profile in "testing" "staging" "production"; do
  echo "Creating profile: $profile"
  curl -s -X POST http://localhost:9867/profiles \
    -H "Content-Type: application/json" \
    -d "{
      \"name\": \"$profile\",
      \"useWhen\": \"$profile environment\"
    }"
done

# List all
echo -e "\nAll profiles:"
curl -s http://localhost:9867/profiles | jq '.[] | {name, useWhen}'

# Get stats for each
echo -e "\nProfile statistics:"
for profile in "testing" "staging" "production"; do
  echo -n "$profile: "
  curl -s http://localhost:9867/profiles/$profile/analytics | jq .totalActions
done
```

### Example 4: Cleanup Script

```bash
#!/bin/bash
# List temporary profiles and delete them

echo "Temporary profiles:"
TEMP_PROFILES=$(curl -s 'http://localhost:9867/profiles?all=true' | \
  jq -r '.[] | select(.temporary == true) | .name')

if [ -z "$TEMP_PROFILES" ]; then
  echo "No temporary profiles found"
  exit 0
fi

echo "$TEMP_PROFILES"

read -p "Delete these profiles? (y/N) " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
  for prof in $TEMP_PROFILES; do
    echo "Deleting: $prof"
    curl -s -X DELETE http://localhost:9867/profiles/$prof
  done
fi
```

---

## Error Handling

### Common Errors

**Profile Not Found (404):**
```bash
curl http://localhost:9867/profiles/nonexistent

# Response
{
  "error": "profile \"nonexistent\" not found",
  "code": "ERR_PROFILE_NOT_FOUND",
  "statusCode": 404
}
```

**Profile Already Exists (400):**
```bash
curl -X POST http://localhost:9867/profiles \
  -d '{"name":"existing-profile"}'

# Response
{
  "error": "profile \"existing-profile\" already exists",
  "statusCode": 400
}
```

**Invalid Source Path (400):**
```bash
curl -X POST http://localhost:9867/profiles/import \
  -d '{"name":"imp","sourcePath":"/invalid/path"}'

# Response
{
  "error": "source path invalid: stat /invalid/path: no such file or directory",
  "statusCode": 400
}
```

**Missing Required Field (400):**
```bash
curl -X POST http://localhost:9867/profiles \
  -d '{}'

# Response
{
  "error": "name required",
  "statusCode": 400
}
```

---

## Best Practices

### Profile Naming

✅ **Good:**
- `work-email`
- `github-scraper`
- `testing-v2`
- `production-accounts`

❌ **Avoid:**
- Names that are too generic: `test`, `temp`, `profile`
- Very long names (>50 chars)
- Special characters that need URL encoding (use hyphens instead of spaces)

### Using Descriptions and UseWhen

```bash
# Include metadata for team collaboration
curl -X POST http://localhost:9867/profiles \
  -d '{
    "name": "github-automation",
    "description": "For automated GitHub operations",
    "useWhen": "When scheduled workflows need to access GitHub repos"
  }'
```

This helps:
- Document profile purpose
- Assist AI agents in selecting right profile
- Make maintenance easier

### Profile Isolation

Each profile is completely isolated:
- Separate cookies and sessions
- Independent browser cache
- Isolated browser storage
- No data sharing between profiles

Use different profiles for:
- Different accounts
- Different environments (dev/staging/prod)
- Different projects
- Security isolation

### Cleanup Strategy

```bash
# Reset before switching accounts
curl -X POST http://localhost:9867/profiles/my-profile/reset

# Delete old profiles periodically
curl -X DELETE http://localhost:9867/profiles/old-profile
```

---

## Integration Examples

### With Bash

```bash
# Create profile, get ID, use it
PROF_ID=$(curl -s -X POST http://localhost:9867/profiles \
  -d '{"name":"test"}' | jq -r .name)

# Use the profile in instance
INST=$(curl -s -X POST http://localhost:9867/instances/start \
  -d "{\"profile\":\"$PROF_ID\",\"mode\":\"headed\"}" | jq -r .id)

# Cleanup
curl -s -X DELETE http://localhost:9867/profiles/$PROF_ID
```

### With Python

```python
import requests
import json

BASE = "http://localhost:9867"

# List profiles
profiles = requests.get(f"{BASE}/profiles").json()
print(f"Found {len(profiles)} profiles")

# Create profile
resp = requests.post(f"{BASE}/profiles", json={
    "name": "python-test",
    "description": "Created from Python"
})
print(f"Created: {resp.json()}")

# Update profile
requests.patch(f"{BASE}/profiles/python-test", json={
    "useWhen": "Python integration testing"
})

# Get analytics
analytics = requests.get(f"{BASE}/profiles/python-test/analytics").json()
print(f"Total actions: {analytics.get('totalActions', 0)}")

# Delete profile
requests.delete(f"{BASE}/profiles/python-test")
```

### With JavaScript/Node.js

```javascript
const BASE = "http://localhost:9867";

// List profiles
async function listProfiles() {
  const resp = await fetch(`${BASE}/profiles`);
  return resp.json();
}

// Create profile
async function createProfile(name) {
  const resp = await fetch(`${BASE}/profiles`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name })
  });
  return resp.json();
}

// Delete profile
async function deleteProfile(id) {
  const resp = await fetch(`${BASE}/profiles/${id}`, {
    method: "DELETE"
  });
  return resp.json();
}

// Usage
(async () => {
  const prof = await createProfile("js-test");
  console.log("Created:", prof);

  const profiles = await listProfiles();
  console.log(`Total profiles: ${profiles.length}`);

  await deleteProfile("js-test");
  console.log("Deleted");
})();
```

---

## Status Codes

| Code | Meaning | Example |
|------|---------|---------|
| **200** | Success (GET, PATCH, POST reset) | Profile retrieved, metadata updated |
| **201** | Created | Profile created (for POST /profiles) |
| **204** | No content | Profile deleted successfully |
| **400** | Bad request | Invalid JSON, missing name, duplicate profile |
| **404** | Not found | Profile doesn't exist |
| **500** | Server error | Internal error |

---

## FAQ

**Q: Can I rename a profile?**
A: Not directly. Create a new profile with the desired name and delete the old one.

**Q: Can I move profiles to different machines?**
A: Yes, use POST /profiles/import with the profile directory from another machine.

**Q: How much disk space does a profile use?**
A: Varies, but typically 100MB-1GB per profile. Check `diskUsage` in profile info.

**Q: Can I share profiles between users?**
A: Not recommended. Create separate profiles for isolation and security.

**Q: What happens when I reset a profile?**
A: Cache, cookies, history, and sessions are cleared. You stay logged out.

**Q: Can I recover a deleted profile?**
A: No, deletion is permanent. Always backup important profiles first.

**Q: How do I export a profile?**
A: Copy the profile directory from `~/.pinchtab/profiles/<name>/`.

---

## Summary Table

| Operation | Method | Endpoint | Requires |
|-----------|--------|----------|----------|
| List | GET | `/profiles` | None |
| Get | GET | `/profiles/{id}` | ID or name |
| Create | POST | `/profiles` | Name |
| Update | PATCH | `/profiles/{id}` | ID or name |
| Delete | DELETE | `/profiles/{id}` | ID or name |
| Reset | POST | `/profiles/{id}/reset` | ID or name |
| Logs | GET | `/profiles/{id}/logs` | ID or name |
| Analytics | GET | `/profiles/{id}/analytics` | ID or name |
| Import | POST | `/profiles/import` | Name + source path |
