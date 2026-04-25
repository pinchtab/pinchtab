import type { Agent } from "../generated/types";
import { useAppStore } from "../stores/useAppStore";
import { isClientActivityEvent, subscribeToEvents } from "./api";

interface RealtimeHandle {
  consumers: number;
  includeMemory: boolean;
  unsubscribe: (() => void) | null;
}

const realtimeHandle: RealtimeHandle = {
  consumers: 0,
  includeMemory: false,
  unsubscribe: null,
};

function sortAgents(agents: Agent[]): Agent[] {
  return [...agents].sort(
    (left, right) =>
      new Date(right.lastActivity || right.connectedAt).getTime() -
      new Date(left.lastActivity || left.connectedAt).getTime(),
  );
}

function mergeAgents(current: Agent[], incoming: Agent[]): Agent[] {
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

function startDashboardRealtime(includeMemory: boolean) {
  realtimeHandle.unsubscribe?.();
  realtimeHandle.includeMemory = includeMemory;
  realtimeHandle.unsubscribe = subscribeToEvents(
    {
      onInit: (agents) => {
        const state = useAppStore.getState();
        state.setAgents(mergeAgents(state.agents, agents));
      },
      onSystem: (event) => {
        const state = useAppStore.getState();
        if (event.type === "tab.handoff") {
          const payload = (event.instance ?? {}) as Record<string, unknown>;
          const tabId =
            typeof payload.tabId === "string" ? payload.tabId : null;
          if (!tabId) {
            return;
          }
          state.addHandoffNotification({
            tabId,
            reason:
              typeof payload.reason === "string"
                ? payload.reason
                : "manual_handoff",
            hint: typeof payload.hint === "string" ? payload.hint : undefined,
            source:
              typeof payload.source === "string" ? payload.source : undefined,
            url: typeof payload.url === "string" ? payload.url : undefined,
            title:
              typeof payload.title === "string" ? payload.title : undefined,
            receivedAt: Date.now(),
          });
          return;
        }
        if (event.type === "tab.resume") {
          const payload = (event.instance ?? {}) as Record<string, unknown>;
          if (typeof payload.tabId === "string") {
            state.dismissHandoffNotification(payload.tabId);
          }
          return;
        }
      },
      onActivity: (event) => {
        if (!isClientActivityEvent(event)) {
          return;
        }
        const state = useAppStore.getState();
        state.upsertAgentFromEvent(event);
        state.appendAgentEvent(event);
        state.addEvent(event);
      },
      onMonitoring: (snapshot) => {
        useAppStore
          .getState()
          .applyMonitoringSnapshot(snapshot, realtimeHandle.includeMemory);
      },
    },
    {
      includeMemory,
      reasoningMode: "both",
    },
  );
}

export function acquireDashboardRealtime(includeMemory: boolean): () => void {
  realtimeHandle.consumers += 1;

  if (
    realtimeHandle.unsubscribe === null ||
    realtimeHandle.includeMemory !== includeMemory
  ) {
    startDashboardRealtime(includeMemory);
  }

  return () => {
    realtimeHandle.consumers = Math.max(0, realtimeHandle.consumers - 1);
    if (realtimeHandle.consumers > 0) {
      return;
    }
    realtimeHandle.unsubscribe?.();
    realtimeHandle.unsubscribe = null;
  };
}
