# Data Storage Guide

PinchTab stores configuration, profiles, session state, and usage logs on local disk. This guide describes what is stored, where it lives by default, and which paths you can change.

## What PinchTab Stores

| Path | Purpose | How To Change It |
| --- | --- | --- |
| `config.json` | Main PinchTab configuration | `PINCHTAB_CONFIG` selects the file |
| `profiles/<profile>/` | Chrome user data for each profile | `profiles.baseDir` |
| `sessions.json` | Saved tab/session state for a bridge instance | `server.stateDir` |
| `activity/events-YYYY-MM-DD.jsonl` | Primary daily request/activity log for `/api/activity`, CLI activity, and dashboard activity views | `server.stateDir`, `observability.activity.retentionDays` |
| `activity/events-<source>-YYYY-MM-DD.jsonl` | Source-specific daily activity log for named sources such as `dashboard` or `orchestrator` | `server.stateDir`, `observability.activity.retentionDays` |
| `<profile>/.pinchtab-state/config.json` | Child instance config written by the orchestrator | generated automatically for managed instances |

## Default Storage Location

PinchTab uses the OS config directory:

| OS | Default Base Directory |
| --- | --- |
| Linux | `~/.pinchtab/` |
| macOS | `~/.pinchtab/` |
| Windows | `%APPDATA%\\pinchtab\\` |

Typical layout:

```text
pinchtab/
├── config.json
├── activity/
│   └── events-2026-03-16.jsonl
├── sessions.json
└── profiles/
    └── default/
```

## Platform Defaults

On macOS and Linux, `~/.pinchtab/` is the default base directory.

On Windows, PinchTab uses the OS-native config directory under `%APPDATA%\\pinchtab\\`.

## Profiles

Profiles are the durable browser state PinchTab reuses across launches. A profile directory can contain:

- cookies and login sessions
- local storage and IndexedDB
- cache and history
- Chrome preferences and session files

Configure the profile root with:

```json
{
  "profiles": {
    "baseDir": "/path/to/profiles",
    "defaultProfile": "default"
  }
}
```

`profiles.defaultProfile` controls the default profile name used by single-instance flows. In orchestrator mode, managed instances can still launch with other profile names.

## Config File

The main config file is read from:

- the path in `PINCHTAB_CONFIG`, if set
- otherwise `<user-config-dir>/config.json`

Example:

```json
{
  "server": {
    "port": "9867",
    "stateDir": "/var/lib/pinchtab/state"
  },
  "profiles": {
    "baseDir": "/var/lib/pinchtab/profiles",
    "defaultProfile": "default"
  }
}
```

## Session State

Bridge session restore data is stored as:

```text
<server.stateDir>/sessions.json
```

This file is used for tab/session restoration when restore behavior is enabled.

## Activity Logs

Request activity is stored as one JSONL file per UTC day:

```text
<server.stateDir>/activity/events-YYYY-MM-DD.jsonl
```

Named sources also get their own daily files:

```text
<server.stateDir>/activity/events-<source>-YYYY-MM-DD.jsonl
```

By default PinchTab keeps 1 day of activity data and prunes older daily files when new activity is recorded. You can change that with:

```json
{
  "observability": {
    "activity": {
      "retentionDays": 1,
      "sessionIdleSec": 1800,
      "events": {
        "dashboard": false,
        "server": false,
        "bridge": false,
        "orchestrator": false,
        "scheduler": false,
        "mcp": false,
        "other": false
      }
    }
  }
}
```

`retentionDays` controls on-disk retention for activity logs. `sessionIdleSec` controls session grouping only.
`events` controls which non-client sources are recorded. Client events are always recorded.

Requests that carry `X-Agent-Id` are stored with that value as `agentId` in the activity event. This is what powers agent-scoped queries such as `GET /api/activity?agentId=<id>` and the dashboard Agents view.

Unfiltered `GET /api/activity` reads the primary feed. Source-specific logs remain queryable by passing `source=<name>`.

In orchestrator mode, child instances get their own state directory under the profile:

```text
<profile>/.pinchtab-state/
```

PinchTab writes a child `config.json` there so the launched instance can inherit the correct profile path, state directory, and port.

Managed child bridges disable their local activity recorder. Dashboard-visible activity comes from the parent server handling client traffic, so orchestrator-managed child state directories should not accumulate their own `activity/events-*.jsonl` files for new runs.

Profile `logs` and `analytics` endpoints are derived from the activity store rather than a separate analytics file.

## Customizing Storage

### Choose A Different Config File

```bash
export PINCHTAB_CONFIG=/etc/pinchtab/config.json
pinchtab
```

### Choose Different Profile And State Paths

```json
{
  "server": {
    "stateDir": "/srv/pinchtab/state"
  },
  "profiles": {
    "baseDir": "/srv/pinchtab/profiles",
    "defaultProfile": "default"
  }
}
```

## Container Use

For Docker or other containers, persist both config and profile data with a mounted volume and point `PINCHTAB_CONFIG` at a file inside that volume.

Example layout inside the volume:

```text
/data/
├── config.json
├── state/
└── profiles/
```

Then set:

```json
{
  "server": {
    "stateDir": "/data/state"
  },
  "profiles": {
    "baseDir": "/data/profiles"
  }
}
```

## Security Notes

Profile directories often contain sensitive browser state:

- cookies
- session tokens
- cached content
- site data

Recommended practice:

- keep profile directories out of version control
- restrict permissions on config and profile directories
- use separate profiles for separate security contexts

## Cleanup

Removing the PinchTab data directory deletes:

- saved profiles
- session restore data
- local configuration

Back up the profile directories first if you need to preserve logged-in browser sessions.
