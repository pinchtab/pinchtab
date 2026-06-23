import type { StateCreator } from "zustand";
import type { Agent, ActivityEvent } from "../../generated/types";
import type { AppState } from "../useAppStore";

const MAX_AGENT_CACHE_SIZE = 20;
const MAX_AGENT_EVENTS_PER_AGENT = 200;

function agentActivityTime(agent: Agent): number {
  return new Date(agent.lastActivity || agent.connectedAt).getTime();
}

function normalizeAgents(agents: Agent[]): Agent[] {
  const deduped = new Map<string, Agent>();
  for (const agent of agents) {
    deduped.set(agent.id, agent);
  }
  return [...deduped.values()].sort(
    (a, b) => agentActivityTime(b) - agentActivityTime(a),
  );
}

function retainedAgentIds(
  agents: Agent[],
  selectedAgentId: string | null,
  extraAgentIds: string[] = [],
): string[] {
  const ids: string[] = [];
  const seen = new Set<string>();
  const push = (id: string | null | undefined) => {
    const normalized = id?.trim();
    if (
      !normalized ||
      seen.has(normalized) ||
      ids.length >= MAX_AGENT_CACHE_SIZE
    ) {
      return;
    }
    seen.add(normalized);
    ids.push(normalized);
  };

  push(selectedAgentId);
  for (const id of extraAgentIds) {
    push(id);
  }
  for (const agent of normalizeAgents(agents)) {
    push(agent.id);
  }
  return ids;
}

function pruneAgentEventsById(
  agentEventsById: Record<string, ActivityEvent[]>,
  retainedIds: string[],
): Record<string, ActivityEvent[]> {
  const retained = new Set(retainedIds);
  return Object.fromEntries(
    Object.entries(agentEventsById).filter(([agentId]) =>
      retained.has(agentId),
    ),
  );
}

// mergeAgentEvents combines one agent's event lists: de-dupe by id (first
// occurrence wins, so an existing event beats an incoming duplicate), order
// ascending by timestamp (stable, preserving ties), and cap at the retention
// limit. Set-based de-dupe keeps this linear rather than the previous
// reduce + Array.some O(n^2) merge.
function mergeAgentEvents(
  existing: ActivityEvent[],
  incoming: ActivityEvent[],
): ActivityEvent[] {
  const seen = new Set<string>();
  const merged: ActivityEvent[] = [];
  for (const event of [...existing, ...incoming]) {
    if (seen.has(event.id)) {
      continue;
    }
    seen.add(event.id);
    merged.push(event);
  }
  merged.sort(
    (left, right) =>
      new Date(left.timestamp).getTime() - new Date(right.timestamp).getTime(),
  );
  return merged.slice(-MAX_AGENT_EVENTS_PER_AGENT);
}

export interface AgentsSlice {
  agents: Agent[];
  selectedAgentId: string | null;
  agentEventsById: Record<string, ActivityEvent[]>;
  setAgents: (agents: Agent[]) => void;
  upsertAgentFromEvent: (event: ActivityEvent) => void;
  hydrateAgentEvents: (agentId: string, events: ActivityEvent[]) => void;
  appendAgentEvent: (event: ActivityEvent) => void;
  setSelectedAgentId: (id: string | null) => void;
}

export const createAgentsSlice: StateCreator<AppState, [], [], AgentsSlice> = (
  set,
) => ({
  agents: [],
  selectedAgentId: null,
  agentEventsById: {},
  setAgents: (agents) =>
    set((state) => {
      const normalized = normalizeAgents(agents);
      const retainedIds = retainedAgentIds(normalized, state.selectedAgentId);
      const retained = new Set(retainedIds);
      return {
        agents: normalized.filter((agent) => retained.has(agent.id)),
        agentEventsById: pruneAgentEventsById(
          state.agentEventsById,
          retainedIds,
        ),
      };
    }),
  upsertAgentFromEvent: (event) =>
    set((state) => {
      const agentId = event.agentId?.trim();
      if (!agentId) {
        return state;
      }
      const existing = state.agents.find((agent) => agent.id === agentId);

      if (!existing) {
        const nextAgents = normalizeAgents([
          {
            id: agentId,
            name: agentId,
            connectedAt: event.timestamp,
            lastActivity: event.timestamp,
            requestCount: 1,
          },
          ...state.agents,
        ]);
        const retainedIds = retainedAgentIds(
          nextAgents,
          state.selectedAgentId,
          [agentId],
        );
        const retained = new Set(retainedIds);
        return {
          agents: nextAgents.filter((agent) => retained.has(agent.id)),
          agentEventsById: pruneAgentEventsById(
            state.agentEventsById,
            retainedIds,
          ),
        };
      }

      const nextAgents = normalizeAgents(
        state.agents.map((agent) =>
          agent.id === agentId
            ? {
                ...agent,
                lastActivity: event.timestamp,
                requestCount: agent.requestCount + 1,
              }
            : agent,
        ),
      );
      const retainedIds = retainedAgentIds(nextAgents, state.selectedAgentId, [
        agentId,
      ]);
      const retained = new Set(retainedIds);
      return {
        agents: nextAgents.filter((agent) => retained.has(agent.id)),
        agentEventsById: pruneAgentEventsById(
          state.agentEventsById,
          retainedIds,
        ),
      };
    }),
  hydrateAgentEvents: (agentId, events) =>
    set((state) => {
      const retainedIds = retainedAgentIds(
        state.agents,
        state.selectedAgentId,
        [agentId],
      );
      return {
        agentEventsById: {
          ...pruneAgentEventsById(state.agentEventsById, retainedIds),
          [agentId]: mergeAgentEvents(
            state.agentEventsById[agentId] ?? [],
            events,
          ),
        },
      };
    }),
  appendAgentEvent: (event) =>
    set((state) => {
      const agentId = event.agentId?.trim();
      if (!agentId) {
        return state;
      }
      const current = state.agentEventsById[agentId] ?? [];
      if (current.some((existing) => existing.id === event.id)) {
        return state;
      }
      const next = mergeAgentEvents(current, [event]);
      const retainedIds = retainedAgentIds(
        state.agents,
        state.selectedAgentId,
        [agentId],
      );
      return {
        agentEventsById: {
          ...pruneAgentEventsById(state.agentEventsById, retainedIds),
          [agentId]: next,
        },
      };
    }),
  setSelectedAgentId: (selectedAgentId) => set({ selectedAgentId }),
});
