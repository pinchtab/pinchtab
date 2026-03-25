import { AgentItem } from "../components/molecules";
import type { Agent, Instance, InstanceTab, Profile } from "../types";
import ActivityFilterMenu from "./ActivityFilterMenu";
import type { ActivityFilters } from "./types";

type WorkspaceTab = "agents" | "activities";

interface AgentWorkspaceSidebarProps {
  sidebarTab: WorkspaceTab;
  visibleAgents: Agent[];
  activeAgentId: string;
  filters: ActivityFilters;
  profiles: Profile[];
  filteredInstances: Instance[];
  visibleTabs: InstanceTab[];
  loading: boolean;
  onSidebarTabChange: (tab: WorkspaceTab) => void;
  onSelectAgent: (agentId: string) => void;
  onClearAgentSelection: () => void;
  onClearFilters: () => void;
  onRefresh: () => void;
  onFilterChange: (key: keyof ActivityFilters, value: string) => void;
  onProfileChange: (value: string) => void;
  onInstanceChange: (value: string) => void;
}

export default function AgentWorkspaceSidebar({
  sidebarTab,
  visibleAgents,
  activeAgentId,
  filters,
  profiles,
  filteredInstances,
  visibleTabs,
  loading,
  onSidebarTabChange,
  onSelectAgent,
  onClearAgentSelection,
  onClearFilters,
  onRefresh,
  onFilterChange,
  onProfileChange,
  onInstanceChange,
}: AgentWorkspaceSidebarProps) {
  return (
    <aside className="order-2 flex w-full shrink-0 flex-col overflow-hidden border-t border-border-subtle bg-bg-surface xl:order-0 xl:w-80 xl:border-t-0 xl:border-l">
      <div className="flex border-b border-border-subtle">
        {[
          { id: "agents" as const, label: "Agents" },
          { id: "activities" as const, label: "Activities" },
        ].map((tab) => (
          <button
            key={tab.id}
            type="button"
            className={`flex-1 border-b px-4 py-3 text-sm font-semibold transition-colors ${
              sidebarTab === tab.id
                ? "border-primary bg-primary/8 text-text-primary"
                : "border-transparent text-text-muted hover:bg-bg-elevated hover:text-text-primary"
            }`}
            onClick={() => onSidebarTabChange(tab.id)}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {sidebarTab === "agents" ? (
        <div className="min-h-0 flex-1 overflow-auto p-2">
          {visibleAgents.length === 0 ? (
            <div className="py-8 text-center text-sm text-text-muted">
              <div className="mb-2 text-2xl">🦀</div>
              No agent activity observed yet
            </div>
          ) : (
            <div className="flex flex-col gap-1">
              <button
                type="button"
                className={`rounded-sm px-3 py-2 text-left text-sm transition-all ${
                  activeAgentId === ""
                    ? "border border-primary/30 bg-primary/10 text-primary"
                    : "border border-transparent text-text-muted hover:bg-bg-elevated"
                }`}
                onClick={onClearAgentSelection}
              >
                All Agents
              </button>
              {visibleAgents.map((agent) => (
                <AgentItem
                  key={agent.id}
                  agent={agent}
                  selected={activeAgentId === agent.id}
                  onClick={() => onSelectAgent(agent.id)}
                />
              ))}
            </div>
          )}
        </div>
      ) : (
        <ActivityFilterMenu
          filters={filters}
          profileOptions={profiles}
          instanceOptions={filteredInstances}
          tabOptions={visibleTabs}
          loading={loading}
          onClear={onClearFilters}
          onRefresh={onRefresh}
          onFilterChange={onFilterChange}
          onProfileChange={onProfileChange}
          onInstanceChange={onInstanceChange}
        />
      )}
    </aside>
  );
}
