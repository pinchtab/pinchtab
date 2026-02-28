# UI Enhancements

## Overview

Focuses on improving the dashboard user experience with real-time monitoring, batch operations, and better instance navigation. Current dashboard provides basic instance listing; Phase 8 adds production-grade observability and control.

## Goals

- Real-time visibility into instance health and resource usage
- Batch operations for efficient multi-instance management
- Improved navigation and search across large instance sets
- Live debugging and diagnostics capability

---

## Section 1: Instance Monitoring Screen

### Objectives

Provide comprehensive real-time visibility into instance status, resource consumption, and activity.

### Tasks

#### 1.1 Real-Time Chrome Status

- [ ] **Display per-instance Chrome state**
  - Chrome running / not running / initializing
  - Process ID
  - Window handle (for headed instances)
  - Last activity timestamp

- [ ] **Implementation Strategy**
  - Add `/instances/{id}/health/chrome` endpoint (returns {status, pid, active_window, last_activity})
  - Update dashboard UI to poll every 2-5 seconds
  - Show status indicator (green=running, yellow=initializing, red=crashed)
  - Display "last activity" as relative time (e.g., "2 mins ago")

- [ ] **UI Components**
  - Chrome status badge in instance card
  - "Restart Chrome" button for crashed instances
  - Process info popup (PID, memory, CPU)

- [ ] **Testing**
  - Verify status updates reflect actual state
  - Test with crashed Chrome (simulate process kill)
  - Verify "Restart Chrome" brings instance back online

- [ ] **Acceptance Criteria**
  - Status updates within 5 seconds
  - Accurate reflection of Chrome state
  - Graceful handling of process crashes
  - Clear user feedback

#### 1.2 Memory & CPU Usage per Instance

- [ ] **Display resource consumption**
  - RSS (resident set size) in MB
  - CPU usage as percentage
  - Trend over time (sparkline)
  - Resource threshold alerts (warn if > 80% of available)

- [ ] **Implementation Strategy**
  - Add `/instances/{id}/health/resources` endpoint (returns {memory_mb, cpu_percent, trend})
  - Collect metrics every 10-30 seconds
  - Calculate trend from last 10 samples
  - Store metrics in circular buffer (last 100 samples)

- [ ] **Metric Collection Methods**
  - Linux: Parse `/proc/[pid]/stat` and `/proc/[pid]/status`
  - macOS: Use `ps` command or native APIs
  - Include Chrome process + all child processes

- [ ] **UI Components**
  - Memory gauge (0-1000 MB range, color-coded)
  - CPU percentage with trend sparkline
  - Alert badge if threshold exceeded
  - Click to see detailed metrics over time

- [ ] **Testing**
  - Verify metrics accurate compared to system tools (`ps`, `top`)
  - Test with idle instance vs active navigation
  - Verify trend calculation correct
  - Test threshold alerts

- [ ] **Acceptance Criteria**
  - Memory readings within ±5 MB of actual
  - CPU readings within ±2% of actual
  - Metrics updated every 10-30 seconds
  - Alerts triggered correctly at thresholds

#### 1.3 Tab Activity Visualization

- [ ] **Display per-instance tab activity**
  - Tab list with URL, title
  - Last modified time
  - Activity sparkline (show tab usage over time)
  - Highlight most active tab

- [ ] **Implementation Strategy**
  - Extend `/instances/{id}/tabs` to include: `last_modified`, `activity_count`, `last_action`
  - Track tab actions (navigate, click, etc.) and timestamps
  - Calculate activity trend (e.g., actions/minute)
  - Reset counters hourly

- [ ] **UI Components**
  - Tab list in instance details panel
  - Activity sparkline for each tab
  - Hover to see: URL, title, last action, action count
  - Color indicate activity level (green=active, gray=idle)

- [ ] **Testing**
  - Verify activity tracking accuracy
  - Test with multiple concurrent tabs
  - Verify sparkline rendering
  - Test activity reset on schedule

- [ ] **Acceptance Criteria**
  - Activity metrics accurate
  - Sparklines render correctly
  - Performance impact negligible (< 5% CPU increase)
  - Activity data persists across instance lifecycle

---

## Section 2: Batch Operations

### Objectives

Enable efficient management of multiple instances with bulk actions.

### Tasks

#### 2.1 Create Multiple Instances at Once

- [ ] **Bulk instance creation UI**
  - Input: Count, profile name, headless flag
  - e.g., "Create 10 scraping instances (headless)"
  - Progress indicator during creation
  - Confirmation of all created (IDs, ports, status)

