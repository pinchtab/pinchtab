import AgentActivityWorkspace from "./AgentActivityWorkspace";

export default function AgentsPage() {
  return (
    <AgentActivityWorkspace
      defaultSidebarTab="agents"
      hiddenSources={["dashboard", "server"]}
    />
  );
}
