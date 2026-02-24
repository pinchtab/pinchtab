# Pinchtab Dashboard Test Plan

**Goal:** Validate the dashboard mode — profile management, orchestrator (instance lifecycle), proxy routing, SSE events, and UI serving.

**Constraint:** Tests MUST NOT delete or modify existing profiles. Use a dedicated test profile (`__test_profile__`) for all mutations.

---

## 1. Health & Mode

| # | Scenario | Steps | Expected | Auto |
|---|----------|-------|----------|------|
| DH1 | Dashboard health | `GET /health` | 200, `{"status":"ok","mode":"dashboard"}` | ✅ |
| DH2 | Dashboard UI serves | `GET /dashboard` | 200, HTML with `<html` | ✅ |
| DH3 | Static assets serve | `GET /dashboard/app.js` | 200, JavaScript content | ✅ |
| DH4 | Static CSS serves | `GET /dashboard/base.css` | 200, CSS content | ✅ |

---

## 2. Profile Management (CRUD)

All tests use `__test_profile__` — cleaned up at end.

| # | Scenario | Steps | Expected | Auto |
|---|----------|-------|----------|------|
| DP1 | List profiles | `GET /profiles` | 200, array of profiles (existing ones present) | ✅ |
| DP2 | Create profile | `POST /profiles/create {"name":"__test_profile__"}` | 201, created | ✅ |
| DP3 | Create duplicate | `POST /profiles/create {"name":"__test_profile__"}` | 409, conflict | ✅ |
| DP4 | Create missing name | `POST /profiles/create {}` | 400, name required | ✅ |
| DP5 | Create bad JSON | `POST /profiles/create {broken` | 400, invalid JSON | ✅ |
| DP6 | Create with metadata | `POST /profiles/create {"name":"__test_profile_meta__","useWhen":"testing","description":"test profile"}` | 201, created with meta | ✅ |
| DP7 | Update metadata | `PATCH /profiles/__test_profile__ {"useWhen":"updated","description":"updated desc"}` | 200, updated info | ✅ |
| DP8 | Rename profile | `PATCH /profiles/__test_profile__ {"name":"__test_profile_renamed__"}` | 200, renamed | ✅ |
| DP9 | Reset profile | `POST /profiles/__test_profile_renamed__/reset` | 200, reset | ✅ |
| DP10 | Delete profile | `DELETE /profiles/__test_profile_renamed__` | 200, deleted | ✅ |
| DP11 | Delete nonexistent | `DELETE /profiles/__nonexistent__` | 404 | ✅ |
| DP12 | Reset nonexistent | `POST /profiles/__nonexistent__/reset` | 404 | ✅ |
| DP13 | Profile logs (empty) | `GET /profiles/__test_profile_meta__/logs` | 200, empty array | ✅ |
| DP14 | Profile analytics | `GET /profiles/__test_profile_meta__/analytics` | 200, analytics object | ✅ |
| DP15 | Cleanup meta profile | `DELETE /profiles/__test_profile_meta__` | 200, deleted | ✅ |

---

## 3. Orchestrator — Instance Lifecycle

| # | Scenario | Steps | Expected | Auto |
|---|----------|-------|----------|------|
| DO1 | List instances (initial) | `GET /instances` | 200, array (may be empty or have running instances) | ✅ |
| DO2 | Launch instance | `POST /instances/launch {"name":"__test_profile__","port":"<free>"}` | 201, instance with id/status | ✅ |
| DO3 | Launch missing fields | `POST /instances/launch {"name":""}` | 400, name and port required | ✅ |
| DO4 | Launch bad JSON | `POST /instances/launch {broken` | 400 | ✅ |
| DO5 | Instance appears in list | `GET /instances` | Array includes __test_profile__ instance | ✅ |
| DO6 | Profile instance status | `GET /profiles/__test_profile__/instance` | 200, running=true, port matches | ✅ |
| DO7 | Instance logs | `GET /instances/{id}/logs` | 200, text content | ✅ |
| DO8 | All tabs across instances | `GET /instances/tabs` | 200, array | ✅ |
| DO9 | Stop instance | `POST /instances/{id}/stop` | 200, stopped | ✅ |
| DO10 | Stop nonexistent | `POST /instances/nonexistent/stop` | 404 | ✅ |
| DO11 | Profile instance after stop | `GET /profiles/__test_profile__/instance` | 200, running=false | ✅ |
| DO12 | Launch duplicate port | Launch two profiles on same port | 409, conflict | ✅ |
| DO13 | Stop by profile name | `POST /profiles/__test_profile__/stop` (when running) | 200, stopped | ✅ |
| DO14 | Start by profile ID | `POST /start/{profileId}` | 201, auto-allocated port | ✅ |
| DO15 | Stop by profile ID | `POST /stop/{profileId}` | 200, stopped | ✅ |

