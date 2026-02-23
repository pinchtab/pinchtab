# Profile Management (Dashboard Mode)

When running `pinchtab dashboard`, profiles are managed via the dashboard API on port 9867.

## List profiles

```bash
curl http://localhost:9867/profiles
```

Returns array of profiles with `id`, `name`, `accountEmail`, `useWhen`, etc.

## Start a profile

```bash
# Auto-allocate port (recommended)
curl -X POST http://localhost:9867/profiles/<ID>/start

# With specific port and headless mode
curl -X POST http://localhost:9867/profiles/<ID>/start \
  -H 'Content-Type: application/json' \
  -d '{"port": "9868", "headless": true}'

# Short alias
curl -X POST http://localhost:9867/start/<ID>
```

Returns instance info including allocated `port`. Use that port for all subsequent API calls.

## Stop a profile

```bash
curl -X POST http://localhost:9867/profiles/<ID>/stop

# Short alias
curl -X POST http://localhost:9867/stop/<ID>
```

## Check instance status

```bash
# By profile ID (recommended)
curl http://localhost:9867/profiles/<ID>/instance

# By profile name
curl http://localhost:9867/profiles/My%20Profile/instance
```

## Launch by name

```bash
curl -X POST http://localhost:9867/instances/launch \
  -H 'Content-Type: application/json' \
  -d '{"name": "work", "port": "9868"}'
```

## Typical agent flow

```bash
# 1. List profiles
PROFILES=$(curl -s http://localhost:9867/profiles)

# 2. Start profile (auto-allocates port)
INSTANCE=$(curl -s -X POST http://localhost:9867/profiles/$PROFILE_ID/start)
PORT=$(echo $INSTANCE | jq -r .port)

# 3. Use the instance
curl -X POST http://localhost:$PORT/navigate -H 'Content-Type: application/json' \
  -d '{"url": "https://mail.google.com"}'
curl http://localhost:$PORT/snapshot?maxTokens=4000

# 4. Stop when done
curl -s -X POST http://localhost:9867/profiles/$PROFILE_ID/stop
```

## Profile IDs

Each profile gets a stable 12-char hex ID (SHA-256 of name, truncated) stored in `profile.json`. IDs are URL-safe and never change — use them instead of names in automation.

## Headed mode

Headed mode = real visible Chrome window managed by Pinchtab.

- Human can log in, pass 2FA/captcha, validate state
- Agent calls HTTP APIs against the same running instance
- Session state persists in profile directory (cookies/storage carry over)

Recommended human + agent flow:

```bash
# Human starts dashboard and sets up profile
pinchtab dashboard

# Agent resolves the profile endpoint
PINCHTAB_BASE_URL="$(pinchtab connect <profile-name>)"
curl "$PINCHTAB_BASE_URL/health"
```
