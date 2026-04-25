import type { DashboardActivityEvent } from "./types";

// Returns true for POST {tab}/handoff events (the pause action).
export function isHandoffEvent(event: DashboardActivityEvent): boolean {
  return (
    event.method === "POST" &&
    event.path.endsWith("/handoff") &&
    Boolean(event.tabId)
  );
}

// Returns true for POST {tab}/resume events (the unpause action).
export function isResumeEvent(event: DashboardActivityEvent): boolean {
  return (
    event.method === "POST" &&
    event.path.endsWith("/resume") &&
    Boolean(event.tabId)
  );
}

// computeHandoffTabs walks events in chronological order and returns the set
// of tabIds whose most recent handoff/resume signal is a pause. A tab goes
// into the set on POST .../handoff and leaves it on POST .../resume.
export function computeHandoffTabs(
  events: DashboardActivityEvent[],
): Set<string> {
  const paused = new Set<string>();

  const sorted = [...events].sort(
    (a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime(),
  );

  for (const event of sorted) {
    if (!event.tabId) continue;
    if (isHandoffEvent(event) && event.status >= 200 && event.status < 300) {
      paused.add(event.tabId);
    } else if (
      isResumeEvent(event) &&
      event.status >= 200 &&
      event.status < 300
    ) {
      paused.delete(event.tabId);
    }
  }

  return paused;
}

// deriveSessionAgentIndex builds two sets:
//  - sessionsWithHandoff: sessions that touched any currently-paused tab
//  - agentsWithHandoff:   agents that touched any currently-paused tab
//
// We look across all events (not just handoff/resume) so a session is flagged
// as long as it produced traffic on a tab that is now in handoff.
export function deriveHandoffIndex(
  events: DashboardActivityEvent[],
  handoffTabs: Set<string>,
): { sessionsWithHandoff: Set<string>; agentsWithHandoff: Set<string> } {
  const sessionsWithHandoff = new Set<string>();
  const agentsWithHandoff = new Set<string>();

  if (handoffTabs.size === 0) {
    return { sessionsWithHandoff, agentsWithHandoff };
  }

  for (const event of events) {
    if (!event.tabId || !handoffTabs.has(event.tabId)) continue;
    if (event.sessionId) sessionsWithHandoff.add(event.sessionId);
    if (event.agentId) agentsWithHandoff.add(event.agentId);
  }

  return { sessionsWithHandoff, agentsWithHandoff };
}