---

## 4. Proxy Routing (requires running instance)

| # | Scenario | Steps | Expected | Auto |
|---|----------|-------|----------|------|
| DX1 | Proxy with no instances | Stop all test instances, `GET /snapshot` | 503, "no running instances" | ✅ |
| DX2 | Proxy navigate | Launch instance, `POST /navigate {"url":"https://example.com"}` via dashboard port | 200, proxied to instance | ✅ |
| DX3 | Proxy snapshot | `GET /snapshot` via dashboard port | 200, valid JSON with nodes | ✅ |
| DX4 | Proxy tabs | `GET /tabs` via dashboard port | 200, array | ✅ |
| DX5 | Proxy screenshot | `GET /screenshot` via dashboard port | 200, image data | ✅ |
| DX6 | Proxy evaluate | `POST /evaluate {"expression":"1+1"}` via dashboard | 200, result | ✅ |
| DX7 | Proxy cookies | `GET /cookies` via dashboard | 200, array | ✅ |
| DX8 | Proxy stealth | `GET /stealth/status` via dashboard | 200, stealth info | ✅ |

---

## 5. SSE Events

| # | Scenario | Steps | Expected | Auto |
|---|----------|-------|----------|------|
| DS1 | SSE connect | `GET /dashboard/events` with Accept: text/event-stream | 200, receives `event: init` with agent data | ⚠️ |
| DS2 | SSE keepalive | Hold connection 35s | Receives `: keepalive` comment | 🔧 Manual |

---

## 6. Dashboard Agents API

| # | Scenario | Steps | Expected | Auto |
|---|----------|-------|----------|------|
| DA1 | List agents | `GET /dashboard/agents` | 200, array of agent objects | ✅ |

---

## 7. Edge Cases

| # | Scenario | Steps | Expected | Auto |
|---|----------|-------|----------|------|
| DE1 | Shutdown endpoint | `POST /shutdown` | 200, dashboard shuts down | 🔧 Manual |
| DE2 | CORS headers | `OPTIONS /health` | CORS headers present | ✅ |
| DE3 | Port already in use | Launch on occupied port | 409 or error | ✅ |
| DE4 | Rapid launch/stop | Launch → stop → launch same profile | Both succeed, no crash | ✅ |
| DE5 | Screencast proxy info | `GET /instances/{id}/proxy/screencast?tabId=test` | 200, wsUrl | ✅ |

---

## 8. UI Functional Tests (Manual / Browser)

| # | Scenario | Steps | Expected |
|---|----------|-------|----------|
| DU1 | Dashboard loads | Open `http://localhost:9867/dashboard` in browser | Page renders, profiles visible |
| DU2 | Profile list shows | View Profiles tab | Existing profiles listed with status |
| DU3 | Create profile via UI | Click create, enter name | Profile appears in list |
| DU4 | Launch profile via UI | Click launch/start on a profile | Instance starts, status updates to running |
| DU5 | Stop profile via UI | Click stop on running profile | Instance stops, status updates |
| DU6 | Screencast tab | Switch to screencast tab while instance running | Live view of browser |
| DU7 | Agents tab | Switch to agents tab | Shows connected agents / activity |
| DU8 | Settings tab | Switch to settings tab | Displays configuration |
| DU9 | Profile analytics via UI | Click analytics on a profile with history | Shows charts/stats |

---

## 9. Endpoint Existence Checks

Verify every registered route returns a non-404 status (may return 400/503 for missing params, but not 404 routing failures).

### Dashboard / Health

| # | Route | Method | Expected | Auto |
|---|-------|--------|----------|------|
| RE1 | `/health` | GET | 200 | ✅ |
| RE2 | `/dashboard` | GET | 200 (HTML) | ✅ |
| RE3 | `/dashboard/agents` | GET | 200 | ✅ |
| RE4 | `/dashboard/events` | GET | 200 (SSE stream) | ✅ |
| RE5 | `/shutdown` | POST | 200 (⚠️ kills dashboard) | 🔧 Manual |

### Profiles

