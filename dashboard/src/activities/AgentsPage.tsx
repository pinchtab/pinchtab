import AgentActivityWorkspace from "./AgentActivityWorkspace";

const AGENT_PAGE_INITIAL_FILTERS = { ageSec: "", limit: "1000" } as const;

export default function AgentsPage() {
  return (
    <AgentActivityWorkspace
      initialFilters={AGENT_PAGE_INITIAL_FILTERS}
      defaultSidebarTab="agents"
      requireAgentIdentity
      requireSelectedAgent
      showAllAgentsOption={false}
      showAgentFilter
      simplifyEventRows
      copyTabId
      preferKnownAgents
      useAgentEventStore
      clearToInitialFilters
    />
  );
}
