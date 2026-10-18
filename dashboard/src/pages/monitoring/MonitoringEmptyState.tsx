import { Button, EmptyState } from "../../components/atoms";

interface MonitoringEmptyStateProps {
  waitingForExpectedInstance: boolean;
  startupRetriesRemaining: number;
  expectsAutoInstance: boolean;
  launchError: string;
  startingDefaultInstance: boolean;
  onStartDefault: () => void;
  onOpenDefaultProfile: () => void;
}

import { useTranslation } from "react-i18next";

export default function MonitoringEmptyState({
  waitingForExpectedInstance,
  startupRetriesRemaining,
  expectsAutoInstance,
  launchError,
  startingDefaultInstance,
  onStartDefault,
  onOpenDefaultProfile,
}: MonitoringEmptyStateProps) {
  const { t } = useTranslation();

  if (waitingForExpectedInstance) {
    return (
      <EmptyState
        title={t("monitoringEmpty.starting")}
        description={t("monitoringEmpty.waiting", {
          count: startupRetriesRemaining,
        })}
        icon="⏳"
      />
    );
  }

  const manualActions = (
    <div className="flex flex-wrap items-center justify-center gap-2">
      <Button
        variant="primary"
        onClick={onStartDefault}
        loading={startingDefaultInstance}
      >
        {t("monitoringEmpty.startDefault")}
      </Button>
      <Button variant="secondary" onClick={onOpenDefaultProfile}>
        {t("monitoringEmpty.openDefaultProfile")}
      </Button>
    </div>
  );

  return (
    <div className="flex flex-col items-center gap-4">
      {launchError && (
        <div className="max-w-md rounded border border-destructive/50 bg-destructive/10 px-3 py-2 text-sm text-destructive">
          {launchError}
        </div>
      )}
      <EmptyState
        title={t("monitoringEmpty.noActive")}
        description={
          expectsAutoInstance
            ? t("monitoringEmpty.expectedMissing")
            : t("monitoringEmpty.startHint")
        }
        icon="📡"
        action={manualActions}
      />
    </div>
  );
}
