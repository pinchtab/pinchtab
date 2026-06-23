import type { Dispatch, SetStateAction } from "react";
import { useState } from "react";
import { Button, Card, Input, Modal } from "../components/atoms";
import { SidebarPanel, SidebarPanelHeader } from "../components/molecules";
import type {
  BackendConfig,
  BackendConfigState,
  LocalDashboardSettings,
} from "../types";
import { BrowserSettingsSection } from "./settings/BrowserSettingsSection";
import { DashboardSettingsSection } from "./settings/DashboardSettingsSection";
import { DefaultsSettingsSection } from "./settings/DefaultsSettingsSection";
import { NetworkSettingsSection } from "./settings/NetworkSettingsSection";
import { OrchestrationSettingsSection } from "./settings/OrchestrationSettingsSection";
import { ProfilesSettingsSection } from "./settings/ProfilesSettingsSection";
import { SecurityIdpiSettingsSection } from "./settings/SecurityIdpiSettingsSection";
import { SecuritySettingsSection } from "./settings/SecuritySettingsSection";
import { AutoSolverSettingsSection } from "./settings/AutoSolverSettingsSection";
import { ObservabilitySettingsSection } from "./settings/ObservabilitySettingsSection";
import {
  sections,
  type SectionId,
  type UpdateBackendSection,
} from "./settings/settingsShared";
import { TimeoutsSettingsSection } from "./settings/TimeoutsSettingsSection";
import { useSettingsController } from "./settings/useSettingsController";

function renderActiveSection(
  activeSection: SectionId,
  options: {
    apiTokenMissing: boolean;
    attachWildcard: boolean;
    backendConfig: BackendConfig;
    backendState: BackendConfigState | null;
    idpiDomainsConfigured: boolean;
    idpiEnabled: boolean;
    idpiWildcard: boolean;
    localSettings: LocalDashboardSettings;
    nonLoopbackBind: boolean;
    sensitiveEndpointsEnabled: boolean;
    setLocalSettings: Dispatch<SetStateAction<LocalDashboardSettings>>;
    updateBackendSection: UpdateBackendSection;
  },
) {
  switch (activeSection) {
    case "dashboard":
      return (
        <DashboardSettingsSection
          localSettings={options.localSettings}
          setLocalSettings={options.setLocalSettings}
        />
      );
    case "defaults":
      return (
        <DefaultsSettingsSection
          backendConfig={options.backendConfig}
          updateBackendSection={options.updateBackendSection}
        />
      );
    case "orchestration":
      return (
        <OrchestrationSettingsSection
          backendConfig={options.backendConfig}
          updateBackendSection={options.updateBackendSection}
        />
      );
    case "security":
      return (
        <SecuritySettingsSection
          backendConfig={options.backendConfig}
          sensitiveEndpointsEnabled={options.sensitiveEndpointsEnabled}
          updateBackendSection={options.updateBackendSection}
        />
      );
    case "security-idpi":
      return (
        <SecurityIdpiSettingsSection
          backendConfig={options.backendConfig}
          idpiDomainsConfigured={options.idpiDomainsConfigured}
          idpiEnabled={options.idpiEnabled}
          idpiWildcard={options.idpiWildcard}
          updateBackendSection={options.updateBackendSection}
        />
      );
    case "profiles":
      return (
        <ProfilesSettingsSection
          backendConfig={options.backendConfig}
          updateBackendSection={options.updateBackendSection}
        />
      );
    case "network":
      return (
        <NetworkSettingsSection
          apiTokenMissing={options.apiTokenMissing}
          attachWildcard={options.attachWildcard}
          backendConfig={options.backendConfig}
          backendState={options.backendState}
          nonLoopbackBind={options.nonLoopbackBind}
          updateBackendSection={options.updateBackendSection}
        />
      );
    case "browser":
      return (
        <BrowserSettingsSection
          backendConfig={options.backendConfig}
          updateBackendSection={options.updateBackendSection}
        />
      );
    case "timeouts":
      return (
        <TimeoutsSettingsSection
          backendConfig={options.backendConfig}
          updateBackendSection={options.updateBackendSection}
        />
      );
    case "autosolver":
      return (
        <AutoSolverSettingsSection
          backendConfig={options.backendConfig}
          backendState={options.backendState}
          updateBackendSection={options.updateBackendSection}
        />
      );
    case "observability":
      return (
        <ObservabilitySettingsSection
          backendConfig={options.backendConfig}
          updateBackendSection={options.updateBackendSection}
        />
      );
  }
}

