# Core Concepts

This document describes the concepts that are implemented today in PinchTab.

## Server

The **server** is the main PinchTab process.

Start it with:

```bash
pinchtab
# or explicitly
pinchtab server
```

What the server does:

- exposes the main HTTP API and dashboard on port `9867` by default
- manages profiles and instances
- proxies tab-scoped requests to the correct managed instance
- can expose shorthand routes such as `/navigate`, `/snapshot`, and `/action`

Important clarification:

- the server is the public entry point
- for managed instances, the server usually does **not** talk to Chrome directly
- instead, it spawns or routes to a per-instance **bridge** process

## Bridge

The **bridge** is the single-instance runtime.

Start it directly only when you want one standalone browser runtime:

```bash
pinchtab bridge
```

What the bridge does:

- owns exactly one Chrome browser process
- exposes browser and tab endpoints such as `/navigate`, `/snapshot`, `/action`, and `/tabs/{id}/...`
- is the process the server launches for each managed instance

In normal multi-instance usage, you usually interact with the server, not with bridge processes directly.

## Profiles

A **profile** is a Chrome user data directory.

It stores persistent browser state such as:

- cookies
- local storage
- cache
- browsing history
- extensions
- saved account state

Profile facts that match the current implementation:

- profiles are persistent on disk
- profiles can exist without any running instance
- at most one active managed instance can use a given profile at a time
- profile IDs use the format `prof_XXXXXXXX`
- `GET /profiles` hides temporary auto-generated profiles unless you pass `?all=true`

Create a profile with the API:

```bash
curl -X POST http://localhost:9867/profiles \
  -H "Content-Type: application/json" \
  -d '{
    "name": "work",
    "description": "Main logged-in work profile"
  }'
# Response
{
  "status": "created",
  "id": "prof_278be873",
  "name": "work"
}
```

## Instances

An **instance** is a managed browser runtime.

In practice, one instance means:

- one bridge process
- one Chrome process
- zero or one profile
- one dedicated port
- many tabs

Instance facts that match the current implementation:

- instance IDs use the format `inst_XXXXXXXX`
- ports are auto-allocated from `9868-9968` by default
- instance status is tracked as `starting`, `running`, `stopping`, `stopped`, or `error`
- one profile cannot be attached to multiple active managed instances at the same time

### Persistent vs temporary instances

There are two common ways to start an instance:

1. with a named profile
2. without a profile ID

If you start an instance with a profile ID, the instance uses that persistent profile.

If you start an instance without a profile ID, PinchTab creates an auto-generated profile named like `instance-...`.
That temporary profile is deleted when the instance stops.

So this is the correct mental model:

- instances without an explicit profile are **ephemeral**
- the implementation still creates a temporary profile directory behind the scenes
- that temporary profile is cleanup state, not a reusable long-term profile

### Starting an instance

Preferred endpoint:

```bash
curl -X POST http://localhost:9867/instances/start \
  -H "Content-Type: application/json" \
  -d '{
    "profileId": "prof_278be873",
    "mode": "headed"
  }'
# CLI Alternative
pinchtab instance start --profile prof_278be873 --mode headed
# Response
{
  "id": "inst_0a89a5bb",
  "profileId": "prof_278be873",
  "profileName": "work",
  "port": "9868",
  "headless": false,
  "status": "starting"
}
```

## Tabs

A **tab** is a single page inside an instance.

Tabs belong to an instance, and therefore inherit that instance's profile state.

What a tab gives you:

- its own URL and page state
- a snapshot of the accessibility tree
- action execution such as click, type, fill, hover, and press
- text extraction, screenshots, PDF export, cookie access, and evaluation

Open a tab in a specific instance:

```bash
INST=inst_0a89a5bb

curl -X POST http://localhost:9867/instances/$INST/tabs/open \
  -H "Content-Type: application/json" \
  -d '{"url":"https://pinchtab.com"}'
# Response
{
  "tabId": "CDP_TARGET_ID"
}
```

