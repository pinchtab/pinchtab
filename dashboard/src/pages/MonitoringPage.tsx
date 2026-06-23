import { ErrorBoundary } from "../components/atoms";
import InstanceListItem from "../instances/InstanceListItem";
import InstanceTabsPanel from "../tabs/InstanceTabsPanel";
import { useMonitoringController } from "./monitoring/useMonitoringController";
import MonitoringEmptyState from "./monitoring/MonitoringEmptyState";
import DefaultInstanceModal from "./monitoring/DefaultInstanceModal";

export default function MonitoringPage() {
  const m = useMonitoringController();

  const emptyState = (
    <MonitoringEmptyState
      waitingForExpectedInstance={m.waitingForExpectedInstance}
      startupRetriesRemaining={m.startupRetriesRemaining}
      expectsAutoInstance={m.expectsAutoInstance}
      launchError={m.launchError}
      startingDefaultInstance={m.startingDefaultInstance}
      onStartDefault={() => {
        m.setLaunchError("");
        m.setShowDefaultModeModal(true);
      }}
      onOpenDefaultProfile={() =>
        m.navigate("/dashboard/profiles", {
          state: { selectedProfileKey: m.defaultProfileId },
        })
      }
    />
  );

  return (
    <ErrorBoundary>
      <>
        <div className="flex h-full flex-col overflow-hidden">
          {m.instances.length === 0 && (
            <div className="flex flex-1 items-center justify-center">
              {emptyState}
            </div>
          )}

          {m.instances.length > 0 && (
            <div className="dashboard-panel flex flex-1 flex-col overflow-hidden rounded-none! border-t-0">
              <div className="flex flex-1 overflow-hidden">
                {!m.sidebarCollapsed && (
                  <div className="w-64 shrink-0 overflow-auto border-r border-border-subtle bg-bg-surface/50">
                    <div className="flex items-center justify-between border-b border-border-subtle px-3 py-1.5">
                      <span className="text-xs font-medium text-text-muted">
                        Instances
                      </span>
                      <button
                        type="button"
                        onClick={() => m.setSidebarCollapsed(true)}
                        title="Collapse sidebar"
                        className="rounded p-1 text-text-muted transition-colors hover:bg-white/10 hover:text-text-secondary"
                      >
                        <svg
                          viewBox="0 0 24 24"
                          aria-hidden="true"
                          className="h-3.5 w-3.5"
                          fill="none"
                          stroke="currentColor"
                          strokeWidth="2"
                          strokeLinecap="round"
                          strokeLinejoin="round"
                        >
                          <polyline points="15 18 9 12 15 6" />
                        </svg>
                      </button>
                    </div>
                    <div>
                      {m.instances.map((inst) => (
                        <InstanceListItem
                          key={inst.id}
                          instance={inst}
                          tabCount={m.currentTabs[inst.id]?.length ?? 0}
                          memoryMB={
                            m.memoryEnabled
                              ? m.currentMemory[inst.id]
                              : undefined
                          }
                          selected={m.selectedId === inst.id}
                          autoRestart={
                            inst.profileName === "default" &&
                            (m.strategy === "always-on" ||
                              m.strategy === "simple-autorestart")
                          }
                          onClick={() => m.setSelectedId(inst.id)}
                          onStop={() => m.handleStop(inst.id)}
                          onOpenProfile={() =>
                            m.navigate("/dashboard/profiles", {
                              state: {
                                selectedProfileKey:
                                  inst.profileId || inst.profileName,
                              },
                            })
                          }
                        />
                      ))}
                    </div>
                  </div>
                )}

                {/* Selected instance details */}
                <div className="flex flex-1 flex-col overflow-hidden">
                  {m.selectedInstance ? (
                    <InstanceTabsPanel
                      tabs={m.selectedTabs}
                      instanceId={m.selectedId || undefined}
                      handoffTabs={m.handoffTabs}
                    />
                  ) : (
                    <div className="flex flex-1 items-center justify-center px-6">
                      {emptyState}
                    </div>
                  )}
                </div>
              </div>
            </div>
          )}
        </div>

        <DefaultInstanceModal
          open={m.showDefaultModeModal}
          startingDefaultInstance={m.startingDefaultInstance}
          startingDefaultMode={m.startingDefaultMode}
          prefersHeadedLaunch={m.prefersHeadedLaunch}
          launchError={m.launchError}
          defaultLaunchMode={m.defaultLaunchMode}
          onClose={() => {
            if (!m.startingDefaultInstance) {
              m.setShowDefaultModeModal(false);
            }
          }}
          onStart={m.handleStartDefaultInstance}
        />
      </>
    </ErrorBoundary>
  );
}
