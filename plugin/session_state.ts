import type { AgentSessionState, PluginRuntimeContext } from "./types.js";

const agentSessionMaxEntries = 256;
const agentSessionMaxAgeMs = 60 * 60 * 1000;

const agentSessions = new Map<string, AgentSessionState>();

function resolveSessionStateKey(context?: PluginRuntimeContext): string {
  return context?.agentId || context?.sessionId || "global";
}

function pruneAgentSessions(): void {
  const cutoff = Date.now() - agentSessionMaxAgeMs;
  for (const [key, state] of agentSessions) {
    if ((state.updatedAt ?? 0) < cutoff) agentSessions.delete(key);
  }
  if (agentSessions.size <= agentSessionMaxEntries) return;
  const ordered = [...agentSessions.entries()].sort(
    (a, b) => (a[1].updatedAt ?? 0) - (b[1].updatedAt ?? 0),
  );
  for (const [key] of ordered) {
    if (agentSessions.size <= agentSessionMaxEntries) break;
    agentSessions.delete(key);
  }
}

export function rememberRuntimeContext(context?: PluginRuntimeContext): AgentSessionState {
  const key = resolveSessionStateKey(context);
  const existing = agentSessions.get(key);
  const next: AgentSessionState = {
    key,
    agentId: context?.agentId ?? existing?.agentId,
    sessionId: context?.sessionId ?? existing?.sessionId,
    lastTabId: existing?.lastTabId,
    updatedAt: Date.now(),
  };
  agentSessions.set(key, next);
  pruneAgentSessions();
  return next;
}

export function getAgentSessionState(context?: PluginRuntimeContext): AgentSessionState | undefined {
  return agentSessions.get(resolveSessionStateKey(context));
}

export function getLastTabId(context?: PluginRuntimeContext): string | undefined {
  return getAgentSessionState(context)?.lastTabId;
}

export function setLastTabId(tabId: string | undefined, context?: PluginRuntimeContext): void {
  const state = rememberRuntimeContext(context);
  state.lastTabId = tabId;
  state.updatedAt = Date.now();
}
