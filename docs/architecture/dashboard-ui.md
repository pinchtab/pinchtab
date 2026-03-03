# Dashboard UI Architecture

The PinchTab dashboard is a modern React SPA that provides visual control and monitoring of Chrome instances.

## Overview

**Technology Stack:**
- React 19 + TypeScript
- Tailwind CSS v4 (dark theme)
- Zustand (state management)
- Vite (build)
- Vitest (testing)
- Hash-based routing

**Build Flow:**
```
dashboard/src (React source)
    ↓
bun run build
    ↓
dashboard/dist (optimized bundle)
    ↓
scripts/build-dashboard.sh (copies to Go binary)
    ↓
internal/dashboard/dashboard/ (embedded)
    ↓
//go:embed dashboard/* (compiled into binary)
```

## Visual Guide

### Overall Dashboard Layout

```
┌─────────────────────────────────────────────────────────────────┐
│ PinchTab    Monitoring  Agents  Profiles  Settings    ↻ ☰       │  ← NavBar (sticky)
├─────────────────────────────────────────────────────────────────┤
│                                                                   │
│                         MAIN CONTENT AREA                        │
│                                                                   │
│                    (One of 4 Pages Below)                        │
│                                                                   │
└─────────────────────────────────────────────────────────────────┘
```

**Features:**
- Dark theme (#0a0a0a bg, #1a1a1a surface)
- Orange accent (#f97316) on active/hover states
- Responsive: Mobile hamburger, desktop horizontal tabs
- Sticky header with refresh button (⌘R)

## Layout & Pages

### Navigation Bar
- Sticky header with tabs and refresh button
- Desktop: horizontal navigation (Monitoring, Agents, Profiles, Settings)
- Mobile: hamburger menu + responsive layout
- Keyboard shortcuts: ⌘1-4 (navigate), ⌘R (refresh)

### 1. **Monitoring Page** (`/monitoring`) - Default

Real-time instance and tab monitoring with memory tracking.

**Visual Layout:**
```
┌─ Chart Area ─────────────────────────────┐
│  ↑ Tabs / Memory (dual Y-axis)            │
│  │     ▂▄▆█▊▋▊▉▆▅▃▁                      │  ← Solid: tabs, Dashed: memory
│  │    ▁ ▃▅▇ ▆█▁ ▂▄▆█▁▂                   │
│  ├─────────────────────────────────────→  │
│  │ 60 min history (polled every 30s)      │
└───────────────────────────────────────────┘

┌─ Instances ───────────────────────────────┐
│ ┌─ instance-1 ┐  ┌─ instance-2 ┐          │
│ │ :9868       │  │ :9869       │          │  ← Shows port, tab count, memory
│ │ 5 tabs      │  │ 12 tabs     │          │
│ │ 524MB       │  │ 1382MB      │          │
│ └─────────────┘  └─────────────┘          │
│ ┌─ Selected Instance Tabs ──────────────┐ │
│ │ └─ tab_abc123 (google.com)            │ │  ← Tab list for selected instance
│ │    └─ tab_def456 (github.com)         │ │
│ └───────────────────────────────────────┘ │
└───────────────────────────────────────────┘
```

**Components:**
- **Dual-Axis Chart**
  - Left Y-axis: Tab count (solid lines)
  - Right Y-axis: Memory in MB (dashed lines)
  - History: 60-minute window
  - Poll interval: 30 seconds (configurable in Settings)

- **Instance List**
  - Grid of running instances
  - Shows: port, tab count, memory (e.g., `:9868 · 5 tabs · 524MB`)
  - Click to select and view tabs

- **Tab Details**
  - List of tabs in selected instance
  - Shows tab title, URL, and ID

**Data Flow:**
```
MonitoringPage
  ↓
  └─ Poll /instances/metrics (every 30s)
  └─ Poll /instances/{id}/tabs (per selected instance)
  └─ addChartDataPoint() → store → chart re-render
```

### 2. **Agents Page** (`/agents`)

Real-time view of agent activity and events.

**Visual Layout:**

Desktop (sm:):
```
┌─ Sidebar ──────┬─ Activity ─────────┐
│ All            │ Filter:            │
│ ► Agent-1      │ Navigate           │
│   Agent-2      │ Snapshot           │
│   Agent-3      │ Actions            │
│                │                    │
│ [events]       │ [activity stream]  │
└────────────────┴────────────────────┘
```

Mobile:
```
┌─ Carousel ─────────────────┐
│ All → A1 → A2 → A3 ← →     │
└────────────────────────────┘
┌─ Activity ─────────────────┐
│ Filter: All / Navigate /   │
│ Snapshot / Actions         │
│ [activity stream]          │
└────────────────────────────┘
```

**Desktop Layout (detailed):**
```
┌─ Sidebar ──────┬─ Activity Log ──────────┐
│ All            │ Filter: All/Navigate/   │
│ ► Agent-1      │ Snapshot/Actions        │
│   Agent-2      │                         │
│   Agent-3      │ [Events stream via SSE] │
│                │                         │
│ [events list]  │ [activity lines]        │
└────────────────┴─────────────────────────┘
```

**Mobile Layout:**
```
┌─ Carousel (horizontal scroll) ─┐
│ All → Agent-1 → Agent-2 → A-3  │  
└────────────────────────────────┘
┌─ Activity Log below ───────────┐
│ [events]                       │
└────────────────────────────────┘
```

**Data Flow:**
```
App.tsx
  ↓
  └─ subscribeToEvents() → SSE /api/events
  └─ init: list of agents
  └─ action: agent performed action
  └─ system: instance lifecycle
  ↓
AgentsPage receives updates
  └─ Filter events
  └─ Render agents + activity
```

### 3. **Profiles Page** (`/profiles`)

Manage Chrome profiles and launch instances.

**Visual Layout:**
```
┌─ Profiles Grid (2 cols desktop, 1 col mobile) ─┐
│ ┌─────────────┐  ┌─────────────┐              │
│ │ Chrome      │  │ Firefox     │              │
│ │ id: prof-1  │  │ id: prof-2  │              │
│ │ [Launch ▼]  │  │ [Launch ▼]  │              │
│ └─────────────┘  └─────────────┘              │
│ ┌─────────────┐                               │
│ │ Edge        │  [+ Create Profile]           │
│ │ id: prof-3  │                               │
│ │ [Launch ▼]  │                               │
│ └─────────────┘                               │
└────────────────────────────────────────────────┘

Modals:
[Launch Dialog]         [Details Dialog]        [Create Dialog]
├ Port: 9868           ├ Name: Chrome          ├ Name: [input]
├ ☐ Headless           ├ Created: 2026-03-02   ├ Use When: [input]
├ [Launch] [Cancel]    └ [Close]               ├ [Create] [Cancel]
```

**Modals:**
1. **Create Profile Dialog**
   - Name input
   - Use When (optional description)
   - Create button

2. **Launch Instance Dialog**
   - Profile selection (pre-filled)
   - Port number (default 9868)
   - Headless toggle (default true)
   - Launch button

3. **Profile Details Modal**
   - Profile metadata
   - Copy buttons for ID
   - Close button

**Data Flow:**
```
ProfilesPage
  ↓
  └─ Fetch /profiles (on mount if empty)
  └─ SSE updates (new profiles)
  ↓
User clicks "Launch"
  ↓
  └─ POST /profiles/{id}/launch with { port, headless }
  └─ Fetch /instances (updated list)
```

### 4. **Settings Page** (`/settings`)

Configure dashboard behavior.

**Visual Layout:**
```
┌─ Settings Page ────────────────────┐
│ ┌─ Monitoring Settings ──────────┐ │
│ │ ☐ Memory Metrics (experimental)│ │
│ │   Poll Interval: 30s [input]   │ │
│ │                                │ │
│ │ Server Info:                   │ │
│ │ Go Runtime v1.26               │ │
│ │ Goroutines: 15                 │ │
│ │ Heap: 12.5 MB                  │ │
│ │                                │ │
│ │              [Reset] [Apply ✓] │ │  ← Disabled until changes
│ └────────────────────────────────┘ │
└────────────────────────────────────┘
```

**Settings:**
- **Memory Metrics** toggle (experimental)
  - When ON: dashboard polls memory data, chart shows memory lines
  - When OFF: skips `/instances/metrics` calls
  
- **Poll Interval** input
  - Default: 30 seconds
  - Controls chart update frequency

- **Server Info Display**
  - Go version
  - Goroutines count
  - Heap allocation

**Behavior:**
- All settings stored in localStorage
- Apply/Reset buttons (disabled until changes)
- Uses `JSON.stringify` to detect changes

**Data Flow:**
```
SettingsPage
  ↓
User toggles Memory Metrics
  ↓
hasChanges = true → Apply button enabled
  ↓
Click Apply
  ↓
setSettings(local) → localStorage + store
  ↓
MonitoringPage reacts → adjusts polling behavior
```

## State Management (Zustand Store)

**useAppStore** manages:
- `instances[]` - running Chrome instances
- `profiles[]` - available profiles
- `agents[]` - connected agents
- `events[]` - agent activity log
- `settings` - dashboard preferences
- `tabsChartData[]` - time-series for chart
- `memoryChartData[]` - memory history
- `serverChartData[]` - Go runtime metrics
- `currentTabs{}` - tabs per instance
- `currentMemory{}` - memory per instance

**Persistence:**
- `settings` persisted to localStorage
- `tabsChartData`, `memoryChartData` kept in memory only
- Chart data limited to last 60 points (~30 min)

## API Endpoints

**Dashboard calls:**
- `GET /health` - server status
- `GET /profiles` - list profiles
- `GET /instances` - list running instances
- `GET /instances/metrics` - memory per instance
- `GET /instances/{id}/tabs` - tabs in instance
- `POST /profiles/{id}/launch` - start instance
- `GET /api/events` - SSE event stream (real-time)

**Environment:**
- Development: proxy via Vite (`vite.config.ts`)
- Production: same origin (served from Go at `/dashboard/`)

## Styling & Theme

**Design System:**
- Dark theme: `#0a0a0a` (bg), `#1a1a1a` (surface)
- Orange accent: `#f97316` (primary actions)
- Text: `#e4e4e7` (primary), `#a1a1a1` (secondary)
- Border: `#2a2a2a` (subtle)

**Tailwind Configuration:**
- Atomic design: buttons, inputs, cards are reusable
- Responsive: `sm:` breakpoint at 640px
- Dark mode optimized (no light theme)
- Custom CSS for NavBar animation, chart styling

**Components Hierarchy:**
```
App
  ├─ NavBar (sticky header)
  └─ Routes
      ├─ MonitoringPage
      │   ├─ TabsChart (Recharts)
      │   ├─ InstanceListItem (molecule)
      │   └─ TabItem (molecule)
      ├─ AgentsPage
      │   ├─ AgentItem (molecule)
      │   └─ ActivityLine (molecule)
      ├─ ProfilesPage
      │   ├─ ProfileCard (molecule)
      │   └─ Modal (atom)
      └─ SettingsPage
          └─ Toolbar (atom)
```

## Testing

**Unit Tests (69 total):**
- Atoms: Badge, Button, StatusDot (7 tests each)
- Molecules: ProfileCard, TabsChart, InstanceListItem, TabItem (3-16 tests)
- Store: useAppStore (19 tests)

**Test Framework:**
- Vitest (via `bun run test`)
- jsdom environment
- Render + user interaction testing

**Gap:**
- No E2E tests for dashboard ↔ Go integration
- Future: Playwright tests for full workflows

## Performance

**Optimization:**
- Vite + code splitting (assets have hash in filename)
- Chart data limited to 60 points (prevents memory bloat)
- useCallback for event handlers
- Memoization for expensive computations
- SSE for push updates (no polling for events)

**Caching:**
- Assets: `Cache-Control: max-age=31536000, immutable` (1 year)
- HTML: `Cache-Control: no-store` (always fresh)
- Settings: localStorage

## Development Workflow

**Build:**
```bash
cd dashboard
bun install
bun run build
```

**Local Dev:**
```bash
cd dashboard
bun run dev  # Runs on :5173, proxies /api to :9867
```

**Test:**
```bash
cd dashboard
bun run test:run
```

**CI/CD:**
- Dashboard workflow triggers on push to `dashboard/` or `.github/workflows/dashboard.yml`
- Lint, type check, test, build all run
- Build artifacts uploaded for release

## Future Enhancements

1. **E2E Tests** - Playwright tests for full workflows
2. **Real-time Collaboration** - Multi-user dashboard
3. **Advanced Filtering** - Tag-based instance filtering
4. **Custom Charts** - User-defined metrics
5. **Dark/Light Theme** - Theme toggle
6. **Notification Center** - Persistent alerts for important events
