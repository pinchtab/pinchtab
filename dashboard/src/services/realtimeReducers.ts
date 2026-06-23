import type { Agent } from "../generated/types";
import type { HandoffNotification } from "../stores/useAppStore";
import type { SystemEvent } from "./api";

export function sortAgents(agents: Agent[]): Agent[] {
  return [...agents].sort(
    (left, right) =>
      new Date(right.lastActivity || right.connectedAt).getTime() -
      new Date(left.lastActivity || left.connectedAt).getTime(),
  );
}

export function mergeAgents(current: Agent[], incoming: Agent[]): Agent[] {
  const merged = new Map<string, Agent>();

  for (const agent of current) {
    merged.set(agent.id, agent);
  }

  for (const agent of incoming) {
    const existing = merged.get(agent.id);
    if (!existing) {
      merged.set(agent.id, agent);
      continue;
    }

    merged.set(agent.id, {
      ...existing,
      ...agent,
      connectedAt:
        new Date(existing.connectedAt).getTime() <
        new Date(agent.connectedAt).getTime()
          ? existing.connectedAt
          : agent.connectedAt,
      lastActivity:
        new Date(existing.lastActivity || existing.connectedAt).getTime() >
        new Date(agent.lastActivity || agent.connectedAt).getTime()
          ? existing.lastActivity
          : agent.lastActivity,
      requestCount: Math.max(existing.requestCount, agent.requestCount),
    });
  }

  return sortAgents([...merged.values()]);
}

// handoffFromSystemEvent parses a tab.handoff system event into a notification,
// or null when the event is not a handoff or carries no tab id.
export function handoffFromSystemEvent(
  event: SystemEvent,
): HandoffNotification | null {
  if (event.type !== "tab.handoff") {
    return null;
  }
  const payload = (event.instance ?? {}) as Record<string, unknown>;
  const tabId = typeof payload.tabId === "string" ? payload.tabId : null;
  if (!tabId) {
    return null;
  }
  return {
    tabId,
    reason:
      typeof payload.reason === "string" ? payload.reason : "manual_handoff",
    hint: typeof payload.hint === "string" ? payload.hint : undefined,
    source: typeof payload.source === "string" ? payload.source : undefined,
    url: typeof payload.url === "string" ? payload.url : undefined,
    title: typeof payload.title === "string" ? payload.title : undefined,
    receivedAt: Date.now(),
  };
}

// resumeTabIdFromSystemEvent returns the tab id to dismiss for a tab.resume event,
// or null when the event is not a resume or carries no tab id.
export function resumeTabIdFromSystemEvent(event: SystemEvent): string | null {
  if (event.type !== "tab.resume") {
    return null;
  }
  const payload = (event.instance ?? {}) as Record<string, unknown>;
  return typeof payload.tabId === "string" ? payload.tabId : null;
}
