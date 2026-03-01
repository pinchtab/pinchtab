# Dashboard

PinchTab includes a built-in web dashboard for monitoring and managing instances, profiles, and agent activity.

Access the dashboard at **`http://localhost:9867`** (adjust port if needed).

Alternatively, use **`http://localhost:9867/dashboard`** (also works for backward compatibility).

---

## Dashboard Overview

The dashboard provides four main screens:

1. **Instances** — View and manage running Chrome instances
2. **Profiles** — Browse and launch saved browser profiles
3. **Profile Details** — Configure and launch a specific profile
4. **Agents Feed** — Monitor agent activity and automation workflows

---

## Instances Screen

> **Screenshot placeholder:** Instances view

### What It Shows

- List of all running PinchTab instances
- Port number for each instance
- Instance status (running, stopped, idle)
- Number of tabs in each instance
- Profile name (if any) for each instance

### What You Can Do

- **View details** — Click an instance to see full information
- **Create new instance** — Start a new Chrome process
- **Stop instance** — Shut down a running instance
- **View tabs** — See all tabs open in the instance

### Use Cases

- Monitor multiple instances running in parallel
- Check resource usage per instance
- Stop instances when no longer needed
- Debug instance configuration

---

## Profiles Screen

> **Screenshot placeholder:** Profiles view

### What It Shows

- Grid of all available browser profiles
- Each profile card displays:
  - Profile name
  - Associated email/account (if available)
  - Last used timestamp
  - Current status (running, stopped)
  - Quick info (cookies count, stored data size)

### What You Can Do

- **Launch profile** — Start a new instance with this profile
- **View details** — Click profile card to see configuration details
- **Edit profile** — Modify profile settings (name, metadata)
- **Delete profile** — Remove a profile (with confirmation)
- **Search/filter** — Find profiles by name or account

### Use Cases

- Quickly launch a profile for a specific user account
- Switch between different login contexts
- See which profiles are currently active
- Manage multiple user sessions

---

## Profile Details Screen

> **Screenshot placeholder:** Profile details view

### What It Shows

- **Profile name** — Editable identifier
- **Account info** — Associated email, username, or account ID
- **Launch settings**:
  - Headless or headed mode
  - Port assignment
  - Stealth level (light, medium, full)
  - Environment variables
- **State info**:
  - Created date
  - Last modified
  - Data size (cookies, storage, cache)
  - Number of saved tabs
- **Instances using this profile** — Currently running instances

### What You Can Do

- **Launch** — Start a new instance with this profile
- **Edit** — Modify profile configuration
- **View data** — See stored cookies, local storage, browsing history
- **Clear data** — Reset cookies/cache while keeping profile
- **Export** — Backup profile configuration
- **Delete** — Remove profile entirely

### Use Cases

- Configure launch options before starting an instance
- Check what data is stored in a profile
- Clone a profile for a similar use case
- Debug profile-related issues

---

## Agents Feed Screen

> **Screenshot placeholder:** Agents feed view

### What It Shows

- Real-time activity log from all connected agents
- Each entry displays:
  - Timestamp
  - Agent name/ID
  - Action performed (navigated, clicked, extracted text, etc.)
  - Associated instance/profile
  - Result (success, error, pending)

### What You Can Do

- **Monitor agents** — Watch what automation is happening in real-time
- **Filter by agent** — Show only activity from a specific agent
- **Filter by instance** — Show only activity in a specific instance
- **Search** — Find activities by action, URL, or data
- **View details** — Click an entry to see full request/response
- **Pause/resume** — Control logging verbosity

### Use Cases

- Debug agent automation workflows
- Audit what agents have done
- Monitor for errors or unexpected behavior
- Understand which agents are most active
- Troubleshoot automation issues

---

## Navigation

The dashboard header provides tabs to switch between screens:

```text
[Instances] | [Profiles] | [Profile Details] | [Agents]
```

You can also navigate by clicking:
- An instance → shows its details
- A profile → shows its profile details screen
- An agent event → shows relevant instance/profile

---

## Status Indicators

### Instance Status
- **Running** (green) — Active Chrome process
- **Idle** (yellow) — Running but no tabs
- **Stopped** (red) — Process not running

### Profile Status
- **Active** (green) — At least one instance using it
- **Dormant** (gray) — No active instances
- **Launching** (blue) — Instance starting

### Agent Status
- **Success** (green) — Action completed
- **Error** (red) — Action failed
- **Pending** (yellow) — In progress
- **Cancelled** (gray) — Aborted

---

## Keyboard Shortcuts

| Shortcut | Action |
|---|---|
| `R` | Refresh current screen |
| `Esc` | Go back / Close modal |
| `Ctrl+K` | Search |
| `Ctrl+1` | Instances tab |
| `Ctrl+2` | Profiles tab |
| `Ctrl+3` | Profile Details tab |
| `Ctrl+4` | Agents Feed tab |

---

## Dark Mode

The dashboard automatically uses your system's dark/light preference.

Toggle manually:
- Click the theme toggle in the top-right corner (sun/moon icon)
- Preference is saved in browser local storage

---

## Performance & Limits

- **Refresh rate**: Real-time updates (WebSocket-based)
- **History retention**: Last 1000 agent events (older events archived)
- **Scalability**: Optimized for 10+ instances, 100+ profiles

For high-throughput monitoring, consider using the REST API directly:

```bash
# Get all instances
curl http://localhost:9867/instances

# Get all profiles
curl http://localhost:9867/profiles

# Stream agent events
curl http://localhost:9867/events/stream
```

---

## Troubleshooting

### Dashboard Not Loading

- Check if PinchTab is running: `curl http://localhost:9867/health`
- Check the port: Default is `9867`, adjust if you started with `--port`
- Clear browser cache: `Ctrl+Shift+Delete` (most browsers)

### No Instances Showing

- Make sure at least one instance is running: `pinchtab --port 9867`
- Refresh the page (`R` key)
- Check browser console for errors (`F12`)

### Agent Events Not Updating

- Confirm agents are actually running tasks
- Check WebSocket connection: Open DevTools → Network → WS tab
- Try refreshing the page

---

## Next Steps

- [Core Concepts](core-concepts.md) — Understand instances, profiles, tabs
- [Get Started](get-started.md) — Set up your first profile
- [Headless vs Headed](headless-vs-headed.md) — Choose the right mode