export default function SettingsPage() {
  const [activeSection, setActiveSection] = useState<SectionId>("dashboard");
  const {
    serverInfo,
    localSettings,
    setLocalSettings,
    backendState,
    backendConfig,
    loading,
    saving,
    error,
    notice,
    pendingElevatedAction,
    elevationToken,
    setElevationToken,
    elevationError,
    elevating,
    hasChanges,
    restartRequired,
    restartReasons,
    sensitiveEndpointsEnabled,
    apiTokenMissing,
    idpiEnabled,
    idpiWildcard,
    idpiDomainsConfigured,
    attachWildcard,
    nonLoopbackBind,
    updateBackendSection,
    handleReset,
    handleSave,
    closeElevationPrompt,
    handleElevationSubmit,
  } = useSettingsController();

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <Modal
        open={pendingElevatedAction !== null}
        onClose={closeElevationPrompt}
        title="Confirm admin action"
        actions={
          <>
            <Button
              variant="secondary"
              onClick={closeElevationPrompt}
              disabled={elevating}
            >
              Cancel
            </Button>
            <Button
              variant="primary"
              type="submit"
              form="settings-elevation-form"
              disabled={elevating || elevationToken.trim() === ""}
            >
              {elevating ? "Verifying..." : "Continue"}
            </Button>
          </>
        }
      >
        <form
          id="settings-elevation-form"
          className="space-y-4"
          autoComplete="off"
          onSubmit={handleElevationSubmit}
        >
          <p className="leading-6 text-text-muted">
            Re-enter the API token to save backend configuration changes. The
            elevated session stays active briefly so you do not need to repeat
            this for every admin action.
          </p>
          <Input
            id="settings-elevation-password"
            type="password"
            autoComplete="off"
            label="API token"
            placeholder="Paste API token"
            value={elevationToken}
            onChange={(e) => setElevationToken(e.target.value)}
            autoFocus
            spellCheck={false}
            autoCapitalize="none"
          />
          {elevationError && (
            <div className="rounded-sm border border-destructive/35 bg-destructive/10 px-3 py-2 text-xs leading-5 text-destructive">
              {elevationError}
            </div>
          )}
        </form>
      </Modal>

      <div className="flex flex-1 flex-col overflow-hidden lg:flex-row">
        <SidebarPanel
          as="aside"
          chrome="sidebar"
          contentPadding="sm"
          headerPadding="sm"
          surface="panel"
          width="narrow"
          header={
            <SidebarPanelHeader
              eyebrow="Settings"
              description={`Version: ${serverInfo?.version || "dev"}`}
              descriptionClassName="dashboard-mono"
            />
          }
        >
          <div className="flex flex-col gap-1.5">
            {sections.map((section) => (
              <button
                key={section.id}
                type="button"
                className={`rounded-sm border px-3 py-2.5 text-left transition-all ${
                  activeSection === section.id
                    ? "border-primary/30 bg-primary/10 text-text-primary"
                    : "border-transparent text-text-secondary hover:border-border-subtle hover:bg-bg-elevated hover:text-text-primary"
                }`}
                onClick={() => setActiveSection(section.id)}
              >
                <div className="text-sm font-medium">{section.label}</div>
                <div className="mt-1 text-xs leading-5 text-text-muted">
                  {section.description}
                </div>
              </button>
            ))}
          </div>
        </SidebarPanel>

        <div className="flex-1 overflow-y-auto pr-1">
          <div className="sticky top-0 z-10 flex flex-wrap items-center gap-2 border-b border-border-subtle bg-bg-surface/95 p-3 backdrop-blur">
            {restartRequired && (
              <div className="rounded-sm border border-warning/25 bg-warning/10 px-3 py-2 text-xs font-semibold uppercase tracking-[0.08em] text-warning">
                Restart required
              </div>
            )}
            <div className="flex-1" />
            <Button
              variant="secondary"
              onClick={handleReset}
              disabled={!hasChanges || saving}
            >
              Reset
            </Button>
            <Button
              variant="primary"
              onClick={handleSave}
              disabled={!hasChanges || saving || !backendConfig}
            >
              {saving ? "Saving..." : "Save"}
            </Button>
          </div>
          {(error || notice || restartReasons.length > 0) && (
            <div className="flex flex-col gap-2 px-3 pb-3">
              {error && (
                <div className="rounded-sm border border-destructive/35 bg-destructive/10 px-3 py-2 text-sm text-destructive">
                  {error}
                </div>
              )}
              {notice && (
                <div className="rounded-sm border border-primary/30 bg-primary/10 px-3 py-2 text-sm text-primary">
                  {notice}
                </div>
              )}
              {restartRequired && restartReasons.length > 0 && (
                <div className="rounded-sm border border-warning/25 bg-warning/10 px-3 py-2 text-sm text-warning">
                  Restart needed for: {restartReasons.join(", ")}.
                </div>
              )}
            </div>
          )}
          {loading || !backendConfig ? (
            <Card className="p-6">
              <div className="text-sm text-text-muted">Loading settings…</div>
            </Card>
          ) : (
            renderActiveSection(activeSection, {
              apiTokenMissing,
              attachWildcard,
              backendConfig,
              backendState,
              idpiDomainsConfigured,
              idpiEnabled,
              idpiWildcard,
              localSettings,
              nonLoopbackBind,
              sensitiveEndpointsEnabled,
              setLocalSettings,
              updateBackendSection,
            })
          )}
        </div>
      </div>
    </div>
  );
}