- [ ] **Implementation Strategy**
  - New form on "Instances" tab
  - Add `/instances/batch-create` endpoint (accepts {count, profile, headless})
  - Create instances in sequence (avoid overwhelming system)
  - Return array of created instance IDs and ports

- [ ] **UI Components**
  - "Create Batch" button
  - Modal with count/profile/headless selectors
  - Progress bar during creation
  - Success confirmation with download CSV option (IDs, ports)

- [ ] **Error Handling**
  - Handle port exhaustion (stop at max available)
  - Handle instance creation failures (skip, continue with others)
  - Provide detailed error messages

- [ ] **Testing**
  - Create 5 instances in batch
  - Verify all created with correct config
  - Test port exhaustion scenario
  - Verify error handling

- [ ] **Acceptance Criteria**
  - 5 instances created in < 30 seconds
  - All instances reachable and responsive
  - Clear feedback on success/failure
  - CSV export contains all created instances

#### 2.2 Terminate All Instances Button

- [ ] **Bulk shutdown UI**
  - "Terminate All" button with confirmation
  - Optional: Select subset before terminating
  - Progress indicator during shutdown
  - Cleanup verification (no orphaned processes)

- [ ] **Implementation Strategy**
  - Add `/instances/batch-stop` endpoint (stops all or filtered list)
  - Send stop requests in parallel (with concurrency limit)
  - Track completion
  - Verify ports released

- [ ] **UI Components**
  - "Terminate All" button in Instances toolbar
  - Confirmation dialog: "Terminate all {N} instances?"
  - Progress bar with count (e.g., "Stopped 7/10")
  - Success message: "All instances terminated, ports released"

- [ ] **Error Handling**
  - Handle instances that fail to stop (provide manual options)
  - Log any orphaned processes
  - Clear error reporting

- [ ] **Testing**
  - Create 5 instances, terminate all
  - Verify all stopped
  - Verify ports released (can create new instances)
  - Test partial failure scenario

- [ ] **Acceptance Criteria**
  - All instances stopped in < 30 seconds
  - All ports released and available for reuse
  - No orphaned Chrome processes
  - Clear feedback on results

#### 2.3 Port Range Configuration UI

- [ ] **Configure instance port range**
  - Display current range (e.g., 9868-9968)
  - Allow modification (validate: start < end, reasonable range)
  - Show available ports vs allocated
  - Visual indicator of "capacity" (e.g., "48/100 ports allocated")

- [ ] **Implementation Strategy**
  - Add `/config/port-range` endpoint (GET/POST)
  - GET returns: current_start, current_end, allocated_count, total_count
  - POST to change range (with validation)
  - Update dashboard to call GET on load

- [ ] **UI Components**
  - Settings panel with port range inputs
  - Validation error messages
  - Visual progress bar: "48 of 100 ports in use"
  - Warning: "Some instances may be disconnected if range changed while running"
  - "Apply" button (with confirmation if instances running)

- [ ] **Error Handling**
  - Reject invalid ranges (start >= end, out of system limits)
  - Warn if changing while instances running
  - Prevent change if no available capacity for existing instances

- [ ] **Testing**
  - Change port range from 9800-9900 to 9900-10000
  - Verify new instances use new range
  - Test invalid range inputs (rejected)
  - Test changing range with instances running (warn user)

- [ ] **Acceptance Criteria**
  - Port range configurable from UI
  - Validation prevents invalid configurations
  - User can see allocated vs available ports
  - Changes take effect immediately for new instances

---

## Section 3: Better Navigation & UX

### Objectives

Improve usability when managing large numbers of instances.

### Tasks

#### 3.1 Search & Filter Instances

- [ ] **Search and filter capabilities**
  - Search by instance ID, profile name, port
  - Filter by: status (running/stopped/error), headless (true/false)
  - Filter by resource usage (e.g., "memory > 200MB")
  - Combine multiple filters

- [ ] **Implementation Strategy**
  - Frontend filtering (no backend API change needed initially)
  - Add search box to Instances toolbar
  - Add filter button with dropdown/checkboxes
  - Real-time filtering as user types/selects

- [ ] **UI Components**
  - Search input field (with clear button)
  - "Filters" dropdown showing active filters
  - Filter options: Status, Profile, Headless, Memory, CPU
  - Results count: "Showing 12 of 50 instances"
  - "Clear filters" button

- [ ] **Testing**
  - Search for instance ID (partial match)
  - Search for profile name
  - Filter by status
  - Combine multiple filters
  - Verify result count accurate