| # | Route | Method | Expected | Auto |
|---|-------|--------|----------|------|
| RE6 | `/profiles` | GET | 200 | ✅ |
| RE7 | `/profiles/create` | POST | 400 (no body) | ✅ |
| RE8 | `/profiles/import` | POST | 400 (no body) | ✅ |
| RE9 | `/profiles/meta` | PATCH | 400 (no body) | ✅ |
| RE10 | `/profiles/__nonexistent__` | DELETE | 404 | ✅ |
| RE11 | `/profiles/__nonexistent__` | PATCH | 400 (no body) | ✅ |
| RE12 | `/profiles/__nonexistent__/reset` | POST | 404 | ✅ |
| RE13 | `/profiles/__nonexistent__/logs` | GET | 200 (empty) | ✅ |
| RE14 | `/profiles/__nonexistent__/analytics` | GET | 200 | ✅ |
| RE15 | `/profiles/__nonexistent__/instance` | GET | 200 (running=false) | ✅ |
| RE16 | `/profiles/__nonexistent__/stop` | POST | 404 | ✅ |

### Orchestrator

| # | Route | Method | Expected | Auto |
|---|-------|--------|----------|------|
| RE17 | `/instances` | GET | 200 | ✅ |
| RE18 | `/instances/tabs` | GET | 200 | ✅ |
| RE19 | `/instances/launch` | POST | 400 (no body) | ✅ |
| RE20 | `/instances/nonexistent/stop` | POST | 404 | ✅ |
| RE21 | `/instances/nonexistent/logs` | GET | 404 | ✅ |
| RE22 | `/instances/nonexistent/proxy/screencast?tabId=x` | GET | 404 | ✅ |

### Profile lifecycle (by ID — canonical for agents)

| # | Route | Method | Expected | Auto |
|---|-------|--------|----------|------|
| RE23 | `/profiles/{id}/start` | POST | 201 (launches instance) | ✅ |
| RE24 | `/profiles/{id}/stop` | POST | 200 (stops instance) | ✅ |
| RE25a | `/profiles/{id}/instance` | GET | 200 (instance status) | ✅ |
| RE25b | `/profiles/{unknownId}/start` | POST | 404 | ✅ |

### Short aliases (agent convenience)

| # | Route | Method | Expected | Auto |
|---|-------|--------|----------|------|
| RE25c | `/start/{id}` | POST | 201 (same as /profiles/{id}/start) | ✅ |
| RE25d | `/stop/{id}` | POST | 200 (same as /profiles/{id}/stop) | ✅ |

### Proxy endpoints (503 when no instance running)

| # | Route | Method | Expected (no instance) | Auto |
|---|-------|--------|------------------------|------|
| RE25 | `/tabs` | GET | 503 | ✅ |
| RE26 | `/snapshot` | GET | 503 | ✅ |
| RE27 | `/screenshot` | GET | 503 | ✅ |
| RE28 | `/text` | GET | 503 | ✅ |
| RE29 | `/navigate` | POST | 503 | ✅ |
| RE30 | `/action` | POST | 503 | ✅ |
| RE31 | `/actions` | POST | 503 | ✅ |
| RE32 | `/evaluate` | POST | 503 | ✅ |
| RE33 | `/tab` | POST | 503 | ✅ |
| RE34 | `/tab/lock` | POST | 503 | ✅ |
| RE35 | `/tab/unlock` | POST | 503 | ✅ |
| RE36 | `/cookies` | GET | 503 | ✅ |
| RE37 | `/cookies` | POST | 503 | ✅ |
| RE38 | `/stealth/status` | GET | 503 | ✅ |
| RE39 | `/fingerprint/rotate` | POST | 503 | ✅ |
| RE40 | `/screencast` | GET | 503 | ✅ |
| RE41 | `/screencast/tabs` | GET | 503 | ✅ |

---

## Release Criteria

### Must Pass
- All Section 1 (health/UI serving)
- All Section 2 (profile CRUD) — no side effects on existing profiles
- Section 3 DO1-DO11 (basic instance lifecycle)
- Section 4 DX1-DX4 (proxy basics)
- Section 9 RE1-RE41 (all endpoints reachable)

### Should Pass
- Section 3 DO12-DO15 (advanced orchestration)
- Section 4 DX5-DX8 (all proxy endpoints)
- Section 6 (agents API)

### Nice to Have
- Section 5 (SSE)
- Section 7 (edge cases)
- Section 8 (UI manual)
