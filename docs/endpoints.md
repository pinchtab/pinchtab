# Endpoints Reference

## Health & Server

```
GET  /health                             # Server health check (tab count, crash logs)
POST /ensure-chrome                      # Force Chrome initialization
GET  /help                               # List all available endpoints
GET  /openapi.json                       # OpenAPI spec
GET  /metrics                            # Global metrics snapshot
GET  /welcome                            # Welcome HTML page
POST /shutdown                           # Graceful server shutdown
GET  /api/events                         # SSE event stream (dashboard)
```

## Navigation

```
POST /navigate                           # Navigate current tab to URL
GET  /navigate?url=<url>                 # Navigate (GET variant)

POST /tabs/{id}/navigate                 # Navigate a specific tab
```

Body: `{"url": "...", "timeout": 60, "blockImages": true, "newTab": true, "blockAds": true}`

## History & Reload

```
POST /back                               # Go back in current tab
POST /back?tabId=<id>                    # Go back in specific tab
POST /tabs/{id}/back                     # Go back (tab-scoped)
POST /forward                            # Go forward in current tab
POST /forward?tabId=<id>                 # Go forward in specific tab
POST /tabs/{id}/forward                  # Go forward (tab-scoped)
POST /reload                             # Reload current tab
POST /reload?tabId=<id>                  # Reload specific tab
POST /tabs/{id}/reload                   # Reload (tab-scoped)
```

## Tab Management

```
GET  /tabs                               # List all open tabs
POST /tab                                # Tab actions: new, close, focus
POST /tabs/{id}/close                    # Close a specific tab (orchestrator)
GET  /tabs/{id}/metrics                  # Per-tab metrics
```

Actions via `POST /tab`:
- `{"action": "new", "url": "..."}` — open new tab
- `{"action": "close", "tabId": "..."}` — close tab
- `{"action": "focus", "tabId": "..."}` — focus/switch to tab

## Tab Locking (multi-agent)

```
POST /tab/lock                           # Lock a tab {tabId, owner, timeoutSec}
POST /tab/unlock                         # Unlock a tab {tabId, owner}
POST /tabs/{id}/lock                     # Lock specific tab
POST /tabs/{id}/unlock                   # Unlock specific tab
```

## Interaction

```
POST /action                             # Single action on current tab
GET  /action                             # Action (GET variant)
POST /actions                            # Batch actions in sequence
POST /macro                              # Multi-step macro with per-step timeout
POST /tabs/{id}/action                   # Action on specific tab
POST /tabs/{id}/actions                  # Batch actions on specific tab
```

Action kinds: `click`, `dblclick`, `type`, `fill`, `press`, `hover`, `scroll`, `select`, `focus`, `drag`, `check`, `uncheck`

Body: `{"kind": "click", "ref": "e5"}` / `{"kind": "dblclick", "ref": "e5"}` / `{"kind": "type", "ref": "e12", "text": "hello"}` / `{"kind": "check", "selector": "#terms"}` / `{"kind": "uncheck", "ref": "e7"}`

## Page Analysis

```
GET  /snapshot                           # Accessibility tree (current tab)
GET  /tabs/{id}/snapshot                 # Accessibility tree (specific tab)
GET  /text                               # Extract readable text
GET  /tabs/{id}/text                     # Extract text (specific tab)
POST /find                               # Semantic search in page
POST /tabs/{id}/find                     # Semantic search (specific tab)
POST /evaluate                           # Evaluate JavaScript
POST /tabs/{id}/evaluate                 # Evaluate JS (specific tab)
```

Snapshot params: `?filter=interactive`, `?format=compact|text|yaml`, `?depth=5`, `?diff=true`, `?selector=main`, `?maxTokens=2000`, `?noAnimations=true`, `?output=file`

Text params: `?mode=raw`, `?format=text`

## Screenshot & PDF

```
GET  /screenshot                         # Screenshot (current tab)
GET  /tabs/{id}/screenshot               # Screenshot (specific tab)
GET  /pdf                                # PDF export (current tab)
POST /pdf                                # PDF export with options
GET  /tabs/{id}/pdf                      # PDF export (specific tab)
POST /tabs/{id}/pdf                      # PDF export with options (specific tab)
GET  /screencast                         # WebRTC screencast stream
GET  /screencast/tabs                    # All tabs screencast
```

Screenshot params: `?raw=true`, `?quality=80`

PDF params: `?raw=true`, `?landscape=true`, `?scale=0.8`, `?pageRanges=1-5`, `?output=file`, `?path=/tmp/out.pdf`

## Downloads & Uploads

```
GET  /download                           # Download file via browser session
GET  /tabs/{id}/download                 # Download (specific tab)
POST /upload                             # Upload file to input element
POST /tabs/{id}/upload                   # Upload (specific tab)
```

Download params: `?url=<url>`, `?raw=true`, `?output=file`

Upload body: `{"selector": "input[type=file]", "files": ["data:...base64..."]}`

## Cookies

```
GET  /cookies                            # Get cookies for current page
POST /cookies                            # Set cookies
GET  /tabs/{id}/cookies                  # Get cookies (specific tab)
POST /tabs/{id}/cookies                  # Set cookies (specific tab)
```

## Fingerprint

```
POST /fingerprint/rotate                 # Rotate browser fingerprint
```

## Instances (multi-instance)

```
GET  /instances                          # List all instances
GET  /instances/{id}                     # Get instance details
GET  /instances/tabs                     # List tabs across all instances
GET  /instances/metrics                  # Metrics across all instances
POST /instances/start                    # Start new instance
POST /instances/launch                   # Launch by profile name
POST /instances/attach                   # Attach external browser
POST /instances/{id}/start               # Start specific instance
POST /instances/{id}/stop                # Stop specific instance
GET  /instances/{id}/logs                # Instance logs (ring buffer)
GET  /instances/{id}/logs/stream         # Stream logs (SSE)
GET  /instances/{id}/tabs                # List instance tabs
POST /instances/{id}/tabs/open           # Open tab in instance
POST /instances/{id}/tab                 # Tab action proxied to instance
```

## Profiles

```
GET  /profiles                           # List all profiles
POST /profiles                           # Create profile
POST /profiles/create                    # Create profile (alias)
GET  /profiles/{id}                      # Get profile details
PATCH /profiles/{id}                     # Update profile
DELETE /profiles/{id}                    # Delete profile
POST /profiles/{id}/start               # Start instance for profile
POST /profiles/{id}/stop                # Stop instance for profile
GET  /profiles/{id}/instance            # Get profile's running instance
POST /profiles/{id}/reset               # Reset profile data
GET  /profiles/{id}/logs                # Profile action logs
GET  /profiles/{id}/analytics           # Profile analytics
POST /profiles/import                   # Import profile from path
PATCH /profiles/meta                    # Update profile metadata
```

## Scheduler

```
POST /tasks                              # Submit task to queue
GET  /tasks                              # List queued/running tasks
GET  /tasks/{id}                         # Get task status and result
POST /tasks/{id}/cancel                  # Cancel a task
POST /tasks/batch                        # Submit batch of tasks
GET  /scheduler/stats                    # Queue stats (depth, inflight, agents)
```

Task body: `{"agentId": "...", "action": "snapshot", "tabId": "..."}`

## Config (Dashboard API)

```
GET  /api/config                         # Get current configuration
PUT  /api/config                         # Update configuration
```

`server.token` is write-only and managed outside the dashboard API via CLI or config file updates.