- [ ] **Acceptance Criteria**
  - Search results instant (< 100ms) for 100+ instances
  - All filter combinations work
  - Results count accurate
  - Clear indication of active filters

#### 3.2 Instance Details Modal

- [ ] **Detailed view for single instance**
  - Full instance info (ID, profile, port, mode, headless)
  - Resource metrics (memory, CPU, trend)
  - Chrome status (PID, window handle)
  - Tab list with activity metrics
  - Action buttons (restart Chrome, restart instance, stop instance)
  - Log viewer (last 50 lines of instance logs)

- [ ] **Implementation Strategy**
  - Create modal component that opens on instance click
  - Combine data from: `/instances/{id}`, `/health/chrome`, `/health/resources`, `/tabs`
  - Add `/instances/{id}/logs` endpoint (returns last N log lines)
  - Update modal every 5 seconds with fresh data

- [ ] **UI Components**
  - Modal with tabs: Overview, Resources, Tabs, Logs
  - Responsive design (readable on small screens)
  - Copy-to-clipboard for IDs/URLs
  - Action buttons with confirmation dialogs

- [ ] **Testing**
  - Open detail modal for multiple instances
  - Verify all data displayed correctly
  - Verify refresh works (data updates)
  - Verify actions work (restart, stop)
  - Test with many tabs (scroll, performance)

- [ ] **Acceptance Criteria**
  - Modal loads in < 1 second
  - Data refreshes every 5 seconds
  - All instance details visible
  - Actions execute correctly
  - Performance acceptable with 50+ tabs

#### 3.3 Live Logs Viewer

- [ ] **Real-time log visualization**
  - Stream logs from orchestrator and instances
  - Filter by log level (info, warning, error, debug)
  - Filter by source (orchestrator, instance ID)
  - Timestamp on each log
  - Autoscroll (can toggle off)
  - Search within logs

- [ ] **Implementation Strategy**
  - Add `/logs` endpoint with SSE (Server-Sent Events) or WebSocket
  - Return stream of {timestamp, level, source, message}
  - Client-side filtering (no backend filter needed initially)
  - Store last 1000 logs in memory

- [ ] **UI Components**
  - New "Logs" tab in main navigation
  - Timeline view with scrollable log list
  - Filter controls: level, source, search term
  - Clear logs button
  - Export logs button (download as text)

- [ ] **Testing**
  - Verify logs stream in real-time
  - Verify filtering works (level, source)
  - Test with high log volume (1000+ logs/minute)
  - Verify autoscroll works
  - Test lag/latency with updates

- [ ] **Acceptance Criteria**
  - Logs appear in < 500ms after event
  - Filtering instant (no noticeable lag)
  - Performance acceptable with 1000+ logs
  - Search works on displayed logs
  - Clear and export functions work

---

## Success Metrics

| Feature | Metric | Target | Priority |
|---------|--------|--------|----------|
| Chrome status | Update latency | < 5s | High |
| Resource metrics | Accuracy | ±5% of actual | High |
| Batch create | Time for 10 instances | < 30s | Medium |
| Search | Response time | < 100ms | Medium |
| Log viewer | Update latency | < 500ms | Low |

---

## Timeline Estimate

- **Chrome status (1.1)**: 2-3 hours
- **Resource metrics (1.2)**: 2-3 hours
- **Tab activity (1.3)**: 1-2 hours
- **Batch create (2.1)**: 2-3 hours
- **Terminate all (2.2)**: 1-2 hours
- **Port range config (2.3)**: 1-2 hours
- **Search & filter (3.1)**: 2-3 hours
- **Details modal (3.2)**: 3-4 hours
- **Live logs (3.3)**: 2-3 hours
- **Integration & testing**: 2-3 hours

**Total: 18-28 hours**

---

## Deliverables

- [ ] Updated dashboard with monitoring screen
- [ ] Monitoring components: Chrome status, resource metrics, tab activity
- [ ] Batch operations: Create multiple, terminate all
- [ ] Port range configuration UI
- [ ] Search and filter system
- [ ] Instance details modal with logs viewer
- [ ] Updated `dashboard/` frontend files
- [ ] New API endpoints: `/health/*`, `/batch-*`, `/logs`, `/config/*`
- [ ] Updated documentation: Dashboard usage guide

---

## Notes

- Prioritize high-visibility metrics (Chrome status, memory usage)
- Use polling initially, upgrade to WebSocket/SSE if performance issues
- Consider mobile responsiveness for remote dashboards
- Log viewer can be phase 8.5 if time-constrained
- Batch operations enable scripting (e.g., "create 100 instances for load test")
- Details modal is foundation for future alerts and anomaly detection
