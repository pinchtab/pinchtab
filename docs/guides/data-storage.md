# Data Storage Guide

PinchTab stores configuration, profiles, session state, and usage logs on local disk. This guide describes what is stored, where it lives by default, and which paths you can change.

## What PinchTab Stores

| Path | Purpose | How To Change It |
| --- | --- | --- |
| `config.json` | Main PinchTab configuration | `PINCHTAB_CONFIG` selects the file |
| `profiles/<profile>/` | Chrome user data for each profile | `profiles.baseDir` |
| `action_logs.json` | Profile activity log used by profile analytics | not currently configurable |
| `sessions.json` | Saved tab/session state for a bridge instance | `server.stateDir` |
| `activity/events-YYYY-MM-DD.jsonl` | Daily request/activity logs for `/api/activity`, CLI activity, and dashboard activity views | `server.stateDir`, `observability.activity.retentionDays` |
| `<profile>/.pinchtab-state/config.json` | Child instance config written by the orchestrator | generated automatically for managed instances |

## Default Storage Location

PinchTab uses the OS config directory:

| OS | Default Base Directory |
| --- | --- |
| Linux | `~/.config/pinchtab/` or `$XDG_CONFIG_HOME/pinchtab/` |
| macOS | `~/Library/Application Support/pinchtab/` |
| Windows | `%APPDATA%\\pinchtab\\` |

Typical layout:

```text
pinchtab/
├── config.json
├── action_logs.json
├── activity/
│   └── events-2026-03-16.jsonl
├── sessions.json
└── profiles/
    └── default/
```

## Legacy Fallback

For backward compatibility, PinchTab still uses `~/.pinchtab/` if:

- that legacy directory already exists
- and the newer OS-native location does not exist yet

That fallback applies to both config lookup and the default base storage directory.

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

By default PinchTab keeps 1 day of activity data and prunes older daily files when new activity is recorded. You can change that with:

```json
{
  "observability": {
    "activity": {
      "retentionDays": 1,
      "sessionIdleSec": 1800
    }
  }
}
```

`retentionDays` controls on-disk retention for activity logs. `sessionIdleSec` controls session grouping only.

In orchestrator mode, child instances get their own state directory under the profile:

```text
<profile>/.pinchtab-state/
```

PinchTab writes a child `config.json` there so the launched instance can inherit the correct profile path, state directory, and port.

## Action Logs

PinchTab also stores profile-level analytics data in:

```text
<user-config-dir>/action_logs.json
```

This powers profile analytics endpoints. It is separate from the request/activity JSONL files and from per-instance session restore state.

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
- action logs
- local configuration

Back up the profile directories first if you need to preserve logged-in browser sessions.