Then use tab-scoped endpoints:

```bash
TAB=CDP_TARGET_ID

curl http://localhost:9867/tabs/$TAB/snapshot

curl -X POST http://localhost:9867/tabs/$TAB/action \
  -H "Content-Type: application/json" \
  -d '{"kind":"click","ref":"e5"}'

curl -X POST http://localhost:9867/tabs/$TAB/close
```

### Are tabs persistent?

Usually, no.

For managed instances started by the server:

- tabs are runtime objects
- tabs disappear when the instance stops
- profiles persist, but open tabs do not

That means the persistent part is the **profile state**, not the tab list.

## Element references

Snapshots return element references such as `e0`, `e1`, `e2`, and so on.

These refs are useful because they let you interact with elements without writing CSS selectors for common flows.

## Relationships

The implementation is easiest to understand with these rules:

| Relationship | What is true today |
|---|---|
| Server -> Instances | One server can manage many instances |
| Bridge -> Chrome | One bridge owns one Chrome process |
| Instance -> Profile | An instance has zero or one profile |
| Profile -> Instance | A profile can have zero or one active managed instance at a time |
| Instance -> Tabs | An instance can have many tabs |
| Tab -> Instance | Every tab belongs to exactly one instance |
| Tab -> Profile | A tab inherits the instance profile, if one exists |

Profiles are reusable persistent state. Instances are temporary runtimes that may use a profile.

## Shorthand routes vs explicit routes

PinchTab exposes two styles of interaction:

### Explicit routes

These always name the resource you want:

- `POST /instances/start`
- `POST /instances/{id}/tabs/open`
- `GET /tabs/{id}/snapshot`
- `POST /tabs/{id}/action`

This is the clearest model for multi-instance work.

### Shorthand routes

These omit the instance and sometimes the tab:

- `POST /navigate`
- `GET /snapshot`
- `POST /action`
- `GET /text`

These route to the "current" or first running instance.

## Recommended mental model

For most users, this is the right sequence:

1. start the server with `pinchtab`
2. create a profile if you need persistence
3. start an instance from that profile
4. open one or more tabs in that instance
5. snapshot a tab
6. act on refs from that snapshot

If you do not need persistence:

1. start an instance without `profileId`
2. use it normally
3. stop it when done
4. let PinchTab delete the temporary profile automatically

## Example workflows

### Workflow 1: persistent logged-in browser

```bash
PROFILE_ID=$(curl -s -X POST http://localhost:9867/profiles \
  -H "Content-Type: application/json" \
  -d '{"name":"work"}' | jq -r '.id')

INST=$(curl -s -X POST http://localhost:9867/instances/start \
  -H "Content-Type: application/json" \
  -d "{\"profileId\":\"$PROFILE_ID\",\"mode\":\"headed\"}" | jq -r '.id')

TAB=$(curl -s -X POST http://localhost:9867/instances/$INST/tabs/open \
  -H "Content-Type: application/json" \
  -d '{"url":"https://pinchtab.com/login"}' | jq -r '.tabId')

curl http://localhost:9867/tabs/$TAB/snapshot
```

Use this when you want cookies and account state to survive instance restarts.

### Workflow 2: disposable run

```bash
INST=$(curl -s -X POST http://localhost:9867/instances/start \
  -H "Content-Type: application/json" \
  -d '{"mode":"headless"}' | jq -r '.id')

TAB=$(curl -s -X POST http://localhost:9867/instances/$INST/tabs/open \
  -H "Content-Type: application/json" \
  -d '{"url":"https://example.com"}' | jq -r '.tabId')

curl http://localhost:9867/tabs/$TAB/text

curl -X POST http://localhost:9867/instances/$INST/stop
```

Use this when you want a clean, throwaway session.

## Summary

The durable object in PinchTab is the **profile**.
The runtime object is the **instance**.
The page object is the **tab**.
The **server** manages them, and the **bridge** executes them.
