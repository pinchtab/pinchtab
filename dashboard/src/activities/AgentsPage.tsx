import { EmptyState } from "../components/atoms";
import { AgentItem } from "../components/molecules";
import { useAppStore } from "../stores/useAppStore";
import ActivityLine from "./ActivityLine";

const filters = [
  { key: "all", label: "All" },
  { key: "progress", label: "Progress" },
  { key: "tool_call", label: "Tool Calls" },
  { key: "navigate", label: "Navigate" },
  { key: "snapshot", label: "Snapshot" },
  { key: "action", label: "Actions" },
];

export default function AgentsPage() {
  const {
    agents,
    selectedAgentId,
    events,
    eventFilter,
    setSelectedAgentId,
    setEventFilter,
  } = useAppStore();

  const filteredEvents = events.filter((e) => {
    if (selectedAgentId && e.agentId !== selectedAgentId) return false;
    if (eventFilter === "all") return true;
    if (eventFilter === "progress" || eventFilter === "tool_call") {
      return e.channel === eventFilter;
    }
    if (eventFilter === "action") return e.type === "action";
    return e.type === eventFilter;
  });

  return (
    <div className="flex h-full flex-col sm:flex-row">
      <div className="order-2 shrink-0 border-t border-border-subtle bg-bg-surface sm:hidden">
        <div className="flex items-center gap-2 overflow-x-auto px-4 py-3">
          <button
            className={`shrink-0 rounded-full px-4 py-2 text-sm font-medium transition-all ${
              !selectedAgentId
                ? "bg-primary text-white"
                : "bg-bg-elevated text-text-secondary hover:bg-bg-elevated/80"
            }`}
            onClick={() => setSelectedAgentId(null)}
          >
            All
          </button>
          {agents.map((agent) => (
            <button
              key={agent.id}
              className={`shrink-0 rounded-full px-4 py-2 text-sm font-medium transition-all ${
                selectedAgentId === agent.id
                  ? "bg-primary text-white"
                  : "bg-bg-elevated text-text-secondary hover:bg-bg-elevated/80"
              }`}
              onClick={() => setSelectedAgentId(agent.id)}
            >
              {agent.name || agent.id}
            </button>
          ))}
        </div>
      </div>

      <div className="flex flex-1 flex-col overflow-hidden">
        <div className="flex items-center justify-between border-b border-border-subtle bg-bg-surface px-4 py-3">
          <div>
            <div className="dashboard-section-label mb-1">Agent Stream</div>
            <h2 className="text-sm font-semibold text-text-secondary">
              Live Reasoning Feed
            </h2>
          </div>
          <div className="flex gap-1">
            {filters.map((f) => (
              <button
                key={f.key}
                className={`rounded-sm border px-2.5 py-1.5 text-xs font-semibold uppercase tracking-[0.08em] transition-all ${
                  eventFilter === f.key
                    ? "border-primary/30 bg-primary/10 text-primary"
                    : "border-transparent text-text-muted hover:border-border-subtle hover:bg-bg-elevated hover:text-text-secondary"
                }`}
                onClick={() => setEventFilter(f.key)}
              >
                {f.label}
              </button>
            ))}
          </div>
        </div>

        <div className="flex-1 overflow-auto">
          {filteredEvents.length === 0 ? (
            <EmptyState title="Waiting for events..." icon="📡" />
          ) : (
            filteredEvents.map((event) => (
              <ActivityLine key={event.id} event={event} />
            ))
          )}
        </div>
      </div>

      <div className="order-1 hidden w-80 shrink-0 border-l border-border-subtle bg-bg-surface sm:block">
        <div className="border-b border-border-subtle px-4 py-3">
          <div className="dashboard-section-label mb-1">Agents</div>
          <h2 className="text-sm font-semibold text-text-secondary">
            Agent Workspace
          </h2>
          <p className="mt-2 text-xs leading-5 text-text-muted">
            Agent-centric layout foundation. Session drill-down can layer in
            next without changing the main stream area.
          </p>
        </div>
        <div className="p-2">
          {agents.length === 0 ? (
            <div className="py-8 text-center text-sm text-text-muted">
              <div className="mb-2 text-2xl">🦀</div>
              No agent activity observed yet
            </div>
          ) : (
            <div className="flex flex-col gap-1">
              <button
                className={`rounded-sm px-3 py-2 text-left text-sm transition-all ${
                  !selectedAgentId
                    ? "border border-primary/30 bg-primary/10 text-primary"
                    : "border border-transparent text-text-muted hover:bg-bg-elevated"
                }`}
                onClick={() => setSelectedAgentId(null)}
              >
                All Agents
              </button>
              {agents.map((agent) => (
                <AgentItem
                  key={agent.id}
                  agent={agent}
                  selected={selectedAgentId === agent.id}
                  onClick={() => setSelectedAgentId(agent.id)}
                />
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
