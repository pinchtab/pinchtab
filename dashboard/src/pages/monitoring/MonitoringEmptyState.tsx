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

export default function MonitoringEmptyState({
  waitingForExpectedInstance,
  startupRetriesRemaining,
  expectsAutoInstance,
  launchError,
  startingDefaultInstance,
  onStartDefault,
  onOpenDefaultProfile,
}: MonitoringEmptyStateProps) {
  if (waitingForExpectedInstance) {
    return (
      <EmptyState
        title="Starting default instance..."
        description={`PinchTab is waiting for the default profile to come online. Checking again automatically (${startupRetriesRemaining} checks left).`}
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
        Start Default Instance
      </Button>
      <Button variant="secondary" onClick={onOpenDefaultProfile}>
        Open Default Profile
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
        title="No active instances"
        description={
          expectsAutoInstance
            ? "PinchTab expected a default instance, but it never became available. Start it manually or inspect the profile."
            : "Start the default instance or open Profiles to launch a different one."
        }
        icon="📡"
        action={manualActions}
      />
    </div>
  );
}
