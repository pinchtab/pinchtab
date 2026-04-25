import { useMemo } from "react";
import { AgentItem, SidebarPanel } from "../components/molecules";
import type { Agent, Instance, InstanceTab, Profile } from "../types";
import type { Session } from "../services/api";
import {
  ActivityFilterActions,
  ActivityFilterFields,
} from "./ActivityFilterMenu";
import type { ActivityFilters } from "./types";

type WorkspaceTab = "agents" | "activities";

interface AgentWorkspaceSidebarProps {
  sidebarTab: WorkspaceTab;
  visibleAgents: Agent[];
  activeAgentId: string;
  filters: ActivityFilters;
  sessions: Session[];
  sessionsWithHandoff?: Set<string>;
  agentsWithHandoff?: Set<string>;
  showAllAgentsOption?: boolean;
  showAgentFilter?: boolean;
  profiles: Profile[];
  filteredInstances: Instance[];
  visibleTabs: InstanceTab[];
  agentOptions?: string[];
  loading: boolean;
  onSidebarTabChange: (tab: WorkspaceTab) => void;
  onSelectAgent: (agentId: string, autoSessionId?: string) => void;
  onSelectSession: (sessionId: string) => void;
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
  sessions,
  sessionsWithHandoff,
  agentsWithHandoff,
  showAllAgentsOption = true,
  showAgentFilter = true,
  profiles,
  filteredInstances,
  visibleTabs,
  agentOptions,
  loading,
  onSidebarTabChange,
  onSelectAgent,
  onSelectSession,
  onClearFilters,
  onRefresh,
  onFilterChange,
  onProfileChange,
  onInstanceChange,
}: AgentWorkspaceSidebarProps) {
  const sessionsByAgent = useMemo(() => {
    const map = new Map<string, Session[]>();
    for (const session of sessions) {
      const agentId = session.agentId || "";
      if (!agentId) continue;
      const list = map.get(agentId) || [];
      list.push(session);
      map.set(agentId, list);
    }
    for (const list of map.values()) {
      list.sort(
        (a, b) =>
          new Date(b.lastSeenAt || b.createdAt).getTime() -
          new Date(a.lastSeenAt || a.createdAt).getTime(),
      );
    }
    return map;
  }, [sessions]);

  return (
    <SidebarPanel
      as="aside"
      chrome="sidebar"
      surface="surface"
      width="wide"
      header={
        <div className="flex">
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
      }
      footer={
        sidebarTab === "activities" ? (
          <ActivityFilterActions
            loading={loading}
            onClear={onClearFilters}
            onRefresh={onRefresh}
          />
        ) : undefined
      }
      headerPadding="none"
    >
      {sidebarTab === "agents" ? (
        visibleAgents.length === 0 ? (
          <div className="py-8 text-center text-sm text-text-muted">
            <div className="mb-2 text-2xl">🦀</div>
            No agent activity observed yet
          </div>
        ) : (
          <div className="flex flex-col">
            {showAllAgentsOption && (
              <button
                type="button"
                className={`px-3 py-2.5 text-left text-sm transition-colors ${
                  activeAgentId === ""
                    ? "bg-primary/8 text-primary"
                    : "text-text-muted hover:bg-bg-elevated"
                }`}
                onClick={() => onSelectAgent("")}
              >
                All Agents
              </button>
            )}
            {visibleAgents.map((agent) => (
              <AgentItem
                key={agent.id}
                agent={agent}
                selected={activeAgentId === agent.id}
                sessions={sessionsByAgent.get(agent.id) || []}
                activeSessionId={filters.sessionId}
                hasHandoff={agentsWithHandoff?.has(agent.id) ?? false}
                sessionsWithHandoff={sessionsWithHandoff}
                onClick={() => onSelectAgent(agent.id)}
                onSelectSession={onSelectSession}
              />
            ))}
          </div>
        )
      ) : (
        <ActivityFilterFields
          filters={filters}
          profileOptions={profiles}
          instanceOptions={filteredInstances}
          tabOptions={visibleTabs}
          agentOptions={agentOptions}
          showAgentFilter={showAgentFilter}
          onFilterChange={onFilterChange}
          onProfileChange={onProfileChange}
          onInstanceChange={onInstanceChange}
        />
      )}
    </SidebarPanel>
  );
}
