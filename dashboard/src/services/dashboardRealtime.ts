import { useAppStore } from "../stores/useAppStore";
import { isClientActivityEvent, subscribeToEvents } from "./api";
import {
  handoffFromSystemEvent,
  mergeAgents,
  resumeTabIdFromSystemEvent,
} from "./realtimeReducers";

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
        const handoff = handoffFromSystemEvent(event);
        if (handoff) {
          state.addHandoffNotification(handoff);
          return;
        }
        const resumeTabId = resumeTabIdFromSystemEvent(event);
        if (resumeTabId) {
          state.dismissHandoffNotification(resumeTabId);
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
